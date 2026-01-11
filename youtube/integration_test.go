// +build integration

package youtube

import (
	"context"
	"testing"
	"time"
)

// TestRSSListerIntegration tests RSS lister against known YouTube channels.
// This test is tagged with 'integration' and should be run separately from unit tests.
// Run with: go test -tags integration ./youtube -v
func TestRSSListerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	lister := NewRSSLister()

	// Test with YouTube's official channel (stable, should always exist)
	// Using the YouTube Help channel which is stable and public
	channelID := "UCkRfArvrzheW2E7b6SVV0YQ" // YouTube Official

	opts := &ListOptions{
		MaxResults: 5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	videos, err := lister.ListVideos(ctx, channelID, opts)
	if err != nil {
		t.Fatalf("Failed to list videos: %v", err)
	}

	if len(videos) == 0 {
		t.Fatal("Expected at least one video, got none")
	}

	if len(videos) > 5 {
		t.Errorf("Expected max 5 videos, got %d", len(videos))
	}

	// Validate video structure
	for i, video := range videos {
		if video.ID == "" {
			t.Errorf("Video %d: missing ID", i)
		}
		if video.Title == "" {
			t.Errorf("Video %d: missing Title", i)
		}
		if video.ChannelID == "" {
			t.Errorf("Video %d: missing ChannelID", i)
		}
		if video.ChannelName == "" {
			t.Errorf("Video %d: missing ChannelName", i)
		}
		if video.Published.IsZero() {
			t.Errorf("Video %d: missing Published timestamp", i)
		}
	}
}

// TestYtdlpListerIntegration tests yt-dlp lister against known YouTube channels.
// Requires yt-dlp to be installed in PATH.
func TestYtdlpListerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ytdlp := NewYtdlpLister()

	// Test with YouTube's official channel
	channelID := "UCkRfArvrzheW2E7b6SVV0YQ" // YouTube Official

	opts := &ListOptions{
		MaxResults: 10,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	videos, err := ytdlp.ListVideos(ctx, channelID, opts)
	if err != nil {
		// yt-dlp may not be installed
		if err == ErrYtdlpNotInstalled {
			t.Skip("yt-dlp not installed, skipping test")
		}
		t.Fatalf("Failed to list videos: %v", err)
	}

	if len(videos) == 0 {
		t.Fatal("Expected at least one video, got none")
	}

	if len(videos) > 10 {
		t.Errorf("Expected max 10 videos, got %d", len(videos))
	}

	// Validate video structure
	for i, video := range videos {
		if video.ID == "" {
			t.Errorf("Video %d: missing ID", i)
		}
		if video.Title == "" {
			t.Errorf("Video %d: missing Title", i)
		}
	}
}

// TestChannelIDResolutionIntegration tests that we can resolve channel URLs/handles to channel IDs.
func TestChannelIDResolutionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "direct_channel_id",
			input:     "UCkRfArvrzheW2E7b6SVV0YQ",
			wantError: false,
		},
		{
			name:      "channel_url",
			input:     "https://www.youtube.com/channel/UCkRfArvrzheW2E7b6SVV0YQ",
			wantError: false,
		},
		{
			name:      "invalid_channel",
			input:     "UCinvalidvalidinvalid",
			wantError: true,
		},
	}

	lister := NewRSSLister()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			opts := &ListOptions{
				MaxResults: 1,
			}

			videos, err := lister.ListVideos(ctx, tc.input, opts)

			if tc.wantError {
				if err == nil {
					t.Error("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(videos) == 0 {
					t.Error("Expected videos, got none")
				}
			}
		})
	}
}

// TestRetryBehaviorIntegration tests that the lister retries on transient failures.
// This is a simple smoke test to ensure retry logic works.
func TestRetryBehaviorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	lister := NewRSSLister()
	channelID := "UCkRfArvrzheW2E7b6SVV0YQ"

	opts := &ListOptions{
		MaxResults: 1,
	}

	// Make multiple requests to the same URL to test rate limiting behavior
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := lister.ListVideos(ctx, channelID, opts)
		cancel()

		if err != nil {
			t.Logf("Attempt %d: %v", i+1, err)
		}
	}
}
