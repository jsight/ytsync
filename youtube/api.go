package youtube

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"ytsync/retry"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// APILister implements VideoLister using YouTube Data API v3.
// It supports full history of videos and graceful fallback to yt-dlp when quota is exhausted.
type APILister struct {
	service      *youtube.Service
	apiKey       string
	quotaReserve int // Minimum quota units to keep in reserve

	// Quota tracking
	mu              sync.Mutex
	estimatedQuota  int // Estimated remaining quota units
	lastQuotaReset  time.Time
	quotaExhausted  bool
	fallbackLister  VideoLister // Fallback lister (e.g., yt-dlp)
	RetryConfig     *retry.Config
}

// NewAPILister creates a new YouTube Data API v3-based video lister.
// quotaReserve specifies the minimum quota units to keep in reserve (default 0).
func NewAPILister(apiKey string, quotaReserve int) (*APILister, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("api key required")
	}

	service, err := youtube.NewService(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("create youtube service: %w", err)
	}

	cfg := retry.DefaultConfig()
	return &APILister{
		service:      service,
		apiKey:       apiKey,
		quotaReserve: quotaReserve,
		estimatedQuota: 10000, // Default daily quota
		lastQuotaReset: time.Now(),
		RetryConfig:    &cfg,
	}, nil
}

// SetFallbackLister sets the fallback lister to use when quota is exhausted.
func (a *APILister) SetFallbackLister(lister VideoLister) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fallbackLister = lister
}

// ListVideos fetches videos from the specified channel using YouTube Data API v3.
// It gracefully falls back to the fallback lister if quota is exhausted.
func (a *APILister) ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error) {
	a.mu.Lock()
	if a.quotaExhausted && a.fallbackLister != nil {
		a.mu.Unlock()
		log.Printf("youtube: API quota exhausted, falling back to %T", a.fallbackLister)
		return a.fallbackLister.ListVideos(ctx, channelURL, opts)
	}
	a.mu.Unlock()

	// Resolve channel ID
	channelID, err := a.resolveChannelID(ctx, channelURL)
	if err != nil {
		return nil, &ListerError{Source: "api", Channel: channelURL, Err: err}
	}

	// Get uploads playlist ID
	uploadsPlaylistID, channelName, err := a.getUploadsPlaylistID(ctx, channelID)
	if err != nil {
		return nil, &ListerError{Source: "api", Channel: channelURL, Err: err}
	}

	// List videos from the uploads playlist
	videos, err := a.listPlaylistVideos(ctx, uploadsPlaylistID, channelID, channelName, opts)
	if err != nil {
		return nil, &ListerError{Source: "api", Channel: channelURL, Err: err}
	}

	return videos, nil
}

// SupportsFullHistory returns true - API can retrieve all videos.
func (a *APILister) SupportsFullHistory() bool {
	return true
}

// resolveChannelID converts a channel URL, handle, or ID to a channel ID.
func (a *APILister) resolveChannelID(ctx context.Context, input string) (string, error) {
	// Check if it's already a channel ID
	if channelIDRegex.MatchString(input) {
		return channelIDRegex.FindString(input), nil
	}

	// If it's a handle (@username), search for it
	if strings.HasPrefix(input, "@") {
		return a.searchChannelByHandle(ctx, input)
	}

	// If it contains /channel/ or /c/, try to resolve it
	if strings.Contains(input, "youtube.com/channel/") {
		if id := extractChannelIDFromURL(input); id != "" {
			return id, nil
		}
	}
	if strings.Contains(input, "youtube.com/c/") {
		// Custom URLs need to be resolved via search
		parts := strings.Split(input, "youtube.com/c/")
		if len(parts) > 1 {
			customURL := strings.Split(parts[1], "/")[0]
			return a.searchChannelByCustomURL(ctx, customURL)
		}
	}

	return "", fmt.Errorf("%w: cannot resolve channel from %q", ErrInvalidURL, input)
}

