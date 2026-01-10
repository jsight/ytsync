package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	schemaVersion = "1.0"
	lockTimeout   = 5 * time.Second
)

// JSONStore implements Store using a single JSON file.
type JSONStore struct {
	path string
	lock *FileLock
	data *storeData
	mu   sync.RWMutex
}

// storeData is the top-level JSON structure.
type storeData struct {
	Version     string                  `json:"version"`
	UpdatedAt   time.Time               `json:"updated_at"`
	Channels    map[string]*Channel     `json:"channels"`
	Videos      map[string]*Video       `json:"videos"`
	Transcripts map[string]*Transcript  `json:"transcripts"`
	SyncStates  map[string]*SyncState   `json:"sync_states"`
	Indexes     *indexes                `json:"indexes"`
}

// indexes maintains lookup tables for efficient queries.
type indexes struct {
	YouTubeChannelID map[string]string   `json:"youtube_channel_id"` // youtube_id -> internal_id
	YouTubeVideoID   map[string]string   `json:"youtube_video_id"`   // youtube_id -> internal_id
	VideosByChannel  map[string][]string `json:"videos_by_channel"`  // channel_id -> []video_id
}

// NewJSONStore creates a new JSON file store at the given path.
// If the file exists, it is loaded; otherwise an empty store is created.
func NewJSONStore(path string) (*JSONStore, error) {
	s := &JSONStore{
		path: path,
		lock: NewFileLock(path),
	}

	if err := s.lock.Lock(lockTimeout); err != nil {
		return nil, err
	}

	if err := s.load(); err != nil {
		s.lock.Unlock()
		return nil, err
	}

	return s, nil
}

// load reads the JSON file into memory. Creates empty data if file doesn't exist.
func (s *JSONStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = newStoreData()
			// Save immediately to catch permission errors early
			return s.save()
		}
		return &StorageError{Op: "read", Entity: "store", Err: err}
	}

	s.data = &storeData{}
	if err := json.Unmarshal(data, s.data); err != nil {
		return &StorageError{Op: "read", Entity: "store", Err: ErrStorageCorrupt}
	}

	// Ensure indexes exist
	if s.data.Indexes == nil {
		s.data.Indexes = newIndexes()
	}

	return nil
}

// save persists the data to disk atomically.
func (s *JSONStore) save() error {
	s.data.UpdatedAt = time.Now()

	writer, err := NewAtomicWriter(s.path)
	if err != nil {
		return &StorageError{Op: "write", Entity: "store", Err: err}
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.data); err != nil {
		writer.Abort()
		return &StorageError{Op: "write", Entity: "store", Err: err}
	}

	if err := writer.Commit(); err != nil {
		return &StorageError{Op: "write", Entity: "store", Err: err}
	}

	return nil
}

// Close releases resources held by the store.
func (s *JSONStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lock.Unlock()
}

func newStoreData() *storeData {
	return &storeData{
		Version:     schemaVersion,
		UpdatedAt:   time.Now(),
		Channels:    make(map[string]*Channel),
		Videos:      make(map[string]*Video),
		Transcripts: make(map[string]*Transcript),
		SyncStates:  make(map[string]*SyncState),
		Indexes:     newIndexes(),
	}
}

func newIndexes() *indexes {
	return &indexes{
		YouTubeChannelID: make(map[string]string),
		YouTubeVideoID:   make(map[string]string),
		VideosByChannel:  make(map[string][]string),
	}
}

// --- ChannelStore implementation ---

func (s *JSONStore) CreateChannel(ctx context.Context, channel *Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channel.ID == "" {
		channel.ID = uuid.NewString()
	}

	if _, exists := s.data.Channels[channel.ID]; exists {
		return &StorageError{Op: "create", Entity: "channel", ID: channel.ID, Err: ErrAlreadyExists}
	}

	if _, exists := s.data.Indexes.YouTubeChannelID[channel.YouTubeID]; exists {
		return &StorageError{Op: "create", Entity: "channel", ID: channel.YouTubeID, Err: ErrAlreadyExists}
	}

	now := time.Now()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	s.data.Channels[channel.ID] = channel
	s.data.Indexes.YouTubeChannelID[channel.YouTubeID] = channel.ID

	return s.save()
}

