package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"
	"ytsync/internal/config"
	"ytsync/internal/youtube"
)

func main() {
	// Parse flags
	useRSS := flag.Bool("rss", false, "Use RSS feed instead of yt-dlp for listing")
	maxVideos := flag.Int("max", 0, "Maximum videos to list (0 = all)")
	since := flag.String("since", "", "Only videos published after this date (RFC3339, e.g., 2024-01-15T00:00:00Z)")
	contentTypeStr := flag.String("type", "videos", "Content type: videos, streams, or both")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ytsync [flags] <youtube-url>\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  ytsync https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw\n")
		fmt.Fprintf(os.Stderr, "  ytsync --max 10 https://www.youtube.com/c/channelname\n")
		fmt.Fprintf(os.Stderr, "  ytsync --rss UCuAXFkgsw1L7xaCfnd5JJOw\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	channelURL := args[0]

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
	fmt.Fprintln(w, "VIDEO ID\tTITLE\tPUBLISHED\tDURATION\tVIEWS\tTYPE")

	for _, v := range videos {
		duration := ""
		if v.Duration > 0 {
			duration = fmt.Sprintf("%d:%02d", int(v.Duration.Minutes()), int(v.Duration.Seconds())%60)
		}

		views := ""
		if v.ViewCount > 0 {
			views = fmt.Sprintf("%d", v.ViewCount)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			v.ID,
			truncate(v.Title, 50),
			v.Published.Format("2006-01-02"),
			duration,
			views,
			v.Type,
		)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\nTotal: %d videos\n", len(videos))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
