package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewJSONStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	defer store.Close()

	// File should exist after creation
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("store file was not created")
	}
}

func TestJSONStore_LoadExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Create store and add a channel
	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	ctx := context.Background()
	channel := &Channel{
		YouTubeID: "UC123",
		Name:      "Test Channel",
		URL:       "https://youtube.com/@test",
	}
	if err := store.CreateChannel(ctx, channel); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	store.Close()

	// Reopen and verify
	store2, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() reopen error = %v", err)
	}
	defer store2.Close()

	loaded, err := store2.GetChannelByYouTubeID(ctx, "UC123")
	if err != nil {
		t.Fatalf("GetChannelByYouTubeID() error = %v", err)
	}
	if loaded.Name != "Test Channel" {
		t.Errorf("loaded channel name = %q, want %q", loaded.Name, "Test Channel")
	}
}

func TestJSONStore_ChannelCRUD(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create
	channel := &Channel{
		YouTubeID:   "UC123",
		Name:        "Test Channel",
		Description: "A test channel",
		URL:         "https://youtube.com/@test",
	}
	if err := store.CreateChannel(ctx, channel); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	if channel.ID == "" {
		t.Error("CreateChannel() did not assign ID")
	}

	// Read
	got, err := store.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("GetChannel() error = %v", err)
	}
	if got.Name != channel.Name {
		t.Errorf("GetChannel() name = %q, want %q", got.Name, channel.Name)
	}

	// Read by YouTube ID
	got, err = store.GetChannelByYouTubeID(ctx, "UC123")
	if err != nil {
		t.Fatalf("GetChannelByYouTubeID() error = %v", err)
	}
	if got.ID != channel.ID {
		t.Errorf("GetChannelByYouTubeID() ID = %q, want %q", got.ID, channel.ID)
	}

	// Update
	channel.Name = "Updated Channel"
	if err := store.UpdateChannel(ctx, channel); err != nil {
		t.Fatalf("UpdateChannel() error = %v", err)
	}
	got, _ = store.GetChannel(ctx, channel.ID)
	if got.Name != "Updated Channel" {
		t.Errorf("UpdateChannel() name = %q, want %q", got.Name, "Updated Channel")
	}

	// List
	channels, err := store.ListChannels(ctx)
	if err != nil {
		t.Fatalf("ListChannels() error = %v", err)
	}
	if len(channels) != 1 {
		t.Errorf("ListChannels() len = %d, want 1", len(channels))
	}

	// Delete
	if err := store.DeleteChannel(ctx, channel.ID); err != nil {
		t.Fatalf("DeleteChannel() error = %v", err)
	}
	_, err = store.GetChannel(ctx, channel.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetChannel() after delete error = %v, want ErrNotFound", err)
	}
}

func TestJSONStore_ChannelDuplicate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	channel := &Channel{YouTubeID: "UC123", Name: "Test"}
	if err := store.CreateChannel(ctx, channel); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}

	// Try to create duplicate
	channel2 := &Channel{YouTubeID: "UC123", Name: "Test 2"}
	err := store.CreateChannel(ctx, channel2)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("CreateChannel() duplicate error = %v, want ErrAlreadyExists", err)
	}
}

func TestJSONStore_VideoCRUD(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create channel first
	channel := &Channel{YouTubeID: "UC123", Name: "Test Channel"}
	store.CreateChannel(ctx, channel)

	// Create video
	video := &Video{
		YouTubeID:   "vid123",
		ChannelID:   channel.ID,
		Title:       "Test Video",
		PublishedAt: time.Now(),
		Duration:    300,
	}
	if err := store.CreateVideo(ctx, video); err != nil {
		t.Fatalf("CreateVideo() error = %v", err)
	}
	if video.ID == "" {
		t.Error("CreateVideo() did not assign ID")
	}

	// Read
	got, err := store.GetVideo(ctx, video.ID)
	if err != nil {
		t.Fatalf("GetVideo() error = %v", err)
	}
	if got.Title != video.Title {
		t.Errorf("GetVideo() title = %q, want %q", got.Title, video.Title)
	}

	// Read by YouTube ID
	got, err = store.GetVideoByYouTubeID(ctx, "vid123")
	if err != nil {
		t.Fatalf("GetVideoByYouTubeID() error = %v", err)
	}
	if got.ID != video.ID {
		t.Errorf("GetVideoByYouTubeID() ID = %q, want %q", got.ID, video.ID)
	}

	// List by channel
	videos, err := store.ListVideosByChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListVideosByChannel() error = %v", err)
	}
	if len(videos) != 1 {
		t.Errorf("ListVideosByChannel() len = %d, want 1", len(videos))
	}

	// List needing transcript
	videos, err = store.ListVideosNeedingTranscript(ctx)
	if err != nil {
		t.Fatalf("ListVideosNeedingTranscript() error = %v", err)
	}
	if len(videos) != 1 {
		t.Errorf("ListVideosNeedingTranscript() len = %d, want 1", len(videos))
	}

	// Update
	video.Title = "Updated Video"
	if err := store.UpdateVideo(ctx, video); err != nil {
		t.Fatalf("UpdateVideo() error = %v", err)
	}

	// Delete
	if err := store.DeleteVideo(ctx, video.ID); err != nil {
		t.Fatalf("DeleteVideo() error = %v", err)
	}
	_, err = store.GetVideo(ctx, video.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetVideo() after delete error = %v, want ErrNotFound", err)
	}
}

