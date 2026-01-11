// Package storage provides abstractions for persisting ytsync data.
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for common conditions.
var (
	ErrNotFound       = errors.New("storage: not found")
	ErrAlreadyExists  = errors.New("storage: already exists")
	ErrInvalidInput   = errors.New("storage: invalid input")
	ErrStorageCorrupt = errors.New("storage: data corruption detected")
	ErrLockTimeout    = errors.New("storage: lock acquisition timeout")
)

// StorageError wraps errors with context.
type StorageError struct {
	Op     string // Operation: "create", "read", "update", "delete"
	Entity string // Entity type: "channel", "video", "transcript"
	ID     string // Entity ID if applicable
	Err    error  // Underlying error
}

func (e *StorageError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("storage: %s %s %s: %v", e.Op, e.Entity, e.ID, e.Err)
	}
	return fmt.Sprintf("storage: %s %s: %v", e.Op, e.Entity, e.Err)
}

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
	CreateChannel(ctx context.Context, channel *Channel) error
	GetChannel(ctx context.Context, id string) (*Channel, error)
	GetChannelByYouTubeID(ctx context.Context, youtubeID string) (*Channel, error)
	UpdateChannel(ctx context.Context, channel *Channel) error
	DeleteChannel(ctx context.Context, id string) error
	ListChannels(ctx context.Context) ([]*Channel, error)
}

// VideoStore handles video CRUD operations.
type VideoStore interface {
	CreateVideo(ctx context.Context, video *Video) error
	GetVideo(ctx context.Context, id string) (*Video, error)
	GetVideoByYouTubeID(ctx context.Context, youtubeID string) (*Video, error)
	UpdateVideo(ctx context.Context, video *Video) error
	DeleteVideo(ctx context.Context, id string) error
	ListVideosByChannel(ctx context.Context, channelID string) ([]*Video, error)
	ListVideosNeedingTranscript(ctx context.Context) ([]*Video, error)
}

// TranscriptStore handles transcript CRUD operations.
type TranscriptStore interface {
	CreateTranscript(ctx context.Context, transcript *Transcript) error
	GetTranscript(ctx context.Context, videoID string) (*Transcript, error)
	UpdateTranscript(ctx context.Context, transcript *Transcript) error
	DeleteTranscript(ctx context.Context, videoID string) error
	ListTranscriptsByChannel(ctx context.Context, channelID string) ([]*Transcript, error)
}

// SyncStateStore handles sync state operations.
type SyncStateStore interface {
	GetSyncState(ctx context.Context, channelID string) (*SyncState, error)
	UpdateSyncState(ctx context.Context, state *SyncState) error
	GetLastSync(ctx context.Context, channelID string) (time.Time, error)
}
