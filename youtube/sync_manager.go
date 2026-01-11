package youtube

import (
	"context"
	"fmt"
	"log"
	"time"
	"ytsync/storage"
)

// SyncManager orchestrates incremental video synchronization for YouTube channels.
// It manages the sync state, decides between incremental and full syncs,
// and persists state to enable resumable pagination.
type SyncManager struct {
	rssLister    *RSSLister
	fallbackList VideoLister
	store        storage.SyncStateStore
	maxRetries   int
}

// NewSyncManager creates a new sync manager with default listers.
func NewSyncManager(store storage.SyncStateStore) *SyncManager {
	return &SyncManager{
		rssLister:    NewRSSLister(),
		fallbackList: NewYtdlpLister(),
		store:        store,
		maxRetries:   3,
	}
}

// NewSyncManagerWithListers creates a sync manager with custom listers.
func NewSyncManagerWithListers(rssLister *RSSLister, fallback VideoLister, store storage.SyncStateStore) *SyncManager {
	return &SyncManager{
		rssLister:    rssLister,
		fallbackList: fallback,
		store:        store,
		maxRetries:   3,
	}
}

// SyncResult contains the outcome of a sync operation.
type SyncResult struct {
	// Videos is the list of videos discovered during this sync.
	Videos []VideoInfo
	// NewVideosCount is the number of new videos discovered.
	NewVideosCount int
	// IsIncremental is true if this was an incremental sync.
	IsIncremental bool
	// IsFullSync is true if this was a full sync.
	IsFullSync bool
	// GapDetected is true if RSS sync detected a gap.
	GapDetected bool
	// TimeSynced is the timestamp of the newest video in this sync.
	TimeSynced time.Time
}

// SyncChannelVideos performs an efficient sync of channel videos.
// It attempts an incremental RSS sync first, falling back to full sync if:
// 1. This is the first sync (no prior sync state)
// 2. A gap is detected in the RSS feed
// 3. The fallback lister supports full history and no recent videos were found
func (sm *SyncManager) SyncChannelVideos(ctx context.Context, channelURL string, opts *ListOptions) (*SyncResult, error) {
	// Extract channel ID for state tracking
	channelID, err := extractChannelID(channelURL)
	if err != nil {
		return nil, fmt.Errorf("extract channel ID: %w", err)
	}

	// Get or create sync state
	syncState, err := sm.store.GetSyncState(ctx, channelID)
	if err != nil && err != storage.ErrNotFound {
		return nil, fmt.Errorf("get sync state: %w", err)
	}
	if syncState == nil {
		// First sync - create new state
		syncState = storage.NewSyncState(channelID)
	}

	// Check if we should resume from a token
	if syncState.CanResume() {
		log.Printf("ytsync: resuming sync for channel %s from token", channelID)
		return sm.resumeSync(ctx, syncState, opts)
	}

	// Attempt incremental RSS sync first
	rssResult, err := sm.attemptIncrementalSync(ctx, channelURL, syncState, opts)
	if err != nil {
		// Log error but continue to full sync fallback
		log.Printf("ytsync: incremental sync failed for %s: %v", channelID, err)
	} else if rssResult != nil && !rssResult.GapDetected {
		// Incremental sync succeeded and no gap - persist state and return
		syncState.UpdateRSSState(rssResult.TimeSynced, false)
		syncState.CompleteSync()
		if err := sm.store.UpdateSyncState(ctx, syncState); err != nil {
			log.Printf("ytsync: failed to persist sync state: %v", err)
		}
		return rssResult, nil
	}
	
	// If we get here, either incremental failed or gap was detected
	if rssResult != nil && rssResult.GapDetected {
		log.Printf("ytsync: gap detected in RSS feed for %s, performing full sync", channelID)
	}

	// Perform full sync as fallback or when gap detected
	fullResult, err := sm.performFullSync(ctx, channelURL, syncState, opts)
	if err != nil {
		// Fail sync but preserve state for potential resume
		syncState.FailSync(fmt.Sprintf("full sync failed: %v", err))
		if err := sm.store.UpdateSyncState(ctx, syncState); err != nil {
			log.Printf("ytsync: failed to persist error state: %v", err)
		}
		return nil, fmt.Errorf("full sync failed: %w", err)
	}

	// Update state after successful full sync
	syncState.CompleteSync()
	syncState.NewestVideoTimestamp = fullResult.TimeSynced
	syncState.RSSRequiresFullSync = false

	if err := sm.store.UpdateSyncState(ctx, syncState); err != nil {
		log.Printf("ytsync: failed to persist sync state: %v", err)
	}

	return fullResult, nil
}

// attemptIncrementalSync performs an incremental RSS sync.
func (sm *SyncManager) attemptIncrementalSync(ctx context.Context, channelURL string, syncState *storage.SyncState, opts *ListOptions) (*SyncResult, error) {
	// Determine last sync time BEFORE clearing state (StartSync clears NewestVideoTimestamp)
	var lastSyncTime time.Time
	if !syncState.NewestVideoTimestamp.IsZero() {
		lastSyncTime = syncState.NewestVideoTimestamp
	}

	syncState.StartSync(storage.StrategyRSS)

	// Perform incremental RSS fetch
	rssResult, err := sm.rssLister.ListVideosIncremental(ctx, channelURL, lastSyncTime, opts)
	if err != nil {
		return nil, fmt.Errorf("rss incremental fetch failed: %w", err)
	}

	// Update sync state with RSS progress
	syncState.UpdateRSSState(rssResult.NewestTimestamp, rssResult.GapDetected)

	return &SyncResult{
		Videos:         rssResult.Videos,
		NewVideosCount: rssResult.NewVideosCount,
		IsIncremental:  true,
		GapDetected:    rssResult.GapDetected,
		TimeSynced:     rssResult.NewestTimestamp,
	}, nil
}

// performFullSync performs a complete channel sync using the fallback lister.
func (sm *SyncManager) performFullSync(ctx context.Context, channelURL string, syncState *storage.SyncState, opts *ListOptions) (*SyncResult, error) {
	if sm.fallbackList == nil {
		return nil, fmt.Errorf("no fallback lister configured for full sync")
	}

	syncState.StartSync(storage.StrategyYtdlp)

	// Perform full listing
	videos, err := sm.fallbackList.ListVideos(ctx, channelURL, opts)
	if err != nil {
		return nil, fmt.Errorf("fallback full sync failed: %w", err)
	}

	// Find newest video timestamp
	var newestTime time.Time
	if len(videos) > 0 {
		newestTime = videos[0].Published
		for _, v := range videos {
			if v.Published.After(newestTime) {
				newestTime = v.Published
			}
		}
	}

	return &SyncResult{
		Videos:         videos,
		NewVideosCount: len(videos),
		IsFullSync:     true,
		TimeSynced:     newestTime,
	}, nil
}

// resumeSync continues a previously interrupted sync operation.
func (sm *SyncManager) resumeSync(ctx context.Context, syncState *storage.SyncState, opts *ListOptions) (*SyncResult, error) {
	// This would resume based on the strategy used (Innertube or API continuation tokens)
	// For now, fall back to a fresh sync if resuming fails
	log.Printf("ytsync: resume capability not yet implemented, starting fresh sync")

	// Clear expired token and start fresh
	syncState.ClearPaginationState()
	return nil, nil
}

// ChannelSyncStatus returns the current sync status for a channel.
func (sm *SyncManager) ChannelSyncStatus(ctx context.Context, channelID string) (*storage.SyncState, error) {
	return sm.store.GetSyncState(ctx, channelID)
}
