package storage

import "time"

// Channel represents a YouTube channel being tracked.
type Channel struct {
	ID          string    `json:"id"`                    // Internal UUID
	YouTubeID   string    `json:"youtube_id"`            // YouTube channel ID (UC...)
	Name        string    `json:"name"`                  // Channel name
	Description string    `json:"description,omitempty"` // Channel description
	URL         string    `json:"url"`                   // Channel URL
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Video represents a YouTube video.
type Video struct {
	ID            string    `json:"id"`              // Internal UUID
	YouTubeID     string    `json:"youtube_id"`      // YouTube video ID
	ChannelID     string    `json:"channel_id"`      // FK to Channel.ID
	Title         string    `json:"title"`           // Video title
	Description   string    `json:"description,omitempty"`
	PublishedAt   time.Time `json:"published_at"`
	Duration      int       `json:"duration"`       // Duration in seconds
	HasTranscript bool      `json:"has_transcript"` // Whether transcript has been fetched
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Transcript represents a video transcript.
type Transcript struct {
	VideoID   string    `json:"video_id"` // FK to Video.ID
	Language  string    `json:"language"` // Language code, e.g., "en", "auto"
	Content   string    `json:"content"`  // Plain text content
	Segments  []Segment `json:"segments,omitempty"`
	Source    string    `json:"source"` // Source: "youtube", "whisper", etc.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Segment represents a timed transcript segment.
type Segment struct {
	Start float64 `json:"start"` // Start time in seconds
	End   float64 `json:"end"`   // End time in seconds
	Text  string  `json:"text"`  // Segment text
}

// SyncState tracks synchronization progress for a channel.
type SyncState struct {
	ChannelID       string    `json:"channel_id"`
	LastSyncAt      time.Time `json:"last_sync_at"`
	LastVideoID     string    `json:"last_video_id,omitempty"` // For pagination
	VideosProcessed int       `json:"videos_processed"`
	TotalVideos     int       `json:"total_videos"`
	Status          string    `json:"status"` // "idle", "syncing", "error"
	LastError       string    `json:"last_error,omitempty"`
}

// SyncStatus constants for SyncState.Status field.
const (
	SyncStatusIdle    = "idle"
	SyncStatusSyncing = "syncing"
	SyncStatusError   = "error"
)
