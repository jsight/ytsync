package innertube

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	ythttp "ytsync/http"
	"ytsync/retry"
	"ytsync/youtube"
)

var (
	// channelIDRegex matches YouTube channel IDs (UC followed by 22 chars).
	channelIDRegex = regexp.MustCompile(`UC[\w-]{22}`)
)

// Lister implements youtube.VideoLister using the Innertube API.
// It supports full channel history with continuation token-based pagination.
type Lister struct {
	client *Client

	// ContinuationState allows callers to resume pagination.
	// Set this before calling ListVideos to resume from a previous state.
	ContinuationState *ContinuationState
}

// ListerOption configures the Innertube lister.
type ListerOption func(*Lister)

// WithContinuationState sets initial continuation state for resuming.
func WithContinuationState(state *ContinuationState) ListerOption {
	return func(l *Lister) {
		l.ContinuationState = state
	}
}

// NewLister creates a new Innertube-based video lister.
func NewLister(httpClient *ythttp.Client, opts ...ListerOption) *Lister {
	l := &Lister{
		client: NewClient(httpClient),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// NewListerWithRetry creates a lister with custom retry configuration.
func NewListerWithRetry(httpClient *ythttp.Client, retryCfg retry.Config, opts ...ListerOption) *Lister {
	l := &Lister{
		client: NewClient(httpClient, WithRetryConfig(retryCfg)),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// ListVideos fetches videos from the specified channel using the Innertube API.
// It handles pagination automatically and respects MaxResults from options.
func (l *Lister) ListVideos(ctx context.Context, channelURL string, opts *youtube.ListOptions) ([]youtube.VideoInfo, error) {
	// Resolve channel ID from URL
	channelID, err := l.resolveChannelID(channelURL)
	if err != nil {
		return nil, &youtube.ListerError{
			Source:  "innertube",
			Channel: channelURL,
			Err:     err,
		}
	}

	// Initialize or use existing continuation state
	var state *ContinuationState
	if l.ContinuationState != nil && l.ContinuationState.ChannelID == channelID {
		state = l.ContinuationState
		// Check if token is expired
		if state.IsExpired() {
			state.Reset()
		}
	} else {
		state = NewContinuationState(channelID)
	}

	var allVideos []youtube.VideoInfo
	var channelName string
	maxResults := 0
	if opts != nil {
		maxResults = opts.MaxResults
	}

	// Pagination loop
	for {
		// Check context cancellation
		if ctx.Err() != nil {
			// Save state for potential resume
			l.ContinuationState = state
			return allVideos, ctx.Err()
		}

		// Check if we've reached the requested limit
		if maxResults > 0 && len(allVideos) >= maxResults {
			break
		}

		// Fetch a page
		resp, err := l.client.Browse(ctx, channelID, state.Token)
		if err != nil {
			// Save state for potential resume
			l.ContinuationState = state
			return nil, &youtube.ListerError{
				Source:  "innertube",
				Channel: channelURL,
				Err:     fmt.Errorf("browse request: %w", err),
			}
		}

		// Extract channel name from first response
		if channelName == "" {
			channelName = extractChannelName(resp)
		}

		// Extract videos from response
		videos := ExtractVideos(resp, channelID, channelName)
		for _, v := range videos {
			info := videoDataToInfo(v)

			// Apply published filter if specified
			if opts != nil && !opts.PublishedAfter.IsZero() {
				if info.Published.Before(opts.PublishedAfter) {
					// We've gone past the filter date, stop pagination
					// (videos are typically sorted by date, newest first)
					l.ContinuationState = state
					return filterAndSortVideos(allVideos, opts), nil
				}
			}

			allVideos = append(allVideos, info)

			// Update state with last video
			if len(videos) > 0 {
				state.LastVideoID = v.VideoID
			}
		}

		state.IncrementVideos(len(videos))

		// Get next continuation token
		nextToken := ExtractContinuationToken(resp)
		state.UpdateToken(nextToken, state.LastVideoID)

		// No more pages
		if !state.HasMore() {
			break
		}
	}

	// Save final state
	l.ContinuationState = state

	return filterAndSortVideos(allVideos, opts), nil
}

// SupportsFullHistory returns true - Innertube API can retrieve all videos.
func (l *Lister) SupportsFullHistory() bool {
	return true
}

// GetContinuationState returns the current continuation state for persistence.
func (l *Lister) GetContinuationState() *ContinuationState {
	return l.ContinuationState
}

// SetContinuationState sets the continuation state for resuming pagination.
func (l *Lister) SetContinuationState(state *ContinuationState) {
	l.ContinuationState = state
}

// resolveChannelID extracts or resolves a channel ID from various URL formats.
func (l *Lister) resolveChannelID(input string) (string, error) {
	// Check if it's already a channel ID
	if channelIDRegex.MatchString(input) {
		return channelIDRegex.FindString(input), nil
	}

	// Extract from /channel/ URL
	if strings.Contains(input, "youtube.com/channel/") {
		parts := strings.Split(input, "youtube.com/channel/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "/")[0]
			id = strings.Split(id, "?")[0]
			if channelIDRegex.MatchString(id) {
				return id, nil
			}
		}
	}

	// Handle @username format - we need to fetch the channel page to get the ID
	if strings.HasPrefix(input, "@") || strings.Contains(input, "youtube.com/@") {
		return "", fmt.Errorf("%w: handle resolution not yet implemented, use channel ID", youtube.ErrInvalidURL)
	}

	// Handle /c/ custom URL format
	if strings.Contains(input, "youtube.com/c/") {
		return "", fmt.Errorf("%w: custom URL resolution not yet implemented, use channel ID", youtube.ErrInvalidURL)
	}

	return "", fmt.Errorf("%w: cannot extract channel ID from %q", youtube.ErrInvalidURL, input)
}

// videoDataToInfo converts internal VideoData to youtube.VideoInfo.
func videoDataToInfo(v VideoData) youtube.VideoInfo {
	info := youtube.VideoInfo{
		ID:          v.VideoID,
		Title:       v.Title,
		Description: v.Description,
		ChannelID:   v.ChannelID,
		ChannelName: v.ChannelName,
		Thumbnail:   v.Thumbnail,
	}

	// Parse published time (e.g., "2 days ago", "3 weeks ago")
	if v.Published != "" {
		info.Published = parseRelativeTime(v.Published)
	}

	// Parse duration (e.g., "10:30", "1:23:45")
	if v.Duration != "" {
		info.Duration = parseDuration(v.Duration)
	}

	// Parse view count (e.g., "1.2M views", "500K views")
	if v.ViewCount != "" {
		info.ViewCount = parseViewCount(v.ViewCount)
	}

	return info
}

// parseRelativeTime converts relative time strings to absolute time.
func parseRelativeTime(s string) time.Time {
	s = strings.ToLower(strings.TrimSpace(s))

	now := time.Now()

	// Handle "Streamed X ago" format
	s = strings.TrimPrefix(s, "streamed ")

	// Common patterns
	patterns := []struct {
		suffix   string
		duration func(int) time.Duration
	}{
		{"second ago", func(n int) time.Duration { return time.Duration(n) * time.Second }},
		{"seconds ago", func(n int) time.Duration { return time.Duration(n) * time.Second }},
		{"minute ago", func(n int) time.Duration { return time.Duration(n) * time.Minute }},
		{"minutes ago", func(n int) time.Duration { return time.Duration(n) * time.Minute }},
		{"hour ago", func(n int) time.Duration { return time.Duration(n) * time.Hour }},
		{"hours ago", func(n int) time.Duration { return time.Duration(n) * time.Hour }},
		{"day ago", func(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }},
		{"days ago", func(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }},
		{"week ago", func(n int) time.Duration { return time.Duration(n) * 7 * 24 * time.Hour }},
		{"weeks ago", func(n int) time.Duration { return time.Duration(n) * 7 * 24 * time.Hour }},
		{"month ago", func(n int) time.Duration { return time.Duration(n) * 30 * 24 * time.Hour }},
		{"months ago", func(n int) time.Duration { return time.Duration(n) * 30 * 24 * time.Hour }},
		{"year ago", func(n int) time.Duration { return time.Duration(n) * 365 * 24 * time.Hour }},
		{"years ago", func(n int) time.Duration { return time.Duration(n) * 365 * 24 * time.Hour }},
	}

	for _, p := range patterns {
		if strings.HasSuffix(s, p.suffix) {
			numStr := strings.TrimSuffix(s, p.suffix)
			numStr = strings.TrimSpace(numStr)
			var n int
			if _, err := fmt.Sscanf(numStr, "%d", &n); err == nil {
				return now.Add(-p.duration(n))
			}
		}
	}

	return time.Time{}
}

// parseDuration converts duration strings like "10:30" or "1:23:45" to time.Duration.
func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")

	var hours, minutes, seconds int

	switch len(parts) {
	case 2:
		fmt.Sscanf(parts[0], "%d", &minutes)
		fmt.Sscanf(parts[1], "%d", &seconds)
	case 3:
		fmt.Sscanf(parts[0], "%d", &hours)
		fmt.Sscanf(parts[1], "%d", &minutes)
		fmt.Sscanf(parts[2], "%d", &seconds)
	default:
		return 0
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// parseViewCount converts view count strings like "1.2M views" to int64.
func parseViewCount(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, " views")
	s = strings.TrimSuffix(s, " view")
	s = strings.ReplaceAll(s, ",", "")

	var value float64
	var suffix string

	if _, err := fmt.Sscanf(s, "%f%s", &value, &suffix); err == nil {
		switch suffix {
		case "k":
			return int64(value * 1000)
		case "m":
			return int64(value * 1000000)
		case "b":
			return int64(value * 1000000000)
		}
	}

	// Try parsing as plain number
	var count int64
	if _, err := fmt.Sscanf(s, "%d", &count); err == nil {
		return count
	}

	return 0
}

// filterAndSortVideos applies filters and sorting from ListOptions.
func filterAndSortVideos(videos []youtube.VideoInfo, opts *youtube.ListOptions) []youtube.VideoInfo {
	if opts == nil {
		return videos
	}

	// Apply PublishedAfter filter
	if !opts.PublishedAfter.IsZero() {
		filtered := make([]youtube.VideoInfo, 0, len(videos))
		for _, v := range videos {
			if !v.Published.IsZero() && v.Published.After(opts.PublishedAfter) {
				filtered = append(filtered, v)
			}
		}
		videos = filtered
	}

	// Apply MaxResults limit
	if opts.MaxResults > 0 && len(videos) > opts.MaxResults {
		videos = videos[:opts.MaxResults]
	}

	return videos
}
