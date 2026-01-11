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

// SyncState tracks synchronization progress for a channel.
// It records the last sync time, progress, and any errors that occurred.
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