func (s *JSONStore) GetChannel(ctx context.Context, id string) (*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channel, exists := s.data.Channels[id]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "channel", ID: id, Err: ErrNotFound}
	}
	return channel, nil
}

func (s *JSONStore) GetChannelByYouTubeID(ctx context.Context, youtubeID string) (*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.data.Indexes.YouTubeChannelID[youtubeID]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "channel", ID: youtubeID, Err: ErrNotFound}
	}

	channel, exists := s.data.Channels[id]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "channel", ID: id, Err: ErrStorageCorrupt}
	}
	return channel, nil
}

func (s *JSONStore) UpdateChannel(ctx context.Context, channel *Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.data.Channels[channel.ID]
	if !exists {
		return &StorageError{Op: "update", Entity: "channel", ID: channel.ID, Err: ErrNotFound}
	}

	// Update YouTube ID index if changed
	if existing.YouTubeID != channel.YouTubeID {
		delete(s.data.Indexes.YouTubeChannelID, existing.YouTubeID)
		s.data.Indexes.YouTubeChannelID[channel.YouTubeID] = channel.ID
	}

	channel.UpdatedAt = time.Now()
	s.data.Channels[channel.ID] = channel

	return s.save()
}

func (s *JSONStore) DeleteChannel(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	channel, exists := s.data.Channels[id]
	if !exists {
		return &StorageError{Op: "delete", Entity: "channel", ID: id, Err: ErrNotFound}
	}

	delete(s.data.Channels, id)
	delete(s.data.Indexes.YouTubeChannelID, channel.YouTubeID)
	delete(s.data.Indexes.VideosByChannel, id)
	delete(s.data.SyncStates, id)

	return s.save()
}

func (s *JSONStore) ListChannels(ctx context.Context) ([]*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels := make([]*Channel, 0, len(s.data.Channels))
	for _, ch := range s.data.Channels {
		channels = append(channels, ch)
	}
	return channels, nil
}

// --- VideoStore implementation ---

func (s *JSONStore) CreateVideo(ctx context.Context, video *Video) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if video.ID == "" {
		video.ID = uuid.NewString()
	}

	if _, exists := s.data.Videos[video.ID]; exists {
		return &StorageError{Op: "create", Entity: "video", ID: video.ID, Err: ErrAlreadyExists}
	}

	if _, exists := s.data.Indexes.YouTubeVideoID[video.YouTubeID]; exists {
		return &StorageError{Op: "create", Entity: "video", ID: video.YouTubeID, Err: ErrAlreadyExists}
	}

	now := time.Now()
	video.CreatedAt = now
	video.UpdatedAt = now

	s.data.Videos[video.ID] = video
	s.data.Indexes.YouTubeVideoID[video.YouTubeID] = video.ID
	s.data.Indexes.VideosByChannel[video.ChannelID] = append(
		s.data.Indexes.VideosByChannel[video.ChannelID], video.ID)

	return s.save()
}

func (s *JSONStore) GetVideo(ctx context.Context, id string) (*Video, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	video, exists := s.data.Videos[id]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "video", ID: id, Err: ErrNotFound}
	}
	return video, nil
}

func (s *JSONStore) GetVideoByYouTubeID(ctx context.Context, youtubeID string) (*Video, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.data.Indexes.YouTubeVideoID[youtubeID]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "video", ID: youtubeID, Err: ErrNotFound}
	}

	video, exists := s.data.Videos[id]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "video", ID: id, Err: ErrStorageCorrupt}
	}
	return video, nil
}

func (s *JSONStore) UpdateVideo(ctx context.Context, video *Video) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.data.Videos[video.ID]
	if !exists {
		return &StorageError{Op: "update", Entity: "video", ID: video.ID, Err: ErrNotFound}
	}

	// Update YouTube ID index if changed
	if existing.YouTubeID != video.YouTubeID {
		delete(s.data.Indexes.YouTubeVideoID, existing.YouTubeID)
		s.data.Indexes.YouTubeVideoID[video.YouTubeID] = video.ID
	}

	video.UpdatedAt = time.Now()
	s.data.Videos[video.ID] = video

	return s.save()
}

