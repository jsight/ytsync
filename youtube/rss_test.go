package youtube

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// MockHTTPClient is a mock HTTP client for testing.
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

// newMockHTTPClient creates a mock client that returns the given body.
func newMockHTTPClient(statusCode int, body string) *http.Client {
	client := &http.Client{}
	mockTransport := &mockTransport{
		statusCode: statusCode,
		body:       body,
	}
	client.Transport = mockTransport
	return client
}

type mockTransport struct {
	statusCode int
	body       string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.body)),
		Header:     make(http.Header),
	}, nil
}

func TestRSSListerListVideos(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		channelURL  string
		wantErr     bool
		wantCount   int
		wantVideoID string
	}{
		{
			name:        "valid feed with videos",
			statusCode:  http.StatusOK,
			body:        SampleAtomFeed,
			channelURL:  "UCuAXFkgsw1L7xaCfnd5JJOw",
			wantErr:     false,
			wantCount:   2,
			wantVideoID: "dQw4w9WgXcQ",
		},
		{
			name:       "empty feed",
			statusCode: http.StatusOK,
			body:       SampleEmptyAtomFeed,
			channelURL: "UCuAXFkgsw1L7xaCfnd5JJOw",
			wantErr:    false,
			wantCount:  0,
		},
		{
			name:       "channel not found",
			statusCode: http.StatusNotFound,
			body:       "",
			channelURL: "UCuAXFkgsw1L7xaCfnd5JJOw",
			wantErr:    true,
		},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			body:       "",
			channelURL: "UCuAXFkgsw1L7xaCfnd5JJOw",
			wantErr:    true,
		},
		{
			name:       "invalid channel URL",
			statusCode: http.StatusOK,
			body:       SampleAtomFeed,
			channelURL: "invalid-url",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockHTTPClient(tt.statusCode, tt.body)
			lister := NewRSSListerWithClient(client)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			videos, err := lister.ListVideos(ctx, tt.channelURL, nil)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListVideos() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(videos) != tt.wantCount {
				t.Errorf("ListVideos() got %d videos, want %d", len(videos), tt.wantCount)
			}

			if !tt.wantErr && tt.wantCount > 0 && videos[0].ID != tt.wantVideoID {
				t.Errorf("ListVideos() first video ID = %s, want %s", videos[0].ID, tt.wantVideoID)
			}
		})
	}
}

func TestRSSListerSupportsFullHistory(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	lister := NewRSSListerWithClient(client)

	if lister.SupportsFullHistory() {
		t.Error("RSSLister.SupportsFullHistory() should return false")
	}
}

