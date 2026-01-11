// Package ytsync provides a library for synchronizing YouTube content.
//
// It enables programmatic access to YouTube videos, transcripts, and metadata.
//
// Overview
//
// ytsync provides high-level convenience functions for the most common operations:
//
//   - ListVideos: Fetch videos from a YouTube channel
//   - ExtractTranscript: Get transcript for a video
//   - FetchVideoMetadata: Retrieve comprehensive video metadata
//
// Quick Start
//
// List videos from a channel:
//
//	ctx := context.Background()
//	videos, err := ytsync.ListVideos(ctx, "https://www.youtube.com/channel/UCxxxxx")
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, v := range videos {
//		fmt.Println(v.Title)
//	}
//
// Extract a transcript:
//
//	transcript, err := ytsync.ExtractTranscript(ctx, "dQw4w9WgXcQ")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(transcript.Content)
//
// Get video metadata:
//
//	metadata, err := ytsync.FetchVideoMetadata(ctx, "dQw4w9WgXcQ")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Title: %s\nViews: %d\n", metadata.Title, metadata.ViewCount)
//
// Configuration
//
// ytsync uses a configuration system that loads settings from multiple sources:
//
//   1. Environment variables (highest priority)
//   2. Config file (ytsync.json or ~/.config/ytsync/ytsync.json)
//   3. Default values (lowest priority)
//
// Environment variables:
//
//   - YTSYNC_YTDLP_PATH: Path to yt-dlp executable
//   - YTSYNC_YTDLP_TIMEOUT: Timeout for yt-dlp operations
//   - YTSYNC_MAX_VIDEOS: Maximum videos to retrieve
//   - YTSYNC_INCLUDE_SHORTS: Include YouTube Shorts (true/false)
//   - YTSYNC_INCLUDE_LIVE: Include live streams (true/false)
//   - YTSYNC_MAX_RETRIES: Maximum retry attempts
//   - YTSYNC_INITIAL_BACKOFF: Initial retry backoff duration
//   - YTSYNC_MAX_BACKOFF: Maximum retry backoff duration
//
// Error Handling
//
// All operations return errors that implement standard Go error handling:
//
// Checking for sentinel errors:
//
//	if errors.Is(err, ytsync.ErrChannelNotFound) {
//		fmt.Println("Channel not found")
//	}
//
// Extracting wrapped error details:
//
//	var listerErr *ytsync.ListerError
//	if errors.As(err, &listerErr) {
//		fmt.Printf("Listing %s failed: %v\n", listerErr.Channel, listerErr.Err)
//	}
//
// Advanced Usage
//
// For more control, use the sub-packages directly:
//
//   - youtube: Video listing, metadata, and transcript extraction
//   - config: Configuration management
//   - storage: Persistent data storage
//   - retry: Exponential backoff retry logic
//
// Example using youtube package directly:
//
//	ytdlp := youtube.NewYtdlpLister()
//	ytdlp.Path = "/usr/bin/yt-dlp"
//	videos, err := ytdlp.ListVideos(ctx, channelURL, &youtube.ListOptions{
//		MaxResults:  100,
//		ContentType: youtube.ContentTypeBoth,
//	})
//
// Dependencies
//
// ytsync requires yt-dlp to be installed and available in PATH or specified via
// YTSYNC_YTDLP_PATH environment variable.
//
// Install yt-dlp: https://github.com/yt-dlp/yt-dlp
//
package ytsync
