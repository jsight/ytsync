package youtube

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"ytsync/retry"
)

const (
	rssFeedURLTemplate = "https://www.youtube.com/feeds/videos.xml?channel_id=%s"
	defaultTimeout     = 30 * time.Second
)

// RSSLister implements VideoLister using YouTube's RSS/Atom feeds.
// RSS feeds only return the 15 most recent videos, so this is best
// suited for incremental sync after an initial full sync.
type RSSLister struct {
	client      *http.Client
	RetryConfig *retry.Config
}

// NewRSSLister creates a new RSS-based video lister.
func NewRSSLister() *RSSLister {
	cfg := retry.DefaultConfig()
	return &RSSLister{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		RetryConfig: &cfg,
	}
}

// NewRSSListerWithClient creates a new RSS lister with a custom HTTP client.
func NewRSSListerWithClient(client *http.Client) *RSSLister {
	return &RSSLister{client: client}
}

// ListVideos fetches videos from the YouTube RSS feed.
// The channelURL must contain a channel ID (UC...) - handles are not supported.
func (r *RSSLister) ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error) {
	channelID, err := extractChannelID(channelURL)
	if err != nil {
		return nil, &ListerError{Source: "rss", Channel: channelURL, Err: err}
	}

	var videos []VideoInfo
	cfg := r.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	err = retry.Do(ctx, *cfg, rssErrorClassifier, func(ctx context.Context) error {
		feedURL := fmt.Sprintf(rssFeedURLTemplate, channelID)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
		if err != nil {
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}

		resp, err := r.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return &ListerError{Source: "rss", Channel: channelURL, Err: ErrNetworkTimeout}
			}
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return &ListerError{Source: "rss", Channel: channelURL, Err: ErrChannelNotFound}
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return &ListerError{Source: "rss", Channel: channelURL, Err: ErrRateLimited}
		}
		if resp.StatusCode != http.StatusOK {
			return &ListerError{Source: "rss", Channel: channelURL,
				Err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}

		feed, err := parseAtomFeed(body)
		if err != nil {
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}

		videos = feedToVideoInfo(feed, channelID)
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

// SupportsFullHistory returns false - RSS only provides the 15 most recent videos.
func (r *RSSLister) SupportsFullHistory() bool {
	return false
}

// RSSIncrementalResult contains the result of an incremental RSS sync.
type RSSIncrementalResult struct {
	// Videos is the list of new videos (after filtering by last sync time).
	Videos []VideoInfo
	// NewestTimestamp is the published time of the most recent video in the feed.
	NewestTimestamp time.Time
	// OldestTimestamp is the published time of the oldest video in the feed.
	OldestTimestamp time.Time
	// GapDetected is true if the oldest video in the feed is newer than the last sync,
	// indicating that videos may have been missed and a full sync is recommended.
	GapDetected bool
	// TotalInFeed is the total number of videos in the RSS feed (typically 15).
	TotalInFeed int
	// NewVideosCount is the number of videos newer than the last sync time.
	NewVideosCount int
}

// ListVideosIncremental performs an incremental sync using the RSS feed.
// It detects gaps (missed videos) and returns metadata about the sync.
//
// Parameters:
//   - lastSyncTime: The time of the last successful sync. Pass zero time for first sync.
//
// Returns RSSIncrementalResult with gap detection and video list.
func (r *RSSLister) ListVideosIncremental(ctx context.Context, channelURL string, lastSyncTime time.Time, opts *ListOptions) (*RSSIncrementalResult, error) {
	channelID, err := extractChannelID(channelURL)
	if err != nil {
		return nil, &ListerError{Source: "rss", Channel: channelURL, Err: err}
	}

	var videos []VideoInfo
	cfg := r.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	err = retry.Do(ctx, *cfg, rssErrorClassifier, func(ctx context.Context) error {
		feedURL := fmt.Sprintf(rssFeedURLTemplate, channelID)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
		if err != nil {
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}

		resp, err := r.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return &ListerError{Source: "rss", Channel: channelURL, Err: ErrNetworkTimeout}
			}
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return &ListerError{Source: "rss", Channel: channelURL, Err: ErrChannelNotFound}
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return &ListerError{Source: "rss", Channel: channelURL, Err: ErrRateLimited}
		}
		if resp.StatusCode != http.StatusOK {
			return &ListerError{Source: "rss", Channel: channelURL,
				Err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}

		feed, err := parseAtomFeed(body)
		if err != nil {
			return &ListerError{Source: "rss", Channel: channelURL, Err: err}
		}

		videos = feedToVideoInfo(feed, channelID)
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Calculate timestamps from all videos in feed
	var newestTimestamp, oldestTimestamp time.Time
	if len(videos) > 0 {
		newestTimestamp = videos[0].Published
		oldestTimestamp = videos[0].Published
		for _, v := range videos {
			if v.Published.After(newestTimestamp) {
				newestTimestamp = v.Published
			}
			if v.Published.Before(oldestTimestamp) {
				oldestTimestamp = v.Published
			}
		}
	}

	// Detect gap: if oldest video in RSS is newer than last sync, we may have missed videos
	gapDetected := false
	if !lastSyncTime.IsZero() && !oldestTimestamp.IsZero() {
		// If the oldest video in the RSS feed is significantly newer than our last sync,
		// it means videos between lastSync and oldestTimestamp may have been pushed out of the feed.
		// We add a small grace period (1 minute) to account for timing differences.
		gracePeriod := 1 * time.Minute
		if oldestTimestamp.After(lastSyncTime.Add(gracePeriod)) {
			gapDetected = true
		}
	}

	// Filter videos to only those after lastSyncTime
	totalInFeed := len(videos)
	var newVideos []VideoInfo
	if !lastSyncTime.IsZero() {
		for _, v := range videos {
			if v.Published.After(lastSyncTime) {
				newVideos = append(newVideos, v)
			}
		}
	} else {
		// First sync - return all videos
		newVideos = videos
	}

	// Apply additional filters from opts
	if opts != nil {
		newVideos = filterVideos(newVideos, opts)
	}

	return &RSSIncrementalResult{
		Videos:          newVideos,
		NewestTimestamp: newestTimestamp,
		OldestTimestamp: oldestTimestamp,
		GapDetected:     gapDetected,
		TotalInFeed:     totalInFeed,
		NewVideosCount:  len(newVideos),
	}, nil
}

// ShouldTriggerFullSync returns true if the RSS sync indicates a full sync is needed.
// This happens when:
// 1. A gap is detected (missed videos)
// 2. No videos are returned but sync state indicates there should be videos
// 3. This is the first sync (lastSyncTime is zero)
func (r *RSSIncrementalResult) ShouldTriggerFullSync() bool {
	if r == nil {
		return true
	}
	return r.GapDetected
}

// atomFeed represents a YouTube Atom feed structure.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Author  atomAuthor  `xml:"author"`
	Entries []atomEntry `xml:"entry"`
}

type atomAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

type atomEntry struct {
	ID          string        `xml:"id"`
	VideoID     string        `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID   string        `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
	Title       string        `xml:"title"`
	Published   time.Time     `xml:"published"`
	Updated     time.Time     `xml:"updated"`
	Description string        `xml:"group>description"`
	Thumbnail   atomThumbnail `xml:"group>thumbnail"`
	Community   atomCommunity `xml:"group>community"`
}

type atomThumbnail struct {
	URL    string `xml:"url,attr"`
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
}

type atomCommunity struct {
	Views atomViews `xml:"http://search.yahoo.com/mrss/ statistics"`
}

type atomViews struct {
	Views int64 `xml:"views,attr"`
}

// parseAtomFeed parses YouTube's Atom XML feed.
func parseAtomFeed(data []byte) (*atomFeed, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse atom feed: %w", err)
	}
	return &feed, nil
}

// feedToVideoInfo converts an Atom feed to VideoInfo slice.
func feedToVideoInfo(feed *atomFeed, channelID string) []VideoInfo {
	videos := make([]VideoInfo, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		video := VideoInfo{
			ID:          entry.VideoID,
			Title:       entry.Title,
			ChannelID:   channelID,
			ChannelName: feed.Author.Name,
			Published:   entry.Published,
			Description: entry.Description,
			Thumbnail:   entry.Thumbnail.URL,
			ViewCount:   entry.Community.Views.Views,
			// Duration not available in RSS feed
		}
		videos = append(videos, video)
	}
	return videos
}

// filterVideos applies ListOptions filters to the video list.
func filterVideos(videos []VideoInfo, opts *ListOptions) []VideoInfo {
	if opts == nil {
		return videos
	}

	// Filter by PublishedAfter
	if !opts.PublishedAfter.IsZero() {
		filtered := make([]VideoInfo, 0, len(videos))
		for _, v := range videos {
			if v.Published.After(opts.PublishedAfter) {
				filtered = append(filtered, v)
			}
		}
		videos = filtered
	}

	// Apply MaxResults
	if opts.MaxResults > 0 && len(videos) > opts.MaxResults {
		videos = videos[:opts.MaxResults]
	}

	return videos
}

// channelIDRegex matches YouTube channel IDs (UC followed by 22 base64 chars).
var channelIDRegex = regexp.MustCompile(`UC[a-zA-Z0-9_-]{22}`)

// extractChannelID extracts a channel ID from various URL formats.
func extractChannelID(input string) (string, error) {
	// Direct channel ID
	if channelIDRegex.MatchString(input) {
		match := channelIDRegex.FindString(input)
		return match, nil
	}

	// Check for channel URL patterns
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

	return "", fmt.Errorf("%w: cannot extract channel ID from %q (handles require resolution)", ErrInvalidURL, input)
}

// rssErrorClassifier determines if an RSS error is retryable.
func rssErrorClassifier(err error) bool {
	if err == nil {
		return false
	}

	// Permanent errors - don't retry
	var listerErr *ListerError
	if errors.As(err, &listerErr) {
		switch listerErr.Err {
		case ErrChannelNotFound, ErrInvalidURL:
			return false
		default:
			// Retryable: rate limit, timeout, network errors
			return true
		}
	}

	// Default to retryable for unknown errors
	return true
}