func TestJSONStore_TranscriptCRUD(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create channel and video
	channel := &Channel{YouTubeID: "UC123", Name: "Test"}
	store.CreateChannel(ctx, channel)
	video := &Video{YouTubeID: "vid123", ChannelID: channel.ID, Title: "Test Video"}
	store.CreateVideo(ctx, video)

	// Create transcript
	transcript := &Transcript{
		VideoID:  video.ID,
		Language: "en",
		Content:  "Hello world",
		Source:   "youtube",
	}
	if err := store.CreateTranscript(ctx, transcript); err != nil {
		t.Fatalf("CreateTranscript() error = %v", err)
	}

	// Video should now have HasTranscript = true
	v, _ := store.GetVideo(ctx, video.ID)
	if !v.HasTranscript {
		t.Error("CreateTranscript() did not set video.HasTranscript")
	}

	// Read
	got, err := store.GetTranscript(ctx, video.ID)
	if err != nil {
		t.Fatalf("GetTranscript() error = %v", err)
	}
	if got.Content != "Hello world" {
		t.Errorf("GetTranscript() content = %q, want %q", got.Content, "Hello world")
	}

	// List by channel
	transcripts, err := store.ListTranscriptsByChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListTranscriptsByChannel() error = %v", err)
	}
	if len(transcripts) != 1 {
		t.Errorf("ListTranscriptsByChannel() len = %d, want 1", len(transcripts))
	}

	// Update
	transcript.Content = "Updated content"
	if err := store.UpdateTranscript(ctx, transcript); err != nil {
		t.Fatalf("UpdateTranscript() error = %v", err)
	}

	// Delete
	if err := store.DeleteTranscript(ctx, video.ID); err != nil {
		t.Fatalf("DeleteTranscript() error = %v", err)
	}

	// Video should now have HasTranscript = false
	v, _ = store.GetVideo(ctx, video.ID)
	if v.HasTranscript {
		t.Error("DeleteTranscript() did not clear video.HasTranscript")
	}
}

func TestJSONStore_SyncState(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	channel := &Channel{YouTubeID: "UC123", Name: "Test"}
	store.CreateChannel(ctx, channel)

	// Initially no sync state
	_, err := store.GetSyncState(ctx, channel.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSyncState() initial error = %v, want ErrNotFound", err)
	}

	// Update (creates) sync state
	now := time.Now()
	state := &SyncState{
		ChannelID:       channel.ID,
		LastSyncAt:      now,
		VideosProcessed: 10,
		TotalVideos:     100,
		Status:          SyncStatusIdle,
	}
	if err := store.UpdateSyncState(ctx, state); err != nil {
		t.Fatalf("UpdateSyncState() error = %v", err)
	}

	// Read back
	got, err := store.GetSyncState(ctx, channel.ID)
	if err != nil {
		t.Fatalf("GetSyncState() error = %v", err)
	}
	if got.VideosProcessed != 10 {
		t.Errorf("GetSyncState() VideosProcessed = %d, want 10", got.VideosProcessed)
	}

	// GetLastSync
	lastSync, err := store.GetLastSync(ctx, channel.ID)
	if err != nil {
		t.Fatalf("GetLastSync() error = %v", err)
	}
	if !lastSync.Equal(now) {
		t.Errorf("GetLastSync() = %v, want %v", lastSync, now)
	}
}

func TestStorageError(t *testing.T) {
	err := &StorageError{
		Op:     "read",
		Entity: "channel",
		ID:     "abc123",
		Err:    ErrNotFound,
	}

	want := "storage: read channel abc123: storage: not found"
	if err.Error() != want {
		t.Errorf("StorageError.Error() = %q, want %q", err.Error(), want)
	}

	if !errors.Is(err, ErrNotFound) {
		t.Error("StorageError should unwrap to ErrNotFound")
	}
}

// newTestStore creates a temporary store for testing.
func newTestStore(t *testing.T) *JSONStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	return store
}
