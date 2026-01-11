package innertube

import (
	"testing"
	"time"
)

func TestResolveChannelID(t *testing.T) {
	lister := &Lister{}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "bare channel ID",
			input: "UCsXVk37bltHxD1rDPwtNM8Q",
			want:  "UCsXVk37bltHxD1rDPwtNM8Q",
		},
		{
			name:  "channel URL",
			input: "https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q",
			want:  "UCsXVk37bltHxD1rDPwtNM8Q",
		},
		{
			name:  "channel URL with path",
			input: "https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q/videos",
			want:  "UCsXVk37bltHxD1rDPwtNM8Q",
		},
		{
			name:  "channel URL with query",
			input: "https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q?view=0",
			want:  "UCsXVk37bltHxD1rDPwtNM8Q",
		},
		{
			name:    "handle not implemented",
			input:   "@someuser",
			wantErr: true,
		},
		{
			name:    "handle URL not implemented",
			input:   "https://www.youtube.com/@someuser",
			wantErr: true,
		},
		{
			name:    "custom URL not implemented",
			input:   "https://www.youtube.com/c/somechannel",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			input:   "not a valid url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lister.resolveChannelID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveChannelID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveChannelID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    string
		expected time.Duration // approximate duration ago
	}{
		{"seconds", "30 seconds ago", 30 * time.Second},
		{"minute", "1 minute ago", time.Minute},
		{"minutes", "5 minutes ago", 5 * time.Minute},
		{"hour", "1 hour ago", time.Hour},
		{"hours", "3 hours ago", 3 * time.Hour},
		{"day", "1 day ago", 24 * time.Hour},
		{"days", "2 days ago", 2 * 24 * time.Hour},
		{"week", "1 week ago", 7 * 24 * time.Hour},
		{"weeks", "2 weeks ago", 2 * 7 * 24 * time.Hour},
		{"month", "1 month ago", 30 * 24 * time.Hour},
		{"months", "3 months ago", 3 * 30 * 24 * time.Hour},
		{"year", "1 year ago", 365 * 24 * time.Hour},
		{"years", "2 years ago", 2 * 365 * 24 * time.Hour},
		{"streamed", "Streamed 2 days ago", 2 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRelativeTime(tt.input)
			if result.IsZero() {
				t.Errorf("parseRelativeTime(%q) returned zero time", tt.input)
				return
			}

			expectedTime := now.Add(-tt.expected)
			diff := result.Sub(expectedTime)

			// Allow 2 second tolerance for test execution time
			if diff > 2*time.Second || diff < -2*time.Second {
				t.Errorf("parseRelativeTime(%q) = %v, expected around %v (diff: %v)",
					tt.input, result, expectedTime, diff)
			}
		})
	}

	// Test invalid input
	result := parseRelativeTime("invalid")
	if !result.IsZero() {
		t.Errorf("parseRelativeTime(invalid) should return zero time, got %v", result)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"minutes:seconds", "10:30", 10*time.Minute + 30*time.Second},
		{"hours:minutes:seconds", "1:23:45", time.Hour + 23*time.Minute + 45*time.Second},
		{"single digit minutes", "5:00", 5 * time.Minute},
		{"long video", "12:34:56", 12*time.Hour + 34*time.Minute + 56*time.Second},
		{"invalid", "invalid", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDuration(tt.input)
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseViewCount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"simple", "1234 views", 1234},
		{"with commas", "1,234,567 views", 1234567},
		{"thousands", "1.5K views", 1500},
		{"millions", "2.3M views", 2300000},
		{"billions", "1.1B views", 1100000000},
		{"view singular", "1 view", 1},
		{"no suffix", "500K views", 500000},
		{"invalid", "invalid", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseViewCount(tt.input)
			if got != tt.expected {
				t.Errorf("parseViewCount(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestVideoDataToInfo(t *testing.T) {
	data := VideoData{
		VideoID:     "abc123",
		Title:       "Test Video",
		Description: "A test video",
		Thumbnail:   "https://example.com/thumb.jpg",
		Published:   "2 days ago",
		Duration:    "10:30",
		ViewCount:   "1.5M views",
		ChannelID:   "UCtest",
		ChannelName: "Test Channel",
	}

	info := videoDataToInfo(data)

	if info.ID != "abc123" {
		t.Errorf("ID = %q, want %q", info.ID, "abc123")
	}
	if info.Title != "Test Video" {
		t.Errorf("Title = %q, want %q", info.Title, "Test Video")
	}
	if info.Description != "A test video" {
		t.Errorf("Description = %q, want %q", info.Description, "A test video")
	}
	if info.Thumbnail != "https://example.com/thumb.jpg" {
		t.Errorf("Thumbnail = %q, want expected URL", info.Thumbnail)
	}
	if info.ChannelID != "UCtest" {
		t.Errorf("ChannelID = %q, want %q", info.ChannelID, "UCtest")
	}
	if info.ChannelName != "Test Channel" {
		t.Errorf("ChannelName = %q, want %q", info.ChannelName, "Test Channel")
	}
	if info.Duration != 10*time.Minute+30*time.Second {
		t.Errorf("Duration = %v, want 10m30s", info.Duration)
	}
	if info.ViewCount != 1500000 {
		t.Errorf("ViewCount = %d, want 1500000", info.ViewCount)
	}
	if info.Published.IsZero() {
		t.Error("Published should not be zero")
	}
}

func TestListerSupportsFullHistory(t *testing.T) {
	lister := &Lister{}
	if !lister.SupportsFullHistory() {
		t.Error("Innertube lister should support full history")
	}
}
