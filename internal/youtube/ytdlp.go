package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultYtdlpPath    = "yt-dlp"
	defaultYtdlpTimeout = 10 * time.Minute
)

// YtdlpLister implements VideoLister using yt-dlp as a subprocess.
// This can retrieve the full video history of a channel.
type YtdlpLister struct {
	// Path is the path to the yt-dlp executable. Defaults to "yt-dlp".
	Path string

	// Timeout is the maximum time to wait for yt-dlp. Defaults to 10 minutes.
	Timeout time.Duration

	// ExtraArgs are additional arguments to pass to yt-dlp.
	ExtraArgs []string
}

// NewYtdlpLister creates a new yt-dlp based video lister.
func NewYtdlpLister() *YtdlpLister {
	return &YtdlpLister{
		Path:    defaultYtdlpPath,
		Timeout: defaultYtdlpTimeout,
	}
}

// ListVideos fetches all videos from the specified channel using yt-dlp.
func (y *YtdlpLister) ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error) {
	// Check if yt-dlp is installed
	if err := y.checkInstalled(ctx); err != nil {
		return nil, err
	}

	// Build the URL for the videos tab
	url := normalizeChannelURL(channelURL)

	// Build arguments
	args := []string{
		"--flat-playlist",
		"-J", // JSON output
		"--no-warnings",
	}

	// Add sorting if specified
	if opts != nil && opts.SortOrder == SortByPopularity {
		args = append(args, "--playlist-items", "1-")
		url = strings.TrimSuffix(url, "/videos") + "/videos?view=0&sort=p"
	}

	// Add extra args
	args = append(args, y.ExtraArgs...)
	args = append(args, url)

	// Create command with timeout
	timeout := y.Timeout
	if timeout == 0 {
		timeout = defaultYtdlpTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, y.path(), args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &ListerError{Source: "ytdlp", Channel: channelURL, Err: ErrNetworkTimeout}
		}
		if ctx.Err() == context.Canceled {
			return nil, &ListerError{Source: "ytdlp", Channel: channelURL, Err: context.Canceled}
		}

		// Check for common error patterns in stderr
		errMsg := stderr.String()
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
			return nil, &ListerError{Source: "ytdlp", Channel: channelURL, Err: ErrChannelNotFound}
		}
		if strings.Contains(errMsg, "rate") || strings.Contains(errMsg, "429") {
			return nil, &ListerError{Source: "ytdlp", Channel: channelURL, Err: ErrRateLimited}
		}

		return nil, &ListerError{Source: "ytdlp", Channel: channelURL,
			Err: fmt.Errorf("yt-dlp failed: %w: %s", err, errMsg)}
	}

	// Parse JSON output
	videos, err := parseYtdlpOutput(stdout.Bytes())
	if err != nil {
		return nil, &ListerError{Source: "ytdlp", Channel: channelURL, Err: err}
	}

	// Apply filters
	if opts != nil {
		videos = filterVideos(videos, opts)
	}

	return videos, nil
}

// SupportsFullHistory returns true - yt-dlp can retrieve all videos.
func (y *YtdlpLister) SupportsFullHistory() bool {
	return true
}

// checkInstalled verifies that yt-dlp is available.
func (y *YtdlpLister) checkInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, y.path(), "--version")
	if err := cmd.Run(); err != nil {
		return &ListerError{Source: "ytdlp", Channel: "", Err: ErrYtdlpNotInstalled}
	}
	return nil
}

func (y *YtdlpLister) path() string {
	if y.Path != "" {
		return y.Path
	}
	return defaultYtdlpPath
}

// normalizeChannelURL ensures the URL points to the videos tab.
func normalizeChannelURL(url string) string {
	// If it's just a channel ID, construct full URL
	if channelIDRegex.MatchString(url) && !strings.Contains(url, "youtube.com") {
		return "https://www.youtube.com/channel/" + url + "/videos"
	}

	// Ensure we're pointing to the videos tab
	if !strings.Contains(url, "/videos") {
		url = strings.TrimSuffix(url, "/")
		url = url + "/videos"
	}

	return url
}

// ytdlpPlaylist represents yt-dlp's JSON output for a playlist/channel.
type ytdlpPlaylist struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Uploader    string        `json:"uploader"`
	UploaderID  string        `json:"uploader_id"`
	ChannelID   string        `json:"channel_id"`
	ChannelURL  string        `json:"channel_url"`
	Entries     []ytdlpEntry  `json:"entries"`
	Description string        `json:"description"`
}

// ytdlpEntry represents a single video in yt-dlp's JSON output.
type ytdlpEntry struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Duration     float64 `json:"duration"` // seconds
	ViewCount    int64   `json:"view_count"`
	Uploader     string  `json:"uploader"`
	UploaderID   string  `json:"uploader_id"`
	ChannelID    string  `json:"channel_id"`
	UploadDate   string  `json:"upload_date"` // YYYYMMDD format
	Timestamp    int64   `json:"timestamp"`   // Unix timestamp
	Thumbnail    string  `json:"thumbnail"`
	Thumbnails   []ytdlpThumbnail `json:"thumbnails"`
}

type ytdlpThumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// parseYtdlpOutput parses yt-dlp's JSON output into VideoInfo slice.
func parseYtdlpOutput(data []byte) ([]VideoInfo, error) {
	var playlist ytdlpPlaylist
	if err := json.Unmarshal(data, &playlist); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}

	videos := make([]VideoInfo, 0, len(playlist.Entries))
	for _, entry := range playlist.Entries {
		video := VideoInfo{
			ID:          entry.ID,
			Title:       entry.Title,
			ChannelID:   coalesce(entry.ChannelID, playlist.ChannelID),
			ChannelName: coalesce(entry.Uploader, playlist.Uploader),
			Duration:    time.Duration(entry.Duration) * time.Second,
			Description: entry.Description,
			ViewCount:   entry.ViewCount,
			Thumbnail:   bestThumbnail(entry),
			Published:   parseYtdlpDate(entry),
		}
		videos = append(videos, video)
	}

	return videos, nil
}

// parseYtdlpDate extracts the published time from a yt-dlp entry.
func parseYtdlpDate(entry ytdlpEntry) time.Time {
	// Prefer timestamp if available
	if entry.Timestamp > 0 {
		return time.Unix(entry.Timestamp, 0).UTC()
	}

	// Fall back to upload_date (YYYYMMDD)
	if entry.UploadDate != "" {
		t, err := time.Parse("20060102", entry.UploadDate)
		if err == nil {
			return t
		}
	}

	return time.Time{}
}

// bestThumbnail returns the best quality thumbnail URL.
func bestThumbnail(entry ytdlpEntry) string {
	// Return direct thumbnail if set
	if entry.Thumbnail != "" {
		return entry.Thumbnail
	}

	// Find highest resolution from thumbnails array
	var best ytdlpThumbnail
	for _, t := range entry.Thumbnails {
		if t.Width*t.Height > best.Width*best.Height {
			best = t
		}
	}
	return best.URL
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
