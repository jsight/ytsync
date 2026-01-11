package storage

import "time"

// Channel represents a YouTube channel being tracked.
// It stores references to a YouTube channel and metadata for synchronization.
type Channel struct {
	// ID is the internal unique identifier (UUID).
	ID string `json:"id"`
	// YouTubeID is the YouTube channel ID (e.g., "UCxxxxxxxxxxxxxxx").
	YouTubeID string `json:"youtube_id"`
	// Name is the display name of the channel.
	Name string `json:"name"`
	// Description is the channel's description from YouTube.
	Description string `json:"description,omitempty"`
	// URL is the full URL to the YouTube channel.
	URL string `json:"url"`
	// CreatedAt is when this channel was first added to ytsync.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this channel record was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// Video represents a YouTube video.
// It stores references to a video and tracks whether transcripts have been processed.
type Video struct {
	// ID is the internal unique identifier (UUID).
	ID string `json:"id"`
	// YouTubeID is the YouTube video ID (e.g., "dQw4w9WgXcQ").
	YouTubeID string `json:"youtube_id"`
	// ChannelID is a foreign key reference to Channel.ID.
	ChannelID string `json:"channel_id"`
	// Title is the video title.
	Title string `json:"title"`
	// Description is the video description from YouTube.
	Description string `json:"description,omitempty"`
	// PublishedAt is when the video was published on YouTube.
	PublishedAt time.Time `json:"published_at"`
	// Duration is the video length in seconds.
	Duration int `json:"duration"`
	// HasTranscript indicates whether a transcript has been successfully fetched.
	HasTranscript bool `json:"has_transcript"`
	// CreatedAt is when this video was first added to ytsync.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this video record was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// Transcript represents a video transcript in a specific language.
// It can be a YouTube auto-generated transcript or from another source like Whisper.
type Transcript struct {
	// VideoID is a foreign key reference to Video.ID.
	VideoID string `json:"video_id"`
	// Language is the ISO 639-1 language code (e.g., "en", "es", "auto").
	Language string `json:"language"`
	// Content is the plain text transcript content.
	Content string `json:"content"`
	// Segments contains timed segments if available.
	Segments []Segment `json:"segments,omitempty"`
	// Source indicates where the transcript came from ("youtube", "whisper", etc.).
	Source string `json:"source"`
	// CreatedAt is when this transcript was first added.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this transcript was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// Segment represents a timed transcript segment with start/end times and text.
type Segment struct {
	// Start is the start time in seconds.
	Start float64 `json:"start"`
	// End is the end time in seconds.
	End float64 `json:"end"`
	// Text is the segment content.
	Text string `json:"text"`
}

// PaginationStrategy indicates which video listing strategy is being used.
type PaginationStrategy string

const (
	// StrategyInnertube uses YouTube's internal Innertube API with continuation tokens.
	StrategyInnertube PaginationStrategy = "innertube"
	// StrategyAPI uses YouTube Data API v3 with pageToken pagination.
	StrategyAPI PaginationStrategy = "api"
	// StrategyRSS uses YouTube RSS feeds (limited to 15 most recent videos).
	StrategyRSS PaginationStrategy = "rss"
	// StrategyYtdlp uses yt-dlp subprocess for video listing.
	StrategyYtdlp PaginationStrategy = "ytdlp"
)

// SyncState tracks synchronization progress for a channel.
// It records the last sync time, progress, and any errors that occurred.
// Supports multiple pagination strategies with strategy-specific fields.
type SyncState struct {
	// ChannelID is a reference to the channel being synced.
	ChannelID string `json:"channel_id"`
	// LastSyncAt is the timestamp of the last successful sync.
	LastSyncAt time.Time `json:"last_sync_at"`
	// LastVideoID is used for pagination in resuming syncs.
	LastVideoID string `json:"last_video_id,omitempty"`
	// VideosProcessed is the count of videos processed in the current sync.
	VideosProcessed int `json:"videos_processed"`
	// TotalVideos is the total number of videos on the channel.
	TotalVideos int `json:"total_videos"`
	// Status indicates the current sync state ("idle", "syncing", "error").
	Status string `json:"status"`
	// LastError contains the error message if the last sync failed.
	LastError string `json:"last_error,omitempty"`

	// Strategy indicates which pagination strategy was used for the last sync.
	Strategy PaginationStrategy `json:"strategy,omitempty"`

	// --- Innertube-specific fields ---

	// ContinuationToken stores the Innertube API continuation token for resumable syncs.
	// This is a JSON-serialized innertube.ContinuationState.
	ContinuationToken string `json:"continuation_token,omitempty"`
	// ContinuationExpiresAt is when the continuation token expires (typically 2-4 hours).
	ContinuationExpiresAt time.Time `json:"continuation_expires_at,omitempty"`

	// --- YouTube Data API v3-specific fields ---

	// APIPageToken stores the YouTube Data API v3 pageToken for resumable syncs.
	APIPageToken string `json:"api_page_token,omitempty"`
	// APIPlaylistID stores the uploads playlist ID to avoid re-resolving.
	APIPlaylistID string `json:"api_playlist_id,omitempty"`
	// APIQuotaUsed tracks estimated quota used in the current sync session.
	APIQuotaUsed int `json:"api_quota_used,omitempty"`

	// --- RSS-specific fields ---

	// NewestVideoTimestamp is the published time of the newest video seen in RSS.
	// Used for incremental sync to filter videos newer than this timestamp.
	NewestVideoTimestamp time.Time `json:"newest_video_timestamp,omitempty"`
	// RSSRequiresFullSync indicates RSS sync detected a gap and full sync is needed.
	RSSRequiresFullSync bool `json:"rss_requires_full_sync,omitempty"`

	// --- Cross-strategy fields ---

	// SyncStartedAt is when the current sync operation began.
	SyncStartedAt time.Time `json:"sync_started_at,omitempty"`
	// LastPageFetchedAt is when the last page of results was fetched.
	LastPageFetchedAt time.Time `json:"last_page_fetched_at,omitempty"`
}

// Sync status constants for the SyncState.Status field.
const (
	// SyncStatusIdle indicates the channel is not currently being synced.
	SyncStatusIdle = "idle"
	// SyncStatusSyncing indicates a sync operation is in progress.
	SyncStatusSyncing = "syncing"
	// SyncStatusError indicates the last sync operation failed.
	SyncStatusError = "error"
)

// CanResume returns true if there is a valid, non-expired pagination token
// that can be used to resume a sync from where it left off.
func (s *SyncState) CanResume() bool {
	if s == nil || s.Status != SyncStatusSyncing {
		return false
	}

	switch s.Strategy {
	case StrategyInnertube:
		// Innertube tokens expire after ~2 hours
		if s.ContinuationToken == "" {
			return false
		}
		if !s.ContinuationExpiresAt.IsZero() && time.Now().After(s.ContinuationExpiresAt) {
			return false
		}
		return true
	case StrategyAPI:
		// API page tokens don't expire, but check we have one
		return s.APIPageToken != ""
	case StrategyRSS, StrategyYtdlp:
		// RSS and ytdlp don't support resumable pagination
		return false
	default:
		return false
	}
}

// HasExpiredToken returns true if the pagination token exists but has expired.
func (s *SyncState) HasExpiredToken() bool {
	if s == nil {
		return false
	}

	switch s.Strategy {
	case StrategyInnertube:
		if s.ContinuationToken == "" {
			return false
		}
		if s.ContinuationExpiresAt.IsZero() {
			return false
		}
		return time.Now().After(s.ContinuationExpiresAt)
	default:
		return false
	}
}

// ClearPaginationState resets all pagination-related fields.
// Call this when starting a fresh sync.
func (s *SyncState) ClearPaginationState() {
	if s == nil {
		return
	}

	// Innertube
	s.ContinuationToken = ""
	s.ContinuationExpiresAt = time.Time{}

	// API
	s.APIPageToken = ""
	s.APIPlaylistID = ""
	s.APIQuotaUsed = 0

	// RSS
	s.NewestVideoTimestamp = time.Time{}
	s.RSSRequiresFullSync = false

	// Cross-strategy
	s.LastVideoID = ""
	s.VideosProcessed = 0
	s.SyncStartedAt = time.Time{}
	s.LastPageFetchedAt = time.Time{}
}

// StartSync initializes the sync state for a new sync operation.
func (s *SyncState) StartSync(strategy PaginationStrategy) {
	if s == nil {
		return
	}

	s.ClearPaginationState()
	s.Strategy = strategy
	s.Status = SyncStatusSyncing
	s.SyncStartedAt = time.Now()
	s.LastError = ""
}

// CompleteSync marks the sync as successfully completed.
func (s *SyncState) CompleteSync() {
	if s == nil {
		return
	}

	s.Status = SyncStatusIdle
	s.LastSyncAt = time.Now()
	s.ClearPaginationState()
}

// FailSync marks the sync as failed with an error message.
// Pagination state is preserved for potential resume.
func (s *SyncState) FailSync(errMsg string) {
	if s == nil {
		return
	}

	s.Status = SyncStatusError
	s.LastError = errMsg
}

// UpdateInnertubeToken updates the Innertube continuation token and expiry.
func (s *SyncState) UpdateInnertubeToken(token string, ttl time.Duration) {
	if s == nil {
		return
	}

	s.ContinuationToken = token
	s.LastPageFetchedAt = time.Now()
	if token != "" {
		s.ContinuationExpiresAt = time.Now().Add(ttl)
	} else {
		s.ContinuationExpiresAt = time.Time{}
	}
}

// UpdateAPIPageToken updates the YouTube Data API v3 page token.
func (s *SyncState) UpdateAPIPageToken(pageToken string, playlistID string, quotaUsed int) {
	if s == nil {
		return
	}

	s.APIPageToken = pageToken
	s.LastPageFetchedAt = time.Now()
	if playlistID != "" {
		s.APIPlaylistID = playlistID
	}
	s.APIQuotaUsed += quotaUsed
}

// UpdateRSSState updates the RSS sync state.
func (s *SyncState) UpdateRSSState(newestTimestamp time.Time, requiresFullSync bool) {
	if s == nil {
		return
	}

	if !newestTimestamp.IsZero() {
		s.NewestVideoTimestamp = newestTimestamp
	}
	s.RSSRequiresFullSync = requiresFullSync
	s.LastPageFetchedAt = time.Now()
}

// IncrementProgress updates the sync progress counters.
func (s *SyncState) IncrementProgress(count int, lastVideoID string) {
	if s == nil {
		return
	}

	s.VideosProcessed += count
	if lastVideoID != "" {
		s.LastVideoID = lastVideoID
	}
}

// NewSyncState creates a new SyncState for a channel.
func NewSyncState(channelID string) *SyncState {
	return &SyncState{
		ChannelID: channelID,
		Status:    SyncStatusIdle,
	}
}
