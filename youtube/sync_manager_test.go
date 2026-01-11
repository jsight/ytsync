package youtube

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
	"ytsync/storage"
)

// MockSyncStateStore implements storage.SyncStateStore for testing.
type MockSyncStateStore struct {
	states map[string]*storage.SyncState
}

func newMockSyncStateStore() *MockSyncStateStore {
	return &MockSyncStateStore{
		states: make(map[string]*storage.SyncState),
	}
}

func (m *MockSyncStateStore) GetSyncState(ctx context.Context, channelID string) (*storage.SyncState, error) {
	if state, ok := m.states[channelID]; ok {
		return state, nil
	}
	return nil, storage.ErrNotFound
}

func (m *MockSyncStateStore) UpdateSyncState(ctx context.Context, state *storage.SyncState) error {
	m.states[state.ChannelID] = state
	return nil
}

func (m *MockSyncStateStore) GetLastSync(ctx context.Context, channelID string) (time.Time, error) {
	if state, ok := m.states[channelID]; ok {
		return state.LastSyncAt, nil
	}
	return time.Time{}, storage.ErrNotFound
}

// TestSyncManagerFirstSync tests the first sync of a channel (incremental).
func TestSyncManagerFirstSync(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	rssLister := NewRSSListerWithClient(client)
	store := newMockSyncStateStore()

	sm := NewSyncManagerWithListers(rssLister, nil, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := sm.SyncChannelVideos(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", nil)
	if err != nil {
		t.Fatalf("SyncChannelVideos() error = %v", err)
	}

	if result == nil {
		t.Fatal("SyncChannelVideos() returned nil result")
	}
	if !result.IsIncremental {
		t.Error("first sync should be incremental")
	}
	if result.NewVideosCount == 0 {
		t.Errorf("NewVideosCount = 0, want > 0")
	}

	// Verify state was persisted
	state, err := store.GetSyncState(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw")
	if err != nil {
		t.Fatalf("GetSyncStateByChannelID() error = %v", err)
	}
	if state.Status != storage.SyncStatusIdle {
		t.Errorf("status = %s, want %s", state.Status, storage.SyncStatusIdle)
	}
}

// TestSyncManagerIncrementalSyncNoGap tests incremental sync when no gap is detected.
func TestSyncManagerIncrementalSyncNoGap(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	rssLister := NewRSSListerWithClient(client)
	store := newMockSyncStateStore()

	// Pre-populate store with previous sync state
	prevState := storage.NewSyncState("UCuAXFkgsw1L7xaCfnd5JJOw")
	prevState.NewestVideoTimestamp = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	prevState.Status = storage.SyncStatusIdle
	prevState.LastSyncAt = time.Now().Add(-1 * time.Hour)
	store.states["UCuAXFkgsw1L7xaCfnd5JJOw"] = prevState

	sm := NewSyncManagerWithListers(rssLister, nil, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := sm.SyncChannelVideos(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", nil)
	if err != nil {
		t.Fatalf("SyncChannelVideos() error = %v", err)
	}

	if result == nil {
		t.Fatal("SyncChannelVideos() returned nil result")
	}
	if !result.IsIncremental {
		t.Error("should be incremental sync")
	}
	if result.GapDetected {
		t.Error("gap should not be detected")
	}
}

// TestSyncManagerGapDetectionFallback tests that full sync is performed when a gap is detected.
func TestSyncManagerGapDetectionFallback(t *testing.T) {
	// RSS lister with old last sync time to trigger gap detection
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	rssLister := NewRSSListerWithClient(client)
	store := newMockSyncStateStore()

	// Simulate old sync state - last sync was 2019-12-01, but feed has videos from 2020-01-01+
	// This should trigger gap detection
	prevState := storage.NewSyncState("UCuAXFkgsw1L7xaCfnd5JJOw")
	prevState.NewestVideoTimestamp = time.Date(2019, 12, 1, 0, 0, 0, 0, time.UTC)
	prevState.Status = storage.SyncStatusIdle
	store.states["UCuAXFkgsw1L7xaCfnd5JJOw"] = prevState

	// Fallback lister that returns videos
	fallback := &mockVideoLister{
		videos: []VideoInfo{
			{
				ID:        "fallback1",
				Title:     "Fallback Video 1",
				Published: time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
			},
			{
				ID:        "fallback2",
				Title:     "Fallback Video 2",
				Published: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	sm := NewSyncManagerWithListers(rssLister, fallback, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := sm.SyncChannelVideos(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", nil)
	if err != nil {
		t.Fatalf("SyncChannelVideos() error = %v", err)
	}

	if result == nil {
		t.Fatal("SyncChannelVideos() returned nil result")
	}
	if !result.IsFullSync {
		t.Error("should have performed full sync due to gap detection")
	}
	if result.NewVideosCount == 0 {
		t.Errorf("NewVideosCount = 0, want > 0")
	}
}

// TestSyncManagerStateUpdated tests that sync state is properly updated.
func TestSyncManagerStateUpdated(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	rssLister := NewRSSListerWithClient(client)
	store := newMockSyncStateStore()

	sm := NewSyncManagerWithListers(rssLister, nil, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := sm.SyncChannelVideos(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", nil)
	if err != nil {
		t.Fatalf("SyncChannelVideos() error = %v", err)
	}

	// Verify state was updated correctly
	state, err := store.GetSyncState(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw")
	if err != nil {
		t.Fatalf("GetSyncStateByChannelID() error = %v", err)
	}

	if state.Status != storage.SyncStatusIdle {
		t.Errorf("status = %s, want idle", state.Status)
	}
	if state.LastSyncAt.IsZero() {
		t.Error("LastSyncAt should be set")
	}
	if state.Strategy != storage.StrategyRSS {
		t.Errorf("strategy = %v, want %v", state.Strategy, storage.StrategyRSS)
	}
}

// mockVideoLister is a mock implementation of VideoLister for testing.
type mockVideoLister struct {
	videos []VideoInfo
	err    error
}

func (m *mockVideoLister) ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.videos, nil
}

func (m *mockVideoLister) SupportsFullHistory() bool {
	return true
}

// TestSyncManagerChannelStatusNotFound tests getting status for non-existent channel.
func TestSyncManagerChannelStatusNotFound(t *testing.T) {
	store := newMockSyncStateStore()
	sm := NewSyncManagerWithListers(nil, nil, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := sm.ChannelSyncStatus(ctx, "UCnonexistent")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestSyncManagerChannelStatusFound tests getting status for existing channel.
func TestSyncManagerChannelStatusFound(t *testing.T) {
	store := newMockSyncStateStore()
	state := storage.NewSyncState("UCexists")
	state.LastSyncAt = time.Now()
	store.states["UCexists"] = state

	sm := NewSyncManagerWithListers(nil, nil, store)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	retrieved, err := sm.ChannelSyncStatus(ctx, "UCexists")
	if err != nil {
		t.Fatalf("ChannelSyncStatus() error = %v", err)
	}
	if retrieved.ChannelID != "UCexists" {
		t.Errorf("ChannelID = %s, want UCexists", retrieved.ChannelID)
	}
}