func (s *JSONStore) DeleteVideo(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	video, exists := s.data.Videos[id]
	if !exists {
		return &StorageError{Op: "delete", Entity: "video", ID: id, Err: ErrNotFound}
	}

	delete(s.data.Videos, id)
	delete(s.data.Indexes.YouTubeVideoID, video.YouTubeID)
	delete(s.data.Transcripts, id)

	// Remove from channel index
	channelVideos := s.data.Indexes.VideosByChannel[video.ChannelID]
	for i, vid := range channelVideos {
		if vid == id {
			s.data.Indexes.VideosByChannel[video.ChannelID] = append(
				channelVideos[:i], channelVideos[i+1:]...)
			break
		}
	}

	return s.save()
}

func (s *JSONStore) ListVideosByChannel(ctx context.Context, channelID string) ([]*Video, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	videoIDs := s.data.Indexes.VideosByChannel[channelID]
	videos := make([]*Video, 0, len(videoIDs))
	for _, id := range videoIDs {
		if video, exists := s.data.Videos[id]; exists {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

func (s *JSONStore) ListVideosNeedingTranscript(ctx context.Context) ([]*Video, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var videos []*Video
	for _, video := range s.data.Videos {
		if !video.HasTranscript {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

// --- TranscriptStore implementation ---

func (s *JSONStore) CreateTranscript(ctx context.Context, transcript *Transcript) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Transcripts[transcript.VideoID]; exists {
		return &StorageError{Op: "create", Entity: "transcript", ID: transcript.VideoID, Err: ErrAlreadyExists}
	}

	now := time.Now()
	transcript.CreatedAt = now
	transcript.UpdatedAt = now

	s.data.Transcripts[transcript.VideoID] = transcript

	// Update video's HasTranscript flag
	if video, exists := s.data.Videos[transcript.VideoID]; exists {
		video.HasTranscript = true
		video.UpdatedAt = now
	}

	return s.save()
}

func (s *JSONStore) GetTranscript(ctx context.Context, videoID string) (*Transcript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	transcript, exists := s.data.Transcripts[videoID]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "transcript", ID: videoID, Err: ErrNotFound}
	}
	return transcript, nil
}

func (s *JSONStore) UpdateTranscript(ctx context.Context, transcript *Transcript) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Transcripts[transcript.VideoID]; !exists {
		return &StorageError{Op: "update", Entity: "transcript", ID: transcript.VideoID, Err: ErrNotFound}
	}

	transcript.UpdatedAt = time.Now()
	s.data.Transcripts[transcript.VideoID] = transcript

	return s.save()
}

func (s *JSONStore) DeleteTranscript(ctx context.Context, videoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Transcripts[videoID]; !exists {
		return &StorageError{Op: "delete", Entity: "transcript", ID: videoID, Err: ErrNotFound}
	}

	delete(s.data.Transcripts, videoID)

	// Update video's HasTranscript flag
	if video, exists := s.data.Videos[videoID]; exists {
		video.HasTranscript = false
		video.UpdatedAt = time.Now()
	}

	return s.save()
}

func (s *JSONStore) ListTranscriptsByChannel(ctx context.Context, channelID string) ([]*Transcript, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	videoIDs := s.data.Indexes.VideosByChannel[channelID]
	var transcripts []*Transcript
	for _, videoID := range videoIDs {
		if transcript, exists := s.data.Transcripts[videoID]; exists {
			transcripts = append(transcripts, transcript)
		}
	}
	return transcripts, nil
}

// --- SyncStateStore implementation ---

func (s *JSONStore) GetSyncState(ctx context.Context, channelID string) (*SyncState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.data.SyncStates[channelID]
	if !exists {
		return nil, &StorageError{Op: "read", Entity: "sync_state", ID: channelID, Err: ErrNotFound}
	}
	return state, nil
}

func (s *JSONStore) UpdateSyncState(ctx context.Context, state *SyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.SyncStates[state.ChannelID] = state
	return s.save()
}

func (s *JSONStore) GetLastSync(ctx context.Context, channelID string) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.data.SyncStates[channelID]
	if !exists {
		return time.Time{}, &StorageError{Op: "read", Entity: "sync_state", ID: channelID, Err: ErrNotFound}
	}
	return state.LastSyncAt, nil
}
