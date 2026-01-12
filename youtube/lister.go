// Package youtube provides YouTube video listing and metadata extraction.
package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
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

// ChannelResolver resolves YouTube channel handles and custom URLs to channel IDs.
type ChannelResolver struct {
	// HTTPClient is the HTTP client to use for requests.
	// If nil, a default client with 30-second timeout is used.
	HTTPClient HTTPDoer
}

// HTTPDoer is an interface for making HTTP requests.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewChannelResolver creates a new channel resolver.
func NewChannelResolver() *ChannelResolver {
	return &ChannelResolver{}
}

// ResolveChannelID resolves a channel URL, handle, or custom URL to a channel ID.
// Supports:
//   - Channel ID directly: UCsBjURrPoezykLs9EqgamOA
//   - Channel URL: https://www.youtube.com/channel/UCsBjURrPoezykLs9EqgamOA
//   - Handle: @Fireship or https://www.youtube.com/@Fireship
//   - Custom URL: https://www.youtube.com/c/Fireship
func (r *ChannelResolver) ResolveChannelID(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)

	// Try extracting channel ID directly first
	if id := extractChannelIDDirect(input); id != "" {
		return id, nil
	}

	// Need to fetch the page to resolve handles/custom URLs
	pageURL := toFetchableURL(input)
	if pageURL == "" {
		return "", fmt.Errorf("%w: cannot parse %q", ErrInvalidURL, input)
	}

	return r.fetchChannelID(ctx, pageURL)
}

// extractChannelIDDirect extracts channel ID without making HTTP requests.
func extractChannelIDDirect(input string) string {
	// Regex for channel ID: UC + 22 base64 chars
	channelIDRegex := regexp.MustCompile(`UC[a-zA-Z0-9_-]{22}`)

	// Direct channel ID
	if channelIDRegex.MatchString(input) {
		return channelIDRegex.FindString(input)
	}

	// Channel URL: https://www.youtube.com/channel/UCxxxxx
	if strings.Contains(input, "youtube.com/channel/") {
		parts := strings.Split(input, "youtube.com/channel/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "/")[0]
			id = strings.Split(id, "?")[0]
			if channelIDRegex.MatchString(id) {
				return id
			}
		}
	}

	return ""
}

// toFetchableURL converts various input formats to a fetchable URL for handle resolution.
func toFetchableURL(input string) string {
	// Handle @username format
	if strings.HasPrefix(input, "@") {
		return "https://www.youtube.com/" + input
	}

	// Already a full URL
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input
	}

	// Bare youtube.com URL
	if strings.HasPrefix(input, "youtube.com") {
		return "https://" + input
	}

	return ""
}

// fetchChannelID fetches a channel page and extracts the channel ID.
func (r *ChannelResolver) fetchChannelID(ctx context.Context, pageURL string) (string, error) {
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Set browser-like headers to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ErrNetworkTimeout
		}
		return "", fmt.Errorf("fetch channel page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrChannelNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read the body (limit to 1MB to avoid memory issues)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Extract channel ID from various locations in the HTML
	channelID := extractChannelIDFromHTML(string(body))
	if channelID == "" {
		return "", fmt.Errorf("%w: could not find channel ID in page", ErrInvalidURL)
	}

	return channelID, nil
}

// extractChannelIDFromHTML extracts the channel ID from YouTube HTML.
func extractChannelIDFromHTML(html string) string {
	// Pattern 1: <meta itemprop="channelId" content="UCxxxxx">
	if idx := strings.Index(html, `"channelId"`); idx != -1 {
		// Look for the channel ID pattern near this location
		searchArea := html[idx:min(idx+200, len(html))]
		channelIDRegex := regexp.MustCompile(`UC[a-zA-Z0-9_-]{22}`)
		if match := channelIDRegex.FindString(searchArea); match != "" {
			return match
		}
	}

	// Pattern 2: "externalId":"UCxxxxx"
	if idx := strings.Index(html, `"externalId":"`); idx != -1 {
		start := idx + len(`"externalId":"`)
		end := strings.Index(html[start:], `"`)
		if end != -1 && end <= 24 {
			candidate := html[start : start+end]
			if strings.HasPrefix(candidate, "UC") && len(candidate) == 24 {
				return candidate
			}
		}
	}

	// Pattern 3: /channel/UCxxxxx in canonical URL or links
	channelIDRegex := regexp.MustCompile(`/channel/(UC[a-zA-Z0-9_-]{22})`)
	if matches := channelIDRegex.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1]
	}

	// Pattern 4: "browseId":"UCxxxxx"
	if idx := strings.Index(html, `"browseId":"UC`); idx != -1 {
		start := idx + len(`"browseId":"`)
		if start+24 <= len(html) {
			candidate := html[start : start+24]
			if strings.HasPrefix(candidate, "UC") {
				return candidate
			}
		}
	}

	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
