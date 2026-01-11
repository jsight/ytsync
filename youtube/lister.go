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

	// ContentType specifies what type of content to list.
	// Default is ContentTypeVideos.
	ContentType ContentType

	// --- Resumable Pagination Options ---

	// ResumeToken is an opaque token for resuming pagination.
	// For YouTube Data API v3, this is the pageToken.
	// For Innertube, this is the continuation token.
	ResumeToken string

	// ResumePlaylistID is the uploads playlist ID for API-based listing.
	// When provided, skips the playlist ID lookup (saves quota).
	ResumePlaylistID string

	// OnProgress is called after each page of results is fetched.
	// It receives the current pagination state and any error that occurred.
	// Return a non-nil error to stop pagination.
	OnProgress func(state *PaginationProgress) error
}

// PaginationProgress reports the current state of paginated listing.
// This is passed to the OnProgress callback for state persistence.
type PaginationProgress struct {
	// Token is the next page token (empty if pagination complete).
	Token string
	// PlaylistID is the uploads playlist ID (API lister only).
	PlaylistID string
	// VideosRetrieved is the total count of videos fetched so far.
	VideosRetrieved int
	// LastVideoID is the ID of the last video retrieved.
	LastVideoID string
	// QuotaUsed is the estimated quota consumed (API lister only).
	QuotaUsed int
	// Complete is true if pagination has finished.
	Complete bool
	// Error is non-nil if pagination stopped due to an error.
	Error error
}

// SortOrder specifies how videos should be sorted.
type SortOrder int

const (
	// SortByDate sorts videos by publication date, newest first.
	SortByDate SortOrder = iota
	// SortByPopularity sorts videos by view count, highest first.
	SortByPopularity
)

// ContentType specifies what type of content to list.
type ContentType int

const (
	// ContentTypeVideos lists regular videos.
	ContentTypeVideos ContentType = iota
	// ContentTypeStreams lists live streams.
	ContentTypeStreams
	// ContentTypeBoth lists both videos and streams.
	ContentTypeBoth
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

	// Duration is the video length. May be zero for some sources like RSS or live streams.
	Duration time.Duration `json:"duration,omitempty"`

	// Description is the video description. May be truncated by some sources.
	Description string `json:"description,omitempty"`

	// Thumbnail is the URL to the video thumbnail image.
	Thumbnail string `json:"thumbnail,omitempty"`

	// ViewCount is the number of views. May be zero if not available.
	ViewCount int64 `json:"view_count,omitempty"`

	// Type indicates whether this is a video or live stream.
	Type string `json:"type,omitempty"`
}

// VideoURL returns the full YouTube URL for this video.
func (v VideoInfo) VideoURL() string {
	return "https://www.youtube.com/watch?v=" + v.ID
}

// ChannelURL returns the full YouTube URL for this video's channel.
func (v VideoInfo) ChannelURL() string {
	return "https://www.youtube.com/channel/" + v.ChannelID
}

// ListerError wraps listing errors with context about what failed.
// Use errors.As() to extract this error type and get operation details:
//
//	var listerErr *youtube.ListerError
//	if errors.As(err, &listerErr) {
//		fmt.Printf("Failed to list from %s: %v\n", listerErr.Source, listerErr.Err)
//	}
type ListerError struct {
	// Source indicates which lister produced the error ("rss", "ytdlp", "api").
	Source string
	// Channel is the channel URL or ID that was being listed.
	Channel string
	// Err is the underlying error that occurred.
	Err error
}

// Error returns a string representation of the listing error.
func (e *ListerError) Error() string {
	return "youtube: " + e.Source + " listing " + e.Channel + ": " + e.Err.Error()
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *ListerError) Unwrap() error { return e.Err }
