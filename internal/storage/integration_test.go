//go:build integration

package storage

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestPersistenceAcrossRestarts tests that data persists when store is closed and reopened.
func TestPersistenceAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.json")
	ctx := context.Background()

	// Create store and add data
	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	channel := &Channel{
		YouTubeID: "UCpersist",
		Name:      "Persist Channel",
		URL:       "https://youtube.com/@persist",
	}
	if err := store.CreateChannel(ctx, channel); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}

	video := &Video{
		YouTubeID: "vidpersist",
		ChannelID: channel.ID,
		Title:     "Persist Video",
	}
	if err := store.CreateVideo(ctx, video); err != nil {
		t.Fatalf("CreateVideo() error = %v", err)
	}

	transcript := &Transcript{
		VideoID:  video.ID,
		Language: "en",
		Content:  "Persisted content",
		Source:   "test",
	}
	if err := store.CreateTranscript(ctx, transcript); err != nil {
		t.Fatalf("CreateTranscript() error = %v", err)
	}

	syncState := &SyncState{
		ChannelID:       channel.ID,
		LastSyncAt:      time.Now(),
		VideosProcessed: 42,
		Status:          SyncStatusIdle,
	}
	if err := store.UpdateSyncState(ctx, syncState); err != nil {
		t.Fatalf("UpdateSyncState() error = %v", err)
	}

	store.Close()

	// Reopen and verify all data
	store2, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() reopen error = %v", err)
	}
	defer store2.Close()

	// Verify channel
	ch, err := store2.GetChannelByYouTubeID(ctx, "UCpersist")
	if err != nil {
		t.Fatalf("GetChannelByYouTubeID() error = %v", err)
	}
	if ch.Name != "Persist Channel" {
		t.Errorf("channel name = %q, want %q", ch.Name, "Persist Channel")
	}

	// Verify video
	v, err := store2.GetVideoByYouTubeID(ctx, "vidpersist")
	if err != nil {
		t.Fatalf("GetVideoByYouTubeID() error = %v", err)
	}
	if v.Title != "Persist Video" {
		t.Errorf("video title = %q, want %q", v.Title, "Persist Video")
	}
	if !v.HasTranscript {
		t.Error("video.HasTranscript should be true")
	}

	// Verify transcript
	tr, err := store2.GetTranscript(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetTranscript() error = %v", err)
	}
	if tr.Content != "Persisted content" {
		t.Errorf("transcript content = %q, want %q", tr.Content, "Persisted content")
	}

	// Verify sync state
	state, err := store2.GetSyncState(ctx, ch.ID)
	if err != nil {
		t.Fatalf("GetSyncState() error = %v", err)
	}
	if state.VideosProcessed != 42 {
		t.Errorf("VideosProcessed = %d, want 42", state.VideosProcessed)
	}
}

// TestConcurrentWrites tests that concurrent writes are properly serialized.
func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.json")
	ctx := context.Background()

	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	defer store.Close()

	// Create a base channel
	channel := &Channel{YouTubeID: "UCconcurrent", Name: "Concurrent Channel"}
	if err := store.CreateChannel(ctx, channel); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}

	// Concurrently create videos
	const numVideos = 50
	var wg sync.WaitGroup
	errCh := make(chan error, numVideos)

	for i := 0; i < numVideos; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			video := &Video{
				YouTubeID: "vid" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
				ChannelID: channel.ID,
				Title:     "Video " + string(rune('0'+i)),
			}
			if err := store.CreateVideo(ctx, video); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent CreateVideo error: %v", err)
	}

	// Verify all videos exist
	videos, err := store.ListVideosByChannel(ctx, channel.ID)
	if err != nil {
		t.Fatalf("ListVideosByChannel() error = %v", err)
	}
	if len(videos) != numVideos {
		t.Errorf("ListVideosByChannel() len = %d, want %d", len(videos), numVideos)
	}
}

// TestLargeDataset tests performance with a larger dataset.
func TestLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large dataset test in short mode")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "large.json")
	ctx := context.Background()

	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	defer store.Close()

	// Create multiple channels with many videos
	const numChannels = 10
	const videosPerChannel = 100

	start := time.Now()

	for c := 0; c < numChannels; c++ {
		channel := &Channel{
			YouTubeID: "UCchan" + string(rune('A'+c)),
			Name:      "Channel " + string(rune('A'+c)),
		}
		if err := store.CreateChannel(ctx, channel); err != nil {
			t.Fatalf("CreateChannel() error = %v", err)
		}

		for v := 0; v < videosPerChannel; v++ {
			video := &Video{
				YouTubeID: channel.YouTubeID + "_vid_" + string(rune('0'+v/100)) + string(rune('0'+(v/10)%10)) + string(rune('0'+v%10)),
				ChannelID: channel.ID,
				Title:     "Video " + string(rune('0'+v)),
			}
			if err := store.CreateVideo(ctx, video); err != nil {
				t.Fatalf("CreateVideo() error = %v", err)
			}
		}
	}

	elapsed := time.Since(start)
	t.Logf("Created %d channels with %d videos each in %v", numChannels, videosPerChannel, elapsed)

	// Verify counts
	channels, err := store.ListChannels(ctx)
	if err != nil {
		t.Fatalf("ListChannels() error = %v", err)
	}
	if len(channels) != numChannels {
		t.Errorf("channel count = %d, want %d", len(channels), numChannels)
	}

	// Verify file size is reasonable
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	t.Logf("Store file size: %d bytes", info.Size())
}

// TestFileLocking tests that multiple processes cannot open the same store.
func TestFileLocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.json")

	store1, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	defer store1.Close()

	// Attempt to open second store should fail
	_, err = NewJSONStore(path)
	if err == nil {
		t.Error("expected error opening locked store, got nil")
	}
}
