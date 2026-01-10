package youtube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRSSLister_ListVideos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID := r.URL.Query().Get("channel_id")
		if channelID != "UCtest123456789012345678" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(sampleAtomFeed))
	}))
	defer server.Close()


	lister := NewRSSLister()
	lister.client = server.Client()

	// Create custom client that rewrites URLs
	lister.client.Transport = &testTransport{
		baseURL: server.URL,
	}

	ctx := context.Background()
	videos, err := lister.ListVideos(ctx, "https://www.youtube.com/channel/UCtest123456789012345678", nil)
	if err != nil {
		t.Fatalf("ListVideos() error = %v", err)
	}

	if len(videos) != 2 {
		t.Errorf("ListVideos() len = %d, want 2", len(videos))
	}

	if videos[0].ID != "dQw4w9WgXcQ" {
		t.Errorf("video[0].ID = %q, want %q", videos[0].ID, "dQw4w9WgXcQ")
	}
	if videos[0].Title != "Test Video 1" {
		t.Errorf("video[0].Title = %q, want %q", videos[0].Title, "Test Video 1")
	}
	if videos[0].ChannelName != "Test Channel" {
		t.Errorf("video[0].ChannelName = %q, want %q", videos[0].ChannelName, "Test Channel")
	}
}

func TestRSSLister_SupportsFullHistory(t *testing.T) {
	lister := NewRSSLister()
	if lister.SupportsFullHistory() {
		t.Error("RSSLister.SupportsFullHistory() = true, want false")
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
			input: "UCtest123456789012345678",
			want:  "UCtest123456789012345678",
		},
		{
			name:  "channel URL",
			input: "https://www.youtube.com/channel/UCtest123456789012345678",
			want:  "UCtest123456789012345678",
		},
		{
			name:  "channel URL with trailing slash",
			input: "https://www.youtube.com/channel/UCtest123456789012345678/videos",
			want:  "UCtest123456789012345678",
		},
		{
			name:  "channel URL with query params",
			input: "https://www.youtube.com/channel/UCtest123456789012345678?sub=1",
			want:  "UCtest123456789012345678",
		},
		{
			name:    "handle not supported",
			input:   "https://www.youtube.com/@testchannel",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			input:   "not-a-url",
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
			if got != tt.want {
				t.Errorf("extractChannelID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRSSLister_Errors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			wantErr:    ErrChannelNotFound,
		},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			wantErr:    ErrRateLimited,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			lister := NewRSSLister()
			lister.client.Transport = &testTransport{baseURL: server.URL}

			ctx := context.Background()
			_, err := lister.ListVideos(ctx, "UCtest123456789012345678", nil)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ListVideos() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestFilterVideos(t *testing.T) {
	videos := []VideoInfo{
		{ID: "1", Published: time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
		{ID: "2", Published: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "3", Published: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	t.Run("MaxResults", func(t *testing.T) {
		opts := &ListOptions{MaxResults: 2}
		result := filterVideos(videos, opts)
		if len(result) != 2 {
			t.Errorf("filterVideos() len = %d, want 2", len(result))
		}
	})

	t.Run("PublishedAfter", func(t *testing.T) {
		opts := &ListOptions{
			PublishedAfter: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		}
		result := filterVideos(videos, opts)
		if len(result) != 2 {
			t.Errorf("filterVideos() len = %d, want 2", len(result))
		}
	})
}

// testTransport rewrites requests to use the test server URL.
type testTransport struct {
	baseURL string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := t.baseURL + "/feeds/videos.xml?" + req.URL.RawQuery
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(newReq)
}

const sampleAtomFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
  <title>Test Channel</title>
  <author>
    <name>Test Channel</name>
    <uri>https://www.youtube.com/channel/UCtest123456789012345678</uri>
  </author>
  <entry>
    <id>yt:video:dQw4w9WgXcQ</id>
    <yt:videoId>dQw4w9WgXcQ</yt:videoId>
    <yt:channelId>UCtest123456789012345678</yt:channelId>
    <title>Test Video 1</title>
    <published>2025-01-10T12:00:00+00:00</published>
    <updated>2025-01-10T12:00:00+00:00</updated>
    <media:group>
      <media:description>Test description 1</media:description>
      <media:thumbnail url="https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" width="480" height="360"/>
      <media:community>
        <media:statistics views="1000000"/>
      </media:community>
    </media:group>
  </entry>
  <entry>
    <id>yt:video:test123abc</id>
    <yt:videoId>test123abc</yt:videoId>
    <yt:channelId>UCtest123456789012345678</yt:channelId>
    <title>Test Video 2</title>
    <published>2025-01-09T12:00:00+00:00</published>
    <updated>2025-01-09T12:00:00+00:00</updated>
    <media:group>
      <media:description>Test description 2</media:description>
      <media:thumbnail url="https://i.ytimg.com/vi/test123abc/hqdefault.jpg" width="480" height="360"/>
      <media:community>
        <media:statistics views="500"/>
      </media:community>
    </media:group>
  </entry>
</feed>`
