// Package youtube provides YouTube video listing and metadata extraction.
package youtube

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for video listing operations.
var (
	ErrChannelNotFound   = errors.New("youtube: channel not found")
	ErrRateLimited       = errors.New("youtube: rate limited")
	ErrNetworkTimeout    = errors.New("youtube: network timeout")
	ErrInvalidURL        = errors.New("youtube: invalid URL")
	ErrYtdlpNotInstalled = errors.New("youtube: yt-dlp not installed")
)

// VideoLister defines the interface for fetching video lists from YouTube channels.
// Different implementations may use different strategies (RSS, yt-dlp, API).
type VideoLister interface {
	// ListVideos fetches videos from the specified channel URL.
	// The URL can be a channel URL, handle (@username), or channel ID.
	ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error)

	// SupportsFullHistory returns true if this lister can retrieve all videos,
	// not just recent ones. RSS feeds return false, yt-dlp returns true.
	SupportsFullHistory() bool
}

// ListOptions configures video listing behavior.
type ListOptions struct {
	// MaxResults limits the number of videos returned. 0 means no limit.
	MaxResults int

	// PublishedAfter filters videos to only those published after this time.
	// Zero time means no filter.
	PublishedAfter time.Time

	// SortOrder specifies how videos should be sorted.
	// Default is SortByDate (newest first).
	SortOrder SortOrder
}

// SortOrder specifies how videos should be sorted.
type SortOrder int

const (
	// SortByDate sorts videos by publication date, newest first.
	SortByDate SortOrder = iota
	// SortByPopularity sorts videos by view count, highest first.
	SortByPopularity
)

// VideoInfo contains metadata about a YouTube video.
type VideoInfo struct {
	// ID is the YouTube video ID (e.g., "dQw4w9WgXcQ").
	ID string `json:"id"`

	// Title is the video title.
	Title string `json:"title"`

	// ChannelID is the YouTube channel ID (e.g., "UCuAXFkgsw1L7xaCfnd5JJOw").
	ChannelID string `json:"channel_id"`

	// ChannelName is the display name of the channel.
	ChannelName string `json:"channel_name"`

	// Published is when the video was published.
	Published time.Time `json:"published"`

	// Duration is the video length. May be zero for some sources like RSS.
	Duration time.Duration `json:"duration,omitempty"`

	// Description is the video description. May be truncated by some sources.
	Description string `json:"description,omitempty"`

	// Thumbnail is the URL to the video thumbnail image.
	Thumbnail string `json:"thumbnail,omitempty"`

	// ViewCount is the number of views. May be zero if not available.
	ViewCount int64 `json:"view_count,omitempty"`
}

// VideoURL returns the full YouTube URL for this video.
func (v VideoInfo) VideoURL() string {
	return "https://www.youtube.com/watch?v=" + v.ID
}

// ChannelURL returns the full YouTube URL for this video's channel.
func (v VideoInfo) ChannelURL() string {
	return "https://www.youtube.com/channel/" + v.ChannelID
}

// ListerError wraps errors with context about the listing operation.
type ListerError struct {
	Source  string // Source of error: "rss", "ytdlp", "api"
	Channel string // Channel URL or ID being listed
	Err     error  // Underlying error
}

func (e *ListerError) Error() string {
	return "youtube: " + e.Source + " listing " + e.Channel + ": " + e.Err.Error()
}

func (e *ListerError) Unwrap() error { return e.Err }
