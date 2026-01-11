package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// VideoMetadata contains essential metadata about a YouTube video.
// This is typically fetched using yt-dlp and contains comprehensive information
// about a video that may not be available from RSS feeds or other sources.
type VideoMetadata struct {
	// ID is the YouTube video ID (e.g., "dQw4w9WgXcQ").
	ID string `json:"id"`
	// Title is the video title.
	Title string `json:"title"`
	// Description is the full video description.
	Description string `json:"description"`
	// Duration is the video length in seconds.
	Duration int `json:"duration"`
	// ViewCount is the total number of views.
	ViewCount int64 `json:"view_count"`
	// UploadDate is when the video was uploaded in YYYYMMDD format.
	UploadDate string `json:"upload_date"`
	// Uploader is the channel name/display name.
	Uploader string `json:"uploader"`
	// UploaderID is the channel ID.
	UploaderID string `json:"uploader_id"`
	// UploaderURL is the full channel URL.
	UploaderURL string `json:"uploader_url"`
	// ThumbnailURL is the URL to the best available thumbnail image.
	ThumbnailURL string `json:"thumbnail_url"`
	// Categories are the video's YouTube categories.
	Categories []string `json:"categories"`
	// Tags are the video's tags/keywords.
	Tags []string `json:"tags"`
	// IsLiveContent indicates whether this is a live stream or premiere.
	IsLiveContent bool `json:"is_live_content"`
	// FetchedAt is the timestamp when this metadata was retrieved.
	FetchedAt time.Time `json:"fetched_at"`
}

// FetchMetadata retrieves comprehensive metadata for a video using yt-dlp.
// It executes yt-dlp with JSON output and parses the result into a VideoMetadata struct.
// The provided context is used to enforce timeouts and handle cancellation.
func FetchMetadata(ctx context.Context, videoID string, ytdlpPath string) (*VideoMetadata, error) {
	// Run yt-dlp to get JSON metadata
	cmd := exec.CommandContext(ctx, ytdlpPath, "-J", "--no-warnings", videoID)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}

	// Parse the JSON output from yt-dlp
	var rawData map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &rawData); err != nil {
		return nil, fmt.Errorf("parse metadata JSON: %w", err)
	}

	// Extract and validate essential fields
	metadata := &VideoMetadata{
		FetchedAt: time.Now().UTC(),
	}

	// Required fields
	if id, ok := rawData["id"].(string); ok && id != "" {
		metadata.ID = id
	} else {
		return nil, fmt.Errorf("invalid metadata: missing or empty id")
	}

	if title, ok := rawData["title"].(string); ok && title != "" {
		metadata.Title = title
	} else {
		return nil, fmt.Errorf("invalid metadata: missing or empty title")
	}

	// Optional fields with type safety
	if desc, ok := rawData["description"].(string); ok {
		metadata.Description = desc
	}

	if duration, ok := rawData["duration"].(float64); ok {
		metadata.Duration = int(duration)
	}

	if views, ok := rawData["view_count"].(float64); ok {
		metadata.ViewCount = int64(views)
	}

	if date, ok := rawData["upload_date"].(string); ok {
		metadata.UploadDate = date
	}

	if uploader, ok := rawData["uploader"].(string); ok {
		metadata.Uploader = uploader
	}

	if uploaderID, ok := rawData["uploader_id"].(string); ok {
		metadata.UploaderID = uploaderID
	}

	if uploaderURL, ok := rawData["uploader_url"].(string); ok {
		metadata.UploaderURL = uploaderURL
	}

	// Thumbnail - prefer URL over object
	if thumb, ok := rawData["thumbnail"].(string); ok {
		metadata.ThumbnailURL = thumb
	}

	// Categories
	if cats, ok := rawData["categories"].([]interface{}); ok {
		metadata.Categories = make([]string, 0, len(cats))
		for _, cat := range cats {
			if s, ok := cat.(string); ok {
				metadata.Categories = append(metadata.Categories, s)
			}
		}
	}

	// Tags
	if tags, ok := rawData["tags"].([]interface{}); ok {
		metadata.Tags = make([]string, 0, len(tags))
		for _, tag := range tags {
			if s, ok := tag.(string); ok {
				metadata.Tags = append(metadata.Tags, s)
			}
		}
	}

	// Is live content
	if live, ok := rawData["is_live_content"].(bool); ok {
		metadata.IsLiveContent = live
	}

	// Validate we have at least the required fields
	if metadata.ID == "" || metadata.Title == "" {
		return nil, fmt.Errorf("invalid metadata: required fields missing")
	}

	return metadata, nil
}
