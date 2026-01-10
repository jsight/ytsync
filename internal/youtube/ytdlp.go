package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"ytsync/internal/retry"
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

	// RetryConfig holds retry behavior configuration.
	RetryConfig *retry.Config
}

// NewYtdlpLister creates a new yt-dlp based video lister.
func NewYtdlpLister() *YtdlpLister {
	cfg := retry.DefaultConfig()
	return &YtdlpLister{
		Path:        defaultYtdlpPath,
		Timeout:     defaultYtdlpTimeout,
		RetryConfig: &cfg,
	}
}

// ListVideos fetches all videos from the specified channel using yt-dlp.
func (y *YtdlpLister) ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error) {
	// Check if yt-dlp is installed
	if err := y.checkInstalled(ctx); err != nil {
		return nil, err
	}

	var videos []VideoInfo
	cfg := y.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	contentType := opts.ContentType
	if contentType == 0 {
		contentType = ContentTypeVideos
	}

	// If ContentTypeBoth, fetch both videos and streams
	if contentType == ContentTypeBoth {
		videosOpts := *opts
		videosOpts.ContentType = ContentTypeVideos
		videosList, err := y.ListVideos(ctx, channelURL, &videosOpts)
		if err != nil {
			return nil, err
		}

		streamsOpts := *opts
		streamsOpts.ContentType = ContentTypeStreams
		streamsList, err := y.ListVideos(ctx, channelURL, &streamsOpts)
		if err != nil {
			return nil, err
		}

		videos = append(videosList, streamsList...)
		return videos, nil
	}

	err := retry.Do(ctx, *cfg, ytdlpErrorClassifier, func(ctx context.Context) error {
		// Build the URL for the videos or streams tab
		url := normalizeChannelURL(channelURL, contentType)

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
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, y.path(), args...)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			if cmdCtx.Err() == context.DeadlineExceeded {
				return &ListerError{Source: "ytdlp", Channel: channelURL, Err: ErrNetworkTimeout}
			}
			if cmdCtx.Err() == context.Canceled {
				return &ListerError{Source: "ytdlp", Channel: channelURL, Err: context.Canceled}
			}

			// Check for common error patterns in stderr
			errMsg := stderr.String()
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
				return &ListerError{Source: "ytdlp", Channel: channelURL, Err: ErrChannelNotFound}
			}
			if strings.Contains(errMsg, "rate") || strings.Contains(errMsg, "429") {
				return &ListerError{Source: "ytdlp", Channel: channelURL, Err: ErrRateLimited}
			}

			return &ListerError{Source: "ytdlp", Channel: channelURL,
				Err: fmt.Errorf("yt-dlp failed: %w: %s", err, errMsg)}
		}

		// Parse JSON output
		parsedVideos, parseErr := parseYtdlpOutput(stdout.Bytes(), contentType)
		if parseErr != nil {
			return parseErr
		}
		videos = parsedVideos
		return nil
	})

	if err != nil {
		return nil, err
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

// normalizeChannelURL ensures the URL points to the correct tab (videos or streams).
func normalizeChannelURL(url string, contentType ContentType) string {
	tab := "videos"
	if contentType == ContentTypeStreams {
		tab = "streams"
	}

	// If it's just a channel ID, construct full URL
	if channelIDRegex.MatchString(url) && !strings.Contains(url, "youtube.com") {
		return "https://www.youtube.com/channel/" + url + "/" + tab
	}

	// Replace /videos or /streams with the desired tab
	if strings.Contains(url, "/videos") {
		url = strings.Replace(url, "/videos", "/"+tab, 1)
	} else if strings.Contains(url, "/streams") {
		url = strings.Replace(url, "/streams", "/"+tab, 1)
	} else {
		// Ensure we're pointing to the correct tab
		url = strings.TrimSuffix(url, "/")
		url = url + "/" + tab
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
func parseYtdlpOutput(data []byte, contentType ContentType) ([]VideoInfo, error) {
	var playlist ytdlpPlaylist
	if err := json.Unmarshal(data, &playlist); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}

	videoType := "video"
	if contentType == ContentTypeStreams {
		videoType = "stream"
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
			Type:        videoType,
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

// ytdlpErrorClassifier determines if an yt-dlp error is retryable.
func ytdlpErrorClassifier(err error) bool {
	if err == nil {
		return false
	}

	// Permanent errors - don't retry
	var listerErr *ListerError
	if errors.As(err, &listerErr) {
		switch listerErr.Err {
		case ErrChannelNotFound:
			return false
		default:
			// Retryable: rate limit, timeout, network errors
			return true
		}
	}

	// Default to retryable for unknown errors
	return true
}
