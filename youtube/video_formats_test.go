package youtube

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListFormats_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")

	// Create mock yt-dlp that returns format data
	script := `#!/bin/sh
if [ "$1" = "-J" ]; then
    cat << 'EOF'
{
  "id": "test123",
  "title": "Test Video",
  "formats": [
    {
      "format_id": "137",
      "ext": "mp4",
      "resolution": "1920x1080",
      "fps": 30.0,
      "vcodec": "avc1.640028",
      "acodec": "none",
      "filesize": 12345678,
      "tbr": 2500.0,
      "vbr": 2500.0,
      "abr": 0,
      "format_note": "1080p"
    },
    {
      "format_id": "140",
      "ext": "m4a",
      "resolution": "audio only",
      "fps": null,
      "vcodec": "none",
      "acodec": "mp4a.40.2",
      "filesize": 2345678,
      "tbr": 128.0,
      "vbr": 0,
      "abr": 128.0,
      "format_note": "medium"
    }
  ]
}
EOF
    exit 0
fi
exit 1
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	formats, err := ListFormats(ctx, "test123", mockPath)
	if err != nil {
		t.Fatalf("ListFormats() error = %v", err)
	}

	if len(formats) != 2 {
		t.Fatalf("ListFormats() returned %d formats, want 2", len(formats))
	}

	// Check video format
	videoFmt := formats[0]
	if videoFmt.FormatID != "137" {
		t.Errorf("formats[0].FormatID = %q, want %q", videoFmt.FormatID, "137")
	}
	if videoFmt.Extension != "mp4" {
		t.Errorf("formats[0].Extension = %q, want %q", videoFmt.Extension, "mp4")
	}
	if videoFmt.Resolution != "1920x1080" {
		t.Errorf("formats[0].Resolution = %q, want %q", videoFmt.Resolution, "1920x1080")
	}
	if videoFmt.FPS != 30.0 {
		t.Errorf("formats[0].FPS = %f, want %f", videoFmt.FPS, 30.0)
	}
	if videoFmt.VideoCodec != "avc1.640028" {
		t.Errorf("formats[0].VideoCodec = %q, want %q", videoFmt.VideoCodec, "avc1.640028")
	}
	if videoFmt.Note != "1080p" {
		t.Errorf("formats[0].Note = %q, want %q", videoFmt.Note, "1080p")
	}

	// Check audio format
	audioFmt := formats[1]
	if audioFmt.FormatID != "140" {
		t.Errorf("formats[1].FormatID = %q, want %q", audioFmt.FormatID, "140")
	}
	if audioFmt.AudioCodec != "mp4a.40.2" {
		t.Errorf("formats[1].AudioCodec = %q, want %q", audioFmt.AudioCodec, "mp4a.40.2")
	}
	if audioFmt.AudioBitrate != 128.0 {
		t.Errorf("formats[1].AudioBitrate = %f, want %f", audioFmt.AudioBitrate, 128.0)
	}
}

func TestListFormats_InvalidPath(t *testing.T) {
	ctx := context.Background()
	_, err := ListFormats(ctx, "test123", "/nonexistent/yt-dlp")
	if err == nil {
		t.Error("expected error for non-existent yt-dlp")
	}
}

func TestListFormats_InvalidJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")

	script := `#!/bin/sh
echo "invalid json"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ListFormats(ctx, "test123", mockPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

func TestListFormats_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")

	// Create a mock that sleeps to allow context cancellation
	script := `#!/bin/sh
sleep 60
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := ListFormats(ctx, "test123", mockPath)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestListFormats_EmptyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx := context.Background()

	// Test with empty ytdlpPath - should default to "yt-dlp"
	_, err := ListFormats(ctx, "test123", "")
	// This will fail because "yt-dlp" is not in PATH in test environment
	// but it tests the default behavior
	if err == nil {
		t.Error("expected error when yt-dlp is not in PATH")
	}
	if !strings.Contains(err.Error(), "executable file not found") && !strings.Contains(err.Error(), "exit status") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListSubtitles_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")

	script := `#!/bin/sh
if [ "$1" = "-J" ]; then
    cat << 'EOF'
{
  "id": "test123",
  "title": "Test Video",
  "subtitles": {
    "en": [{"ext": "vtt", "url": "http://example.com/en.vtt"}],
    "es": [{"ext": "vtt", "url": "http://example.com/es.vtt"}]
  },
  "automatic_captions": {
    "en": [{"ext": "vtt", "url": "http://example.com/en-auto.vtt"}],
    "fr": [{"ext": "vtt", "url": "http://example.com/fr-auto.vtt"}]
  }
}
EOF
    exit 0
fi
exit 1
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	subs, err := ListSubtitles(ctx, "test123", mockPath)
	if err != nil {
		t.Fatalf("ListSubtitles() error = %v", err)
	}

	if len(subs) != 4 {
		t.Fatalf("ListSubtitles() returned %d subtitles, want 4", len(subs))
	}

	// Check that we have both manual and automatic subtitles
	manualCount := 0
	autoCount := 0
	hasEn := false
	hasEs := false
	hasFr := false

	for _, sub := range subs {
		if sub.IsAutomatic {
			autoCount++
		} else {
			manualCount++
		}
		if sub.Language == "en" {
			hasEn = true
		}
		if sub.Language == "es" {
			hasEs = true
		}
		if sub.Language == "fr" {
			hasFr = true
		}
	}

	if manualCount != 2 {
		t.Errorf("expected 2 manual subtitles, got %d", manualCount)
	}
	if autoCount != 2 {
		t.Errorf("expected 2 automatic subtitles, got %d", autoCount)
	}
	if !hasEn {
		t.Error("expected English subtitles")
	}
	if !hasEs {
		t.Error("expected Spanish subtitles")
	}
	if !hasFr {
		t.Error("expected French subtitles")
	}
}

func TestListSubtitles_NoSubtitles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")

	script := `#!/bin/sh
if [ "$1" = "-J" ]; then
    cat << 'EOF'
{
  "id": "test123",
  "title": "Test Video",
  "subtitles": {},
  "automatic_captions": {}
}
EOF
    exit 0
fi
exit 1
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	subs, err := ListSubtitles(ctx, "test123", mockPath)
	if err != nil {
		t.Fatalf("ListSubtitles() error = %v", err)
	}

	if len(subs) != 0 {
		t.Errorf("ListSubtitles() returned %d subtitles, want 0", len(subs))
	}
}

func TestListSubtitles_InvalidPath(t *testing.T) {
	ctx := context.Background()
	_, err := ListSubtitles(ctx, "test123", "/nonexistent/yt-dlp")
	if err == nil {
		t.Error("expected error for non-existent yt-dlp")
	}
}
