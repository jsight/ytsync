package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"ytsync/internal/config"
	"ytsync/internal/youtube"
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
	case "help", "-h", "--help":
		printUsage()
	default:
		// Assume it's a list command for backward compatibility
		cmdList(os.Args[1:])
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `ytsync - YouTube channel lister and transcript extractor

Usage:
  ytsync list [flags] <youtube-url>     List videos from a channel
  ytsync transcript [flags] <video-id>  Extract transcript from a video
  ytsync help                           Show this help message

Examples:
  ytsync https://www.youtube.com/channel/UCxxxxx              # List videos (default)
  ytsync --type both --max 10 <url>                           # Advanced listing
  ytsync transcript dQw4w9WgXcQ                               # Get transcript
  ytsync transcript dQw4w9WgXcQ --lang en,es                  # Multiple languages

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

	// Extract transcript
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Fetching transcript for %s...\n", videoID)
	opts := &youtube.ExtractOptions{
		Languages:         languages,
		Format:            "json3",
		SkipAutoGenerated: *skipAuto,
	}
	transcript, err := extractor.Extract(ctx, videoID, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching transcript: %v\n", err)
		os.Exit(1)
	}

	// Display result
	fmt.Printf("Video ID:      %s\n", transcript.VideoID)
	fmt.Printf("Language:      %s (%s)\n", transcript.Language, transcript.LanguageName)
	fmt.Printf("Auto-generated: %v\n", transcript.IsAutoGenerated)

	if transcript.DownloadURL != "" {
		fmt.Printf("\nTranscript available at: %s\n", transcript.DownloadURL)
		fmt.Println("\nTo download and process the transcript, use:")
		fmt.Printf("  curl %s | jq .\n", transcript.DownloadURL)
	} else {
		fmt.Println("\nNo transcript URL available (may not have captions)")
	}

	if len(transcript.Entries) > 0 {
		fmt.Println("\nTranscript entries:")
		for _, entry := range transcript.Entries {
			fmt.Printf("[%ds] %s\n", int(entry.Start), entry.Text)
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