func TestExtractChannelID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "direct channel ID",
			input: "UCuAXFkgsw1L7xaCfnd5JJOw",
			want:  "UCuAXFkgsw1L7xaCfnd5JJOw",
		},
		{
			name:  "full channel URL",
			input: "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw",
			want:  "UCuAXFkgsw1L7xaCfnd5JJOw",
		},
		{
			name:  "channel URL with trailing slash",
			input: "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/",
			want:  "UCuAXFkgsw1L7xaCfnd5JJOw",
		},
		{
			name:  "channel URL with query params",
			input: "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw?sub_confirmation=1",
			want:  "UCuAXFkgsw1L7xaCfnd5JJOw",
		},
		{
			name:    "invalid URL (handle)",
			input:   "@testchannel",
			wantErr: true,
		},
		{
			name:    "invalid URL (custom name)",
			input:   "https://www.youtube.com/c/testchannel",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractChannelID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractChannelID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("extractChannelID() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestFilterVideos(t *testing.T) {
	videos := []VideoInfo{
		{
			ID:        "video1",
			Published: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "video2",
			Published: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "video3",
			Published: time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC),
		},
	}

	tests := []struct {
		name      string
		opts      *ListOptions
		wantCount int
		wantIDs   []string
	}{
		{
			name:      "no options",
			opts:      nil,
			wantCount: 3,
			wantIDs:   []string{"video1", "video2", "video3"},
		},
		{
			name:      "max results",
			opts:      &ListOptions{MaxResults: 2},
			wantCount: 2,
			wantIDs:   []string{"video1", "video2"},
		},
		{
			name:      "published after",
			opts:      &ListOptions{PublishedAfter: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
			wantCount: 2,
			wantIDs:   []string{"video2", "video3"},
		},
		{
			name: "published after and max results",
			opts: &ListOptions{
				PublishedAfter: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				MaxResults:     1,
			},
			wantCount: 1,
			wantIDs:   []string{"video2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterVideos(videos, tt.opts)
			if len(filtered) != tt.wantCount {
				t.Errorf("filterVideos() got %d videos, want %d", len(filtered), tt.wantCount)
				return
			}
			for i, v := range filtered {
				if v.ID != tt.wantIDs[i] {
					t.Errorf("filterVideos()[%d] ID = %s, want %s", i, v.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestRSSListerListVideosIncremental(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	lister := NewRSSListerWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First sync (no last sync time) - should return all videos
	result, err := lister.ListVideosIncremental(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", time.Time{}, nil)
	if err != nil {
		t.Fatalf("ListVideosIncremental() error = %v", err)
	}

	if result.TotalInFeed != 2 {
		t.Errorf("TotalInFeed = %d, want 2", result.TotalInFeed)
	}
	if result.NewVideosCount != 2 {
		t.Errorf("NewVideosCount = %d, want 2", result.NewVideosCount)
	}
	if result.GapDetected {
		t.Error("GapDetected should be false for first sync")
	}
	if result.NewestTimestamp.IsZero() {
		t.Error("NewestTimestamp should be set")
	}
	if result.OldestTimestamp.IsZero() {
		t.Error("OldestTimestamp should be set")
	}
}

func TestRSSListerIncrementalWithLastSync(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	lister := NewRSSListerWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The sample feed has videos from 2020-01-01 and 2020-01-02
	// Last sync was 2020-01-01 at 12:00 - should only return the newer video
	lastSync := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)

	result, err := lister.ListVideosIncremental(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", lastSync, nil)
	if err != nil {
		t.Fatalf("ListVideosIncremental() error = %v", err)
	}

	// Should only have 1 video (the one from 2020-01-02)
	if result.NewVideosCount != 1 {
		t.Errorf("NewVideosCount = %d, want 1", result.NewVideosCount)
	}
	if len(result.Videos) != 1 {
		t.Errorf("len(Videos) = %d, want 1", len(result.Videos))
	}
}

func TestRSSListerIncrementalGapDetection(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	lister := NewRSSListerWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The sample feed has videos from 2020-01-01 and 2020-01-02
	// Last sync was 2019-12-01 - oldest video (2020-01-01) is way after last sync
	// This means a gap should be detected (videos may have been pushed out)
	lastSync := time.Date(2019, 12, 1, 0, 0, 0, 0, time.UTC)

	result, err := lister.ListVideosIncremental(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", lastSync, nil)
	if err != nil {
		t.Fatalf("ListVideosIncremental() error = %v", err)
	}

	// Gap should be detected because the oldest video (2020-01-01) is significantly
	// after our last sync (2019-12-01) - meaning videos may have been pushed out
	if !result.GapDetected {
		t.Error("GapDetected should be true when oldest feed video is after last sync")
	}
}

func TestRSSListerIncrementalNoGap(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	lister := NewRSSListerWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The sample feed has videos from 2020-01-01 and 2020-01-02
	// Last sync was 2020-01-01 at 00:00 (same as oldest video)
	// So no gap - we're in sync with the feed
	lastSync := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	result, err := lister.ListVideosIncremental(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", lastSync, nil)
	if err != nil {
		t.Fatalf("ListVideosIncremental() error = %v", err)
	}

	// No gap should be detected
	if result.GapDetected {
		t.Error("GapDetected should be false when oldest feed video is at or before last sync")
	}
}

func TestRSSIncrementalResultShouldTriggerFullSync(t *testing.T) {
	tests := []struct {
		name   string
		result *RSSIncrementalResult
		want   bool
	}{
		{
			name:   "nil result",
			result: nil,
			want:   true,
		},
		{
			name: "gap detected",
			result: &RSSIncrementalResult{
				GapDetected: true,
			},
			want: true,
		},
		{
			name: "no gap",
			result: &RSSIncrementalResult{
				GapDetected: false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.ShouldTriggerFullSync(); got != tt.want {
				t.Errorf("ShouldTriggerFullSync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRSSListerIncrementalError(t *testing.T) {
	client := newMockHTTPClient(http.StatusNotFound, "")
	lister := NewRSSListerWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := lister.ListVideosIncremental(ctx, "UCuAXFkgsw1L7xaCfnd5JJOw", time.Time{}, nil)
	if err == nil {
		t.Error("ListVideosIncremental() should return error for 404")
	}
}

func TestRSSListerIncrementalInvalidChannel(t *testing.T) {
	client := newMockHTTPClient(http.StatusOK, SampleAtomFeed)
	lister := NewRSSListerWithClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := lister.ListVideosIncremental(ctx, "invalid-url", time.Time{}, nil)
	if err == nil {
		t.Error("ListVideosIncremental() should return error for invalid channel URL")
	}
}
