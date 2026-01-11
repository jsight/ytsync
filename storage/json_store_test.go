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

func TestSyncState_CanResume(t *testing.T) {
	tests := []struct {
		name  string
		state *SyncState
		want  bool
	}{
		{
			name:  "nil state",
			state: nil,
			want:  false,
		},
		{
			name: "idle status",
			state: &SyncState{
				Status:            SyncStatusIdle,
				Strategy:          StrategyInnertube,
				ContinuationToken: "token123",
			},
			want: false,
		},
		{
			name: "innertube with valid token",
			state: &SyncState{
				Status:                SyncStatusSyncing,
				Strategy:              StrategyInnertube,
				ContinuationToken:     "token123",
				ContinuationExpiresAt: time.Now().Add(1 * time.Hour),
			},
			want: true,
		},
		{
			name: "innertube with expired token",
			state: &SyncState{
				Status:                SyncStatusSyncing,
				Strategy:              StrategyInnertube,
				ContinuationToken:     "token123",
				ContinuationExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			want: false,
		},
		{
			name: "innertube with empty token",
			state: &SyncState{
				Status:   SyncStatusSyncing,
				Strategy: StrategyInnertube,
			},
			want: false,
		},
		{
			name: "api with valid page token",
			state: &SyncState{
				Status:       SyncStatusSyncing,
				Strategy:     StrategyAPI,
				APIPageToken: "pageToken123",
			},
			want: true,
		},
		{
			name: "api with empty page token",
			state: &SyncState{
				Status:   SyncStatusSyncing,
				Strategy: StrategyAPI,
			},
			want: false,
		},
		{
			name: "rss never resumable",
			state: &SyncState{
				Status:   SyncStatusSyncing,
				Strategy: StrategyRSS,
			},
			want: false,
		},
		{
			name: "ytdlp never resumable",
			state: &SyncState{
				Status:   SyncStatusSyncing,
				Strategy: StrategyYtdlp,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.CanResume(); got != tt.want {
				t.Errorf("CanResume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncState_HasExpiredToken(t *testing.T) {
	tests := []struct {
		name  string
		state *SyncState
		want  bool
	}{
		{
			name:  "nil state",
			state: nil,
			want:  false,
		},
		{
			name: "innertube expired",
			state: &SyncState{
				Strategy:              StrategyInnertube,
				ContinuationToken:     "token123",
				ContinuationExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			want: true,
		},
		{
			name: "innertube not expired",
			state: &SyncState{
				Strategy:              StrategyInnertube,
				ContinuationToken:     "token123",
				ContinuationExpiresAt: time.Now().Add(1 * time.Hour),
			},
			want: false,
		},
		{
			name: "innertube no token",
			state: &SyncState{
				Strategy: StrategyInnertube,
			},
			want: false,
		},
		{
			name: "api tokens don't expire",
			state: &SyncState{
				Strategy:     StrategyAPI,
				APIPageToken: "pageToken123",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.HasExpiredToken(); got != tt.want {
				t.Errorf("HasExpiredToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncState_ClearPaginationState(t *testing.T) {
	state := &SyncState{
		ChannelID:             "ch123",
		ContinuationToken:     "token123",
		ContinuationExpiresAt: time.Now(),
		APIPageToken:          "page123",
		APIPlaylistID:         "PL123",
		APIQuotaUsed:          100,
		NewestVideoTimestamp:  time.Now(),
		RSSRequiresFullSync:   true,
		LastVideoID:           "vid123",
		VideosProcessed:       50,
		SyncStartedAt:         time.Now(),
		LastPageFetchedAt:     time.Now(),
	}

	state.ClearPaginationState()

	if state.ChannelID != "ch123" {
		t.Error("ClearPaginationState() should not clear ChannelID")
	}
	if state.ContinuationToken != "" {
		t.Error("ClearPaginationState() should clear ContinuationToken")
	}
	if state.APIPageToken != "" {
		t.Error("ClearPaginationState() should clear APIPageToken")
	}
	if state.APIPlaylistID != "" {
		t.Error("ClearPaginationState() should clear APIPlaylistID")
	}
	if state.APIQuotaUsed != 0 {
		t.Error("ClearPaginationState() should clear APIQuotaUsed")
	}
	if !state.NewestVideoTimestamp.IsZero() {
		t.Error("ClearPaginationState() should clear NewestVideoTimestamp")
	}
	if state.RSSRequiresFullSync {
		t.Error("ClearPaginationState() should clear RSSRequiresFullSync")
	}
	if state.VideosProcessed != 0 {
		t.Error("ClearPaginationState() should clear VideosProcessed")
	}

	// Test nil safety
	var nilState *SyncState
	nilState.ClearPaginationState() // Should not panic
}

func TestSyncState_StartSync(t *testing.T) {
	state := &SyncState{
		ChannelID:         "ch123",
		ContinuationToken: "old_token",
		Status:            SyncStatusIdle,
		LastError:         "previous error",
	}

	state.StartSync(StrategyAPI)

	if state.Strategy != StrategyAPI {
		t.Errorf("StartSync() Strategy = %v, want %v", state.Strategy, StrategyAPI)
	}
	if state.Status != SyncStatusSyncing {
		t.Errorf("StartSync() Status = %v, want %v", state.Status, SyncStatusSyncing)
	}
	if state.LastError != "" {
		t.Error("StartSync() should clear LastError")
	}
	if state.SyncStartedAt.IsZero() {
		t.Error("StartSync() should set SyncStartedAt")
	}
	if state.ContinuationToken != "" {
		t.Error("StartSync() should clear pagination state")
	}
}

func TestSyncState_CompleteSync(t *testing.T) {
	state := &SyncState{
		ChannelID:         "ch123",
		ContinuationToken: "token",
		APIPageToken:      "page",
		Status:            SyncStatusSyncing,
		VideosProcessed:   100,
	}

	state.CompleteSync()

	if state.Status != SyncStatusIdle {
		t.Errorf("CompleteSync() Status = %v, want %v", state.Status, SyncStatusIdle)
	}
	if state.LastSyncAt.IsZero() {
		t.Error("CompleteSync() should set LastSyncAt")
	}
	if state.ContinuationToken != "" || state.APIPageToken != "" {
		t.Error("CompleteSync() should clear pagination state")
	}
}

func TestSyncState_FailSync(t *testing.T) {
	state := &SyncState{
		ChannelID:         "ch123",
		ContinuationToken: "token",
		Status:            SyncStatusSyncing,
	}

	state.FailSync("connection timeout")

	if state.Status != SyncStatusError {
		t.Errorf("FailSync() Status = %v, want %v", state.Status, SyncStatusError)
	}
	if state.LastError != "connection timeout" {
		t.Errorf("FailSync() LastError = %q, want %q", state.LastError, "connection timeout")
	}
	// Should preserve pagination state for resume
	if state.ContinuationToken != "token" {
		t.Error("FailSync() should preserve ContinuationToken for resume")
	}
}

func TestSyncState_UpdateTokens(t *testing.T) {
	t.Run("innertube token", func(t *testing.T) {
		state := &SyncState{ChannelID: "ch123"}
		state.UpdateInnertubeToken("newtoken", 2*time.Hour)

		if state.ContinuationToken != "newtoken" {
			t.Errorf("UpdateInnertubeToken() Token = %q, want %q", state.ContinuationToken, "newtoken")
		}
		if state.ContinuationExpiresAt.IsZero() {
			t.Error("UpdateInnertubeToken() should set expiry")
		}
		if state.LastPageFetchedAt.IsZero() {
			t.Error("UpdateInnertubeToken() should set LastPageFetchedAt")
		}

		// Clear token
		state.UpdateInnertubeToken("", 0)
		if !state.ContinuationExpiresAt.IsZero() {
			t.Error("UpdateInnertubeToken() with empty token should clear expiry")
		}
	})

	t.Run("api page token", func(t *testing.T) {
		state := &SyncState{ChannelID: "ch123"}
		state.UpdateAPIPageToken("pageToken", "playlistID", 10)

		if state.APIPageToken != "pageToken" {
			t.Errorf("UpdateAPIPageToken() Token = %q, want %q", state.APIPageToken, "pageToken")
		}
		if state.APIPlaylistID != "playlistID" {
			t.Errorf("UpdateAPIPageToken() PlaylistID = %q, want %q", state.APIPlaylistID, "playlistID")
		}
		if state.APIQuotaUsed != 10 {
			t.Errorf("UpdateAPIPageToken() QuotaUsed = %d, want %d", state.APIQuotaUsed, 10)
		}

		// Accumulate quota
		state.UpdateAPIPageToken("nextPage", "", 5)
		if state.APIQuotaUsed != 15 {
			t.Errorf("UpdateAPIPageToken() should accumulate quota, got %d", state.APIQuotaUsed)
		}
	})

	t.Run("rss state", func(t *testing.T) {
		state := &SyncState{ChannelID: "ch123"}
		now := time.Now()
		state.UpdateRSSState(now, false)

		if !state.NewestVideoTimestamp.Equal(now) {
			t.Errorf("UpdateRSSState() Timestamp = %v, want %v", state.NewestVideoTimestamp, now)
		}
		if state.RSSRequiresFullSync {
			t.Error("UpdateRSSState() RequiresFullSync should be false")
		}

		state.UpdateRSSState(time.Time{}, true)
		if !state.NewestVideoTimestamp.Equal(now) {
			t.Error("UpdateRSSState() with zero time should preserve existing timestamp")
		}
		if !state.RSSRequiresFullSync {
			t.Error("UpdateRSSState() RequiresFullSync should be true")
		}
	})
}

func TestSyncState_IncrementProgress(t *testing.T) {
	state := &SyncState{ChannelID: "ch123"}

	state.IncrementProgress(10, "vid1")
	if state.VideosProcessed != 10 {
		t.Errorf("IncrementProgress() VideosProcessed = %d, want 10", state.VideosProcessed)
	}
	if state.LastVideoID != "vid1" {
		t.Errorf("IncrementProgress() LastVideoID = %q, want %q", state.LastVideoID, "vid1")
	}

	state.IncrementProgress(5, "vid2")
	if state.VideosProcessed != 15 {
		t.Errorf("IncrementProgress() should accumulate, got %d", state.VideosProcessed)
	}

	// Empty video ID should not update
	state.IncrementProgress(5, "")
	if state.LastVideoID != "vid2" {
		t.Error("IncrementProgress() with empty videoID should preserve existing")
	}
}

func TestNewSyncState(t *testing.T) {
	state := NewSyncState("ch123")

	if state.ChannelID != "ch123" {
		t.Errorf("NewSyncState() ChannelID = %q, want %q", state.ChannelID, "ch123")
	}
	if state.Status != SyncStatusIdle {
		t.Errorf("NewSyncState() Status = %v, want %v", state.Status, SyncStatusIdle)
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