// extractChannelIDFromURL extracts channel ID from a standard channel URL.
func extractChannelIDFromURL(url string) string {
	if strings.Contains(url, "youtube.com/channel/") {
		parts := strings.Split(url, "youtube.com/channel/")
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

// searchChannelByHandle searches for a channel by its handle (@username).
func (a *APILister) searchChannelByHandle(ctx context.Context, handle string) (string, error) {
	// Remove @ prefix if present
	handle = strings.TrimPrefix(handle, "@")

	var channelID string
	cfg := a.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	err := retry.Do(ctx, *cfg, apiErrorClassifier, func(ctx context.Context) error {
		call := a.service.Search.List([]string{"id"}).
			Q(handle).
			Type("channel").
			MaxResults(1).
			Context(ctx)

		resp, err := call.Do()
		if err != nil {
			if ctx.Err() != nil {
				return ErrNetworkTimeout
			}
			return err
		}

		if len(resp.Items) == 0 {
			return ErrChannelNotFound
		}

		channelID = resp.Items[0].Id.ChannelId
		a.trackQuotaUsage(100) // Search uses 100 units
		return nil
	})

	if err != nil {
		return "", err
	}

	return channelID, nil
}

// searchChannelByCustomURL searches for a channel by its custom URL.
func (a *APILister) searchChannelByCustomURL(ctx context.Context, customURL string) (string, error) {
	var channelID string
	cfg := a.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	err := retry.Do(ctx, *cfg, apiErrorClassifier, func(ctx context.Context) error {
		call := a.service.Search.List([]string{"id"}).
			Q(customURL).
			Type("channel").
			MaxResults(1).
			Context(ctx)

		resp, err := call.Do()
		if err != nil {
			if ctx.Err() != nil {
				return ErrNetworkTimeout
			}
			return err
		}

		if len(resp.Items) == 0 {
			return ErrChannelNotFound
		}

		channelID = resp.Items[0].Id.ChannelId
		a.trackQuotaUsage(100) // Search uses 100 units
		return nil
	})

	if err != nil {
		return "", err
	}

	return channelID, nil
}

// getUploadsPlaylistID gets the uploads playlist ID for a channel.
func (a *APILister) getUploadsPlaylistID(ctx context.Context, channelID string) (string, string, error) {
	var playlistID string
	var channelName string

	cfg := a.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	err := retry.Do(ctx, *cfg, apiErrorClassifier, func(ctx context.Context) error {
		call := a.service.Channels.List([]string{"contentDetails", "snippet"}).
			Id(channelID).
			Context(ctx)

		resp, err := call.Do()
		if err != nil {
			if ctx.Err() != nil {
				return ErrNetworkTimeout
			}
			return err
		}

		if len(resp.Items) == 0 {
			return ErrChannelNotFound
		}

		channel := resp.Items[0]
		playlistID = channel.ContentDetails.RelatedPlaylists.Uploads
		if channel.Snippet != nil {
			channelName = channel.Snippet.Title
		}

		a.trackQuotaUsage(1) // channels.list uses 1 unit
		return nil
	})

	if err != nil {
		return "", "", err
	}

	return playlistID, channelName, nil
}

// listPlaylistVideos fetches all videos from a playlist using pagination.
func (a *APILister) listPlaylistVideos(ctx context.Context, playlistID, channelID, channelName string, opts *ListOptions) ([]VideoInfo, error) {
	var allVideos []VideoInfo

	cfg := a.RetryConfig
	if cfg == nil {
		defaultCfg := retry.DefaultConfig()
		cfg = &defaultCfg
	}

	pageToken := ""
	for {
		// Check if we should stop
		if opts != nil && opts.MaxResults > 0 && len(allVideos) >= opts.MaxResults {
			allVideos = allVideos[:opts.MaxResults]
			break
		}

		// Fetch a page of results
		err := retry.Do(ctx, *cfg, apiErrorClassifier, func(ctx context.Context) error {
			call := a.service.PlaylistItems.List([]string{"snippet", "contentDetails"}).
				PlaylistId(playlistID).
				MaxResults(50).
				PageToken(pageToken).
				Context(ctx)

			resp, err := call.Do()
			if err != nil {
				if ctx.Err() != nil {
					return ErrNetworkTimeout
				}
				return err
			}

			// Convert playlist items to VideoInfo
			for _, item := range resp.Items {
				video := VideoInfo{
					ID:          item.ContentDetails.VideoId,
					ChannelID:   channelID,
					ChannelName: channelName,
				}

				if item.Snippet != nil {
					video.Title = item.Snippet.Title
					video.Description = item.Snippet.Description
					if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Default != nil {
						video.Thumbnail = item.Snippet.Thumbnails.Default.Url
					}
					// Parse RFC3339 published date
					if t, err := time.Parse(time.RFC3339, item.Snippet.PublishedAt); err == nil {
						video.Published = t
					}
				}

				allVideos = append(allVideos, video)
			}

			pageToken = resp.NextPageToken
			a.trackQuotaUsage(1) // playlistItems.list uses 1 unit per page

			return nil
		})

		if err != nil {
			return nil, err
		}

		// Stop if no more pages
		if pageToken == "" {
			break
		}

		// Check quota and potentially fallback
		a.mu.Lock()
		if a.quotaExhausted && a.fallbackLister != nil {
			a.mu.Unlock()
			log.Printf("youtube: API quota exhausted during pagination, falling back to %T", a.fallbackLister)
			// Fallback to alternate lister for remaining videos
			remainingOpts := *opts
			if opts != nil && opts.MaxResults > 0 {
				remainingOpts.MaxResults = opts.MaxResults - len(allVideos)
			}
			fallbackVideos, err := a.fallbackLister.ListVideos(ctx, "https://www.youtube.com/channel/"+channelID, &remainingOpts)
			if err != nil {
				return allVideos, nil // Return what we got
			}
			allVideos = append(allVideos, fallbackVideos...)
			break
		}
		a.mu.Unlock()
	}

	// Apply filters
	if opts != nil {
		allVideos = filterVideos(allVideos, opts)
	}

	return allVideos, nil
}

// trackQuotaUsage updates the estimated quota and checks if we've exhausted it.
func (a *APILister) trackQuotaUsage(units int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Reset quota if a day has passed
	if time.Since(a.lastQuotaReset) > 24*time.Hour {
		a.estimatedQuota = 10000
		a.lastQuotaReset = time.Now()
		a.quotaExhausted = false
		log.Printf("youtube: quota reset (new day)")
	}

	a.estimatedQuota -= units

	if a.estimatedQuota < a.quotaReserve {
		if !a.quotaExhausted {
			log.Printf("youtube: quota exhausted (remaining: %d, reserve: %d)", a.estimatedQuota, a.quotaReserve)
			a.quotaExhausted = true
		}
	} else {
		log.Printf("youtube: quota usage - remaining: %d units", a.estimatedQuota)
	}
}

// GetEstimatedQuota returns the estimated remaining quota units.
func (a *APILister) GetEstimatedQuota() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.estimatedQuota
}

// GetQuotaExhausted returns whether the quota has been exhausted.
func (a *APILister) GetQuotaExhausted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.quotaExhausted
}

// apiErrorClassifier determines if an API error is retryable.
func apiErrorClassifier(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry specific sentinel errors
	switch err {
	case ErrChannelNotFound, ErrInvalidURL:
		return false
	}

	// Rate limit errors are retryable
	if strings.Contains(err.Error(), "quotaExceeded") {
		return true
	}
	if strings.Contains(err.Error(), "rateLimitExceeded") {
		return true
	}

	// Timeout errors are retryable
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Default to retryable for unknown errors
	return true
}
