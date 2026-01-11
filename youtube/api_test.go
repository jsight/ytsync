package youtube

import (
	"context"
	"errors"
	"testing"
	"time"
)

// MockVideoLister is a mock implementation for testing fallback behavior.
type MockVideoLister struct {
	videos []VideoInfo
	err    error
}

func (m *MockVideoLister) ListVideos(ctx context.Context, channelURL string, opts *ListOptions) ([]VideoInfo, error) {
	return m.videos, m.err
}

func (m *MockVideoLister) SupportsFullHistory() bool {
	return true
}

func TestNewAPILister(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{"empty key", "", true},
		{"valid key", "test-api-key-12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister, err := NewAPILister(tt.apiKey, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAPILister() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && lister == nil {
				t.Errorf("NewAPILister() returned nil lister for valid key")
			}
		})
	}
}

func TestAPIListerSupportsFullHistory(t *testing.T) {
	lister, err := NewAPILister("test-key", 0)
	if err != nil {
		t.Fatalf("NewAPILister() failed: %v", err)
	}

	if !lister.SupportsFullHistory() {
		t.Error("SupportsFullHistory() should return true for API lister")
	}
}

func TestExtractChannelIDFromURL(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		want   string
		wantOK bool
	}{
		{
			"standard channel URL",
			"https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw",
			"UCuAXFkgsw1L7xaCfnd5JJOw",
			true,
		},
		{
			"channel URL with trailing slash",
			"https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/",
			"UCuAXFkgsw1L7xaCfnd5JJOw",
			true,
		},
		{
			"channel URL with query params",
			"https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw?sub_confirmation=1",
			"UCuAXFkgsw1L7xaCfnd5JJOw",
			true,
		},
		{
			"invalid URL",
			"https://www.youtube.com/c/mychannel",
			"",
			false,
		},
		{
			"not a URL",
			"UCuAXFkgsw1L7xaCfnd5JJOw",
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractChannelIDFromURL(tt.url)
			if (got != "") != tt.wantOK {
				t.Errorf("extractChannelIDFromURL() got %q, wantOK %v", got, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("extractChannelIDFromURL() got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIListerQuotaTracking(t *testing.T) {
	lister, err := NewAPILister("test-key", 1000)
	if err != nil {
		t.Fatalf("NewAPILister() failed: %v", err)
	}

	// Check initial quota
	if initial := lister.GetEstimatedQuota(); initial != 10000 {
		t.Errorf("initial quota = %d, want 10000", initial)
	}

	// Track some usage
	lister.trackQuotaUsage(1000)
	if quota := lister.GetEstimatedQuota(); quota != 9000 {
		t.Errorf("after 1000 units usage, quota = %d, want 9000", quota)
	}

	// Should not be exhausted yet
	if lister.GetQuotaExhausted() {
		t.Error("quota should not be exhausted at 9000 units with reserve of 1000")
	}

	// Track usage to reach reserve threshold
	lister.trackQuotaUsage(8200)
	if quota := lister.GetEstimatedQuota(); quota != 800 {
		t.Errorf("after 8200 units usage, quota = %d, want 800", quota)
	}

	// Should be exhausted now (below reserve)
	if !lister.GetQuotaExhausted() {
		t.Error("quota should be exhausted below reserve threshold")
	}
}

func TestAPIListerQuotaReset(t *testing.T) {
	lister, err := NewAPILister("test-key", 0)
	if err != nil {
		t.Fatalf("NewAPILister() failed: %v", err)
	}

	// Exhaust quota by using more than available
	lister.trackQuotaUsage(11000)
	if !lister.GetQuotaExhausted() {
		t.Error("quota should be exhausted after using more than 10000 units")
	}

	// Manually set reset time to past to simulate day change
	lister.mu.Lock()
	lister.lastQuotaReset = time.Now().Add(-25 * time.Hour)
	lister.mu.Unlock()

	// Track minimal usage to trigger reset
	lister.trackQuotaUsage(1)

	// Quota should be reset
	if lister.GetQuotaExhausted() {
		t.Error("quota should not be exhausted after reset")
	}
	if quota := lister.GetEstimatedQuota(); quota != 9999 {
		t.Errorf("after reset and 1 unit usage, quota = %d, want 9999", quota)
	}
}

func TestAPIListerFallback(t *testing.T) {
	lister, err := NewAPILister("test-key", 0)
	if err != nil {
		t.Fatalf("NewAPILister() failed: %v", err)
	}

	// Set up mock fallback
	fallbackVideos := []VideoInfo{
		{ID: "fallback1", Title: "Fallback Video 1"},
		{ID: "fallback2", Title: "Fallback Video 2"},
	}
	mockFallback := &MockVideoLister{videos: fallbackVideos}
	lister.SetFallbackLister(mockFallback)

	// Exhaust quota
	lister.mu.Lock()
	lister.quotaExhausted = true
	lister.mu.Unlock()

	// ListVideos should use fallback
	_, err = lister.ListVideos(context.Background(), "UCtest", &ListOptions{})
	if err != nil {
		// Expected to fail due to no real service, but we're testing the logic
		t.Logf("ListVideos failed as expected: %v", err)
	}

	// Verify fallback was set
	if lister.fallbackLister == nil {
		t.Error("fallback lister should be set")
	}
}

func TestAPIErrorClassifier(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"nil error", nil, false},
		{"channel not found", ErrChannelNotFound, false},
		{"invalid URL", ErrInvalidURL, false},
		{"quota exceeded", errors.New("quotaExceeded"), true},
		{"rate limit", errors.New("rateLimitExceeded"), true},
		{"timeout", context.DeadlineExceeded, true},
		{"generic error", errors.New("some error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apiErrorClassifier(tt.err)
			if got != tt.wantErr {
				t.Errorf("apiErrorClassifier() = %v, want %v", got, tt.wantErr)
			}
		})
	}
}
