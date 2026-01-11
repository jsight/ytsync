// Package storage provides abstractions for persisting ytsync data.
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for common storage conditions.
var (
	// ErrNotFound indicates the requested entity was not found.
	ErrNotFound = errors.New("storage: not found")
	// ErrAlreadyExists indicates the entity already exists in storage.
	ErrAlreadyExists = errors.New("storage: already exists")
	// ErrInvalidInput indicates invalid or malformed input was provided.
	ErrInvalidInput = errors.New("storage: invalid input")
	// ErrStorageCorrupt indicates data corruption was detected.
	ErrStorageCorrupt = errors.New("storage: data corruption detected")
	// ErrLockTimeout indicates a timeout acquiring a file lock.
	ErrLockTimeout = errors.New("storage: lock acquisition timeout")
)

// StorageError wraps storage errors with operation and entity context.
// Use errors.As() to extract this error type and get operation details:
//
//	var storErr *storage.StorageError
//	if errors.As(err, &storErr) {
//		fmt.Printf("Failed to %s %s %s: %v\n", storErr.Op, storErr.Entity, storErr.ID, storErr.Err)
//	}
type StorageError struct {
	// Op is the operation that failed ("create", "read", "update", "delete").
	Op string
	// Entity is the entity type ("channel", "video", "transcript", etc.).
	Entity string
	// ID is the entity ID if applicable.
	ID string
	// Err is the underlying error that occurred.
	Err error
}

// Error returns a string representation of the storage error.
func (e *StorageError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("storage: %s %s %s: %v", e.Op, e.Entity, e.ID, e.Err)
	}
	return fmt.Sprintf("storage: %s %s: %v", e.Op, e.Entity, e.Err)
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *StorageError) Unwrap() error { return e.Err }

// Store is the main storage interface for all ytsync data operations.
// Implementations must be safe for concurrent use.
type Store interface {
	ChannelStore
	VideoStore
	TranscriptStore
	SyncStateStore

	// Close releases any resources held by the store.
	Close() error
}

// ChannelStore handles channel CRUD operations.
type ChannelStore interface {
	// CreateChannel saves a new channel to storage.
	CreateChannel(ctx context.Context, channel *Channel) error
	// GetChannel retrieves a channel by its internal ID.
	GetChannel(ctx context.Context, id string) (*Channel, error)
	// GetChannelByYouTubeID retrieves a channel by its YouTube ID.
	GetChannelByYouTubeID(ctx context.Context, youtubeID string) (*Channel, error)
	// UpdateChannel updates an existing channel record.
	UpdateChannel(ctx context.Context, channel *Channel) error
	// DeleteChannel removes a channel from storage.
	DeleteChannel(ctx context.Context, id string) error
	// ListChannels retrieves all channels in storage.
	ListChannels(ctx context.Context) ([]*Channel, error)
}

// VideoStore handles video CRUD operations.
type VideoStore interface {
	// CreateVideo saves a new video to storage.
	CreateVideo(ctx context.Context, video *Video) error
	// GetVideo retrieves a video by its internal ID.
	GetVideo(ctx context.Context, id string) (*Video, error)
	// GetVideoByYouTubeID retrieves a video by its YouTube ID.
	GetVideoByYouTubeID(ctx context.Context, youtubeID string) (*Video, error)
	// UpdateVideo updates an existing video record.
	UpdateVideo(ctx context.Context, video *Video) error
	// DeleteVideo removes a video from storage.
	DeleteVideo(ctx context.Context, id string) error
	// ListVideosByChannel retrieves all videos for a specific channel.
	ListVideosByChannel(ctx context.Context, channelID string) ([]*Video, error)
	// ListVideosNeedingTranscript retrieves videos that don't have a transcript yet.
	ListVideosNeedingTranscript(ctx context.Context) ([]*Video, error)
}

// TranscriptStore handles transcript CRUD operations.
type TranscriptStore interface {
	// CreateTranscript saves a new transcript to storage.
	CreateTranscript(ctx context.Context, transcript *Transcript) error
	// GetTranscript retrieves a transcript for a specific video.
	GetTranscript(ctx context.Context, videoID string) (*Transcript, error)
	// UpdateTranscript updates an existing transcript record.
	UpdateTranscript(ctx context.Context, transcript *Transcript) error
	// DeleteTranscript removes a transcript from storage.
	DeleteTranscript(ctx context.Context, videoID string) error
	// ListTranscriptsByChannel retrieves all transcripts for videos in a channel.
	ListTranscriptsByChannel(ctx context.Context, channelID string) ([]*Transcript, error)
}

// SyncStateStore handles sync state operations for tracking sync progress.
type SyncStateStore interface {
	// GetSyncState retrieves the current sync state for a channel.
	GetSyncState(ctx context.Context, channelID string) (*SyncState, error)
	// UpdateSyncState updates the sync state for a channel.
	UpdateSyncState(ctx context.Context, state *SyncState) error
	// GetLastSync returns the timestamp of the last successful sync for a channel.
	GetLastSync(ctx context.Context, channelID string) (time.Time, error)
}
