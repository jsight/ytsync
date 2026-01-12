package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"
	"ytsync/config"
	"ytsync/youtube"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "list":
		cmdList(args)
	case "transcript":
		cmdTranscript(args)
	case "download":
		cmdDownload(args)
	case "help", "-h", "--help":
		printUsage()
	default:
		// Assume it's a list command for backward compatibility
		cmdList(os.Args[1:])
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `ytsync - YouTube downloader and transcript extractor

Usage:
  ytsync list [flags] <youtube-url>     List videos from a channel
  ytsync transcript [flags] <video-id>  Extract transcript from a video
  ytsync download [flags] <video-id>    Download a video
  ytsync help                           Show this help message

Examples:
  ytsync https://www.youtube.com/channel/UCxxxxx              # List videos (default)
  ytsync --type both --max 10 <url>                           # Advanced listing
  ytsync transcript dQw4w9WgXcQ                               # Get transcript
  ytsync transcript dQw4w9WgXcQ --lang en,es                  # Multiple languages
  ytsync download dQw4w9WgXcQ                                 # Download video
  ytsync download dQw4w9WgXcQ --audio-only                    # Audio only
  ytsync download dQw4w9WgXcQ --dir ~/Downloads               # Specify directory

For help on specific command: ytsync <command> -h
`)
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	useRSS := fs.Bool("rss", false, "Use RSS feed instead of yt-dlp for listing")
	maxVideos := fs.Int("max", 0, "Maximum videos to list (0 = all)")
	since := fs.String("since", "", "Only videos published after this date (RFC3339)")
	contentTypeStr := fs.String("type", "videos", "Content type: videos, streams, or both")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ytsync list [flags] <youtube-url>\n\nFlags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	argv := fs.Args()
	if len(argv) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing youtube-url\n")
		fs.Usage()
		os.Exit(1)
	}

	channelURL := argv[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Parse date filter if provided
	var publishedAfter time.Time
	if *since != "" {
		t, err := time.Parse(time.RFC3339, *since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing --since: %v (use RFC3339 format)\n", err)
			os.Exit(1)
		}
		publishedAfter = t
	}

	// Parse content type
	var contentType youtube.ContentType
	switch *contentTypeStr {
	case "videos":
		contentType = youtube.ContentTypeVideos
	case "streams":
		contentType = youtube.ContentTypeStreams
	case "both":
		contentType = youtube.ContentTypeBoth
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid --type value %q (use videos, streams, or both)\n", *contentTypeStr)
		os.Exit(1)
	}

	// Create lister
	var lister youtube.VideoLister
	if *useRSS {
		lister = youtube.NewRSSLister()
	} else {
		ytdlp := youtube.NewYtdlpLister()
		ytdlp.Path = cfg.YtdlpPath
		ytdlp.Timeout = cfg.YtdlpTimeout
		lister = ytdlp
	}

	// Build list options
	opts := &youtube.ListOptions{
		MaxResults:     *maxVideos,
		PublishedAfter: publishedAfter,
		ContentType:    contentType,
	}

	// List videos with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Fetching videos from %s...\n", channelURL)
	videos, err := lister.ListVideos(ctx, channelURL, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching videos: %v\n", err)
		os.Exit(1)
	}

	if len(videos) == 0 {
		fmt.Println("No videos found.")
		return
	}

	// Format and print results
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VIDEO ID\tTITLE\tDURATION\tVIEWS\tTYPE")

	for _, v := range videos {
		duration := ""
		if v.Duration > 0 {
			duration = fmt.Sprintf("%d:%02d", int(v.Duration.Minutes()), int(v.Duration.Seconds())%60)
		}

		views := ""
		if v.ViewCount > 0 {
			views = fmt.Sprintf("%d", v.ViewCount)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			v.ID,
			truncate(v.Title, 50),
			duration,
			views,
			v.Type,
		)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\nTotal: %d videos\n", len(videos))
}

func cmdTranscript(args []string) {
	fs := flag.NewFlagSet("transcript", flag.ExitOnError)
	langStr := fs.String("lang", "", "Comma-separated language codes (e.g., en,es). Empty = all available")
	skipAuto := fs.Bool("no-auto", false, "Skip auto-generated captions")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ytsync transcript [flags] <video-id>\n\nFlags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	argv := fs.Args()
	if len(argv) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing video-id\n")
		fs.Usage()
		os.Exit(1)
	}

	videoID := argv[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Parse language preference
	var languages []string
	if *langStr != "" {
		languages = strings.Split(*langStr, ",")
		for i, lang := range languages {
			languages[i] = strings.TrimSpace(lang)
		}
	}

	// Create extractor
	extractor := youtube.NewTranscriptExtractor()
	extractor.YtdlpPath = cfg.YtdlpPath
	extractor.Timeout = cfg.YtdlpTimeout

	// Extract transcript with configured timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.YtdlpTimeout)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Fetching transcript for %s...\n", videoID)
	opts := &youtube.ExtractOptions{
		Languages:         languages,
		Format:            "json3",
		SkipAutoGenerated: *skipAuto,
	}

	transcript, err := extractor.Extract(ctx, videoID, opts)
	if err != nil {
		// Check for timeout errors
		if strings.Contains(err.Error(), "context deadline exceeded") {
			fmt.Fprintf(os.Stderr, "Error: Request timed out. YouTube may have blocked the request or signature expired.\n")
			fmt.Fprintf(os.Stderr, "Try again in a few minutes, or check if the video has captions.\n")
		} else {
			fmt.Fprintf(os.Stderr, "Error fetching transcript: %v\n", err)
		}
		os.Exit(1)
	}

	// Display result
	fmt.Printf("Video ID:      %s\n", transcript.VideoID)
	fmt.Printf("Language:      %s (%s)\n", transcript.Language, transcript.LanguageName)
	fmt.Printf("Auto-generated: %v\n", transcript.IsAutoGenerated)

	if len(transcript.Entries) > 0 {
		fmt.Printf("\nTranscript (%d entries):\n", len(transcript.Entries))
		fmt.Println()
		for _, entry := range transcript.Entries {
			timestamp := formatTimestamp(entry.Start)
			duration := formatTimestamp(entry.Duration)
			fmt.Printf("[%s +%s] %s\n", timestamp, duration, entry.Text)
		}
	} else if transcript.DownloadURL != "" {
		fmt.Println("\nTranscript URL available but download failed. Try manually:")
		fmt.Printf("  curl %s | jq .\n", transcript.DownloadURL)
	} else {
		fmt.Println("\nNo transcript available for this video")
	}
}

func cmdDownload(args []string) {
	fs := flag.NewFlagSet("download", flag.ExitOnError)
	audioOnly := fs.Bool("audio-only", false, "Download audio only (MP3)")
	outputDir := fs.String("dir", ".", "Directory to save video")
	format := fs.String("format", "best", "Video format: best, mp4, webm, or audio quality")
	noMetadata := fs.Bool("no-metadata", false, "Skip downloading metadata JSON")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ytsync download [flags] <video-id>\n\nFlags:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	argv := fs.Args()
	if len(argv) == 0 {
		fmt.Fprintf(os.Stderr, "Error: missing video-id\n")
		fs.Usage()
		os.Exit(1)
	}

	videoID := argv[0]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Fetch metadata first if not skipped
	var metadata *youtube.VideoMetadata
	if !*noMetadata {
		fmt.Fprintf(os.Stderr, "Fetching metadata...\n")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		metadata, err = youtube.FetchMetadata(ctx, videoID, cfg.YtdlpPath)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch metadata: %v\n", err)
			fmt.Fprintf(os.Stderr, "Continuing with download without metadata...\n")
		}
	}

	// Build yt-dlp arguments
	ytdlpArgs := []string{
		"-o", fmt.Sprintf("%s/%%(title)s.%%(ext)s", *outputDir),
		"--no-warnings",
	}

	if *audioOnly {
		ytdlpArgs = append(ytdlpArgs,
			"-f", "bestaudio/best",
			"-x",
			"--audio-format", "mp3",
			"--audio-quality", "192",
		)
	} else {
		// Video download with best format
		if *format == "best" {
			ytdlpArgs = append(ytdlpArgs, "-f", "best[height<=1080]")
		} else {
			ytdlpArgs = append(ytdlpArgs, "-f", *format)
		}
	}

	ytdlpArgs = append(ytdlpArgs, videoID)

	// Run yt-dlp
	fmt.Fprintf(os.Stderr, "Downloading %s...\n", videoID)
	cmd := exec.Command(cfg.YtdlpPath, ytdlpArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error downloading video: %v\n", err)
		os.Exit(1)
	}

	// Save metadata if we have it
	if metadata != nil {
		metadataPath := fmt.Sprintf("%s/%s.json", *outputDir, sanitizeFilename(metadata.Title))
		if err := saveMetadata(metadata, metadataPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save metadata: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Metadata saved to: %s\n", metadataPath)
		}
	}

	fmt.Fprintf(os.Stderr, "Download complete!\n")
}

// saveMetadata saves video metadata to a JSON file.
func saveMetadata(metadata *youtube.VideoMetadata, path string) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write metadata file: %w", err)
	}

	return nil
}

// sanitizeFilename removes/replaces characters that are invalid in filenames.
func sanitizeFilename(s string) string {
	// Replace invalid characters with underscores
	replacements := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, char := range replacements {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatTimestamp(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}
