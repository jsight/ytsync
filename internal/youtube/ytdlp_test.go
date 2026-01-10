package youtube

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestYtdlpLister_SupportsFullHistory(t *testing.T) {
	lister := NewYtdlpLister()
	if !lister.SupportsFullHistory() {
		t.Error("YtdlpLister.SupportsFullHistory() = false, want true")
	}
}

func TestNormalizeChannelURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "channel ID only",
			input: "UCuAXFkgsw1L7xaCfnd5JJOw",
			want:  "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos",
		},
		{
			name:  "channel URL without videos",
			input: "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw",
			want:  "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos",
		},
		{
			name:  "channel URL with videos",
			input: "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos",
			want:  "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos",
		},
		{
			name:  "handle URL",
			input: "https://www.youtube.com/@testchannel",
			want:  "https://www.youtube.com/@testchannel/videos",
		},
		{
			name:  "handle URL with trailing slash",
			input: "https://www.youtube.com/@testchannel/",
			want:  "https://www.youtube.com/@testchannel/videos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeChannelURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeChannelURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseYtdlpOutput(t *testing.T) {
	data := []byte(sampleYtdlpOutput)

	videos, err := parseYtdlpOutput(data)
	if err != nil {
		t.Fatalf("parseYtdlpOutput() error = %v", err)
	}

	if len(videos) != 2 {
		t.Errorf("parseYtdlpOutput() len = %d, want 2", len(videos))
	}

	// Check first video
	v := videos[0]
	if v.ID != "dQw4w9WgXcQ" {
		t.Errorf("video.ID = %q, want %q", v.ID, "dQw4w9WgXcQ")
	}
	if v.Title != "Test Video 1" {
		t.Errorf("video.Title = %q, want %q", v.Title, "Test Video 1")
	}
	if v.Duration != 212*time.Second {
		t.Errorf("video.Duration = %v, want %v", v.Duration, 212*time.Second)
	}
	if v.ViewCount != 1000000 {
		t.Errorf("video.ViewCount = %d, want %d", v.ViewCount, 1000000)
	}
	if v.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("video.ChannelID = %q, want %q", v.ChannelID, "UCuAXFkgsw1L7xaCfnd5JJOw")
	}
}

func TestParseYtdlpDate(t *testing.T) {
	tests := []struct {
		name  string
		entry ytdlpEntry
		want  time.Time
	}{
		{
			name: "timestamp",
			entry: ytdlpEntry{
				Timestamp: 1704067200, // 2024-01-01 00:00:00 UTC
			},
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "upload_date",
			entry: ytdlpEntry{
				UploadDate: "20240115",
			},
			want: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "timestamp preferred over upload_date",
			entry: ytdlpEntry{
				Timestamp:  1704067200,
				UploadDate: "20240115",
			},
			want: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "no date",
			entry: ytdlpEntry{},
			want:  time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseYtdlpDate(tt.entry)
			if !got.Equal(tt.want) {
				t.Errorf("parseYtdlpDate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBestThumbnail(t *testing.T) {
	entry := ytdlpEntry{
		Thumbnails: []ytdlpThumbnail{
			{URL: "small.jpg", Width: 120, Height: 90},
			{URL: "medium.jpg", Width: 320, Height: 180},
			{URL: "large.jpg", Width: 1280, Height: 720},
		},
	}

	got := bestThumbnail(entry)
	if got != "large.jpg" {
		t.Errorf("bestThumbnail() = %q, want %q", got, "large.jpg")
	}

	// Test with direct thumbnail
	entry.Thumbnail = "direct.jpg"
	got = bestThumbnail(entry)
	if got != "direct.jpg" {
		t.Errorf("bestThumbnail() with direct = %q, want %q", got, "direct.jpg")
	}
}

func TestYtdlpLister_NotInstalled(t *testing.T) {
	lister := &YtdlpLister{
		Path: "/nonexistent/path/to/yt-dlp",
	}

	ctx := context.Background()
	_, err := lister.ListVideos(ctx, "https://www.youtube.com/@test", nil)
	if err == nil {
		t.Error("expected error for non-existent yt-dlp")
	}
}

// TestYtdlpLister_Integration tests with real yt-dlp if available.
// Skip if yt-dlp is not installed.
func TestYtdlpLister_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if yt-dlp is installed
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("yt-dlp not installed, skipping integration test")
	}

	// Create a mock yt-dlp script that returns sample output
	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")

	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "2024.01.01"
    exit 0
fi
cat << 'EOF'
` + sampleYtdlpOutput + `
EOF
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	lister := &YtdlpLister{
		Path:    mockPath,
		Timeout: 30 * time.Second,
	}

	ctx := context.Background()
	videos, err := lister.ListVideos(ctx, "https://www.youtube.com/@test", nil)
	if err != nil {
		t.Fatalf("ListVideos() error = %v", err)
	}

	if len(videos) != 2 {
		t.Errorf("ListVideos() len = %d, want 2", len(videos))
	}
}

const sampleYtdlpOutput = `{
  "id": "UCuAXFkgsw1L7xaCfnd5JJOw",
  "title": "Test Channel - Videos",
  "uploader": "Test Channel",
  "uploader_id": "@testchannel",
  "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
  "channel_url": "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw",
  "entries": [
    {
      "id": "dQw4w9WgXcQ",
      "title": "Test Video 1",
      "description": "This is test video 1",
      "duration": 212,
      "view_count": 1000000,
      "uploader": "Test Channel",
      "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
      "upload_date": "20250110",
      "timestamp": 1736505600,
      "thumbnail": "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg"
    },
    {
      "id": "test123abc",
      "title": "Test Video 2",
      "description": "This is test video 2",
      "duration": 300,
      "view_count": 5000,
      "uploader": "Test Channel",
      "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
      "upload_date": "20250109",
      "timestamp": 1736419200,
      "thumbnail": "https://i.ytimg.com/vi/test123abc/maxresdefault.jpg"
    }
  ]
}`
