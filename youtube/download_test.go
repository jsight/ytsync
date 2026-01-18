package youtube

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewDownloader(t *testing.T) {
	d := NewDownloader()
	if d.YtdlpPath != "yt-dlp" {
		t.Errorf("NewDownloader().YtdlpPath = %q, want %q", d.YtdlpPath, "yt-dlp")
	}
}

func TestDownloadOptions_Defaults(t *testing.T) {
	opts := &DownloadOptions{}

	if opts.OutputDir != "" {
		t.Errorf("DownloadOptions.OutputDir default = %q, want empty", opts.OutputDir)
	}
	if opts.Format != "" {
		t.Errorf("DownloadOptions.Format default = %q, want empty", opts.Format)
	}
	if opts.AudioOnly {
		t.Error("DownloadOptions.AudioOnly default = true, want false")
	}
	if opts.AudioQuality != 0 {
		t.Errorf("DownloadOptions.AudioQuality default = %d, want 0", opts.AudioQuality)
	}
	if opts.IncludeMetadata {
		t.Error("DownloadOptions.IncludeMetadata default = true, want false")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean filename",
			input: "My Video Title",
			want:  "My Video Title",
		},
		{
			name:  "filename with forward slash",
			input: "Video/Part 1",
			want:  "Video_Part 1",
		},
		{
			name:  "filename with backslash",
			input: "Video\\Part 1",
			want:  "Video_Part 1",
		},
		{
			name:  "filename with colon",
			input: "Video: Part 1",
			want:  "Video_ Part 1",
		},
		{
			name:  "filename with multiple invalid chars",
			input: "Video: Part 1 - \"Best\" <2024>",
			want:  "Video_ Part 1 - _Best_ _2024_",
		},
		{
			name:  "filename with question mark and asterisk",
			input: "What is this? * and more",
			want:  "What is this_ _ and more",
		},
		{
			name:  "filename with pipe",
			input: "Video | Part 1",
			want:  "Video _ Part 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDownloader_Download_InvalidPath(t *testing.T) {
	d := &Downloader{
		YtdlpPath: "/nonexistent/path/to/yt-dlp",
	}

	ctx := context.Background()
	_, err := d.Download(ctx, "test123", nil)
	if err == nil {
		t.Error("expected error for non-existent yt-dlp")
	}
}

func TestDownloader_Download_WithMockYtdlp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create a mock yt-dlp script
	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "output")

	// Create mock that simulates successful download
	script := `#!/bin/sh
# Mock yt-dlp for testing
OUTPUT_DIR="` + outputDir + `"
mkdir -p "$OUTPUT_DIR"

# Check for -J flag (metadata request)
for arg in "$@"; do
    if [ "$arg" = "-J" ]; then
        cat << 'METADATA'
{
  "id": "test123",
  "title": "Test Video",
  "description": "A test video",
  "duration": 120,
  "view_count": 1000,
  "upload_date": "20250115",
  "uploader": "Test Channel",
  "uploader_id": "UCtest123",
  "uploader_url": "https://www.youtube.com/channel/UCtest123",
  "thumbnail": "https://example.com/thumb.jpg",
  "categories": ["Test"],
  "tags": ["test", "video"],
  "is_live_content": false
}
METADATA
        exit 0
    fi
done

# Simulate download - create a dummy file and print path
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{
		YtdlpPath: mockPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:       outputDir,
		IncludeMetadata: true,
	}

	result, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if result.VideoPath == "" {
		t.Error("Download() VideoPath is empty")
	}

	if result.Metadata == nil {
		t.Error("Download() Metadata is nil when IncludeMetadata=true")
	} else {
		if result.Metadata.ID != "test123" {
			t.Errorf("Metadata.ID = %q, want %q", result.Metadata.ID, "test123")
		}
		if result.Metadata.Title != "Test Video" {
			t.Errorf("Metadata.Title = %q, want %q", result.Metadata.Title, "Test Video")
		}
	}
}

func TestDownloader_Download_AudioOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "output")

	// Track what arguments are passed to verify audio-only flags
	argsFile := filepath.Join(dir, "args.txt")

	script := `#!/bin/sh
# Record args for verification
echo "$@" > "` + argsFile + `"
OUTPUT_DIR="` + outputDir + `"
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/Test Audio.mp3"
echo "$OUTPUT_DIR/Test Audio.mp3"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:    outputDir,
		AudioOnly:    true,
		AudioQuality: 320,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify the args contain audio-only flags
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "-x") {
		t.Error("expected -x flag for audio extraction")
	}
	if !strings.Contains(argsStr, "--audio-format mp3") {
		t.Error("expected --audio-format mp3 flag")
	}
	if !strings.Contains(argsStr, "--audio-quality 320") {
		t.Error("expected --audio-quality 320 flag")
	}
}

func TestDownloader_Download_CustomFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "output")
	argsFile := filepath.Join(dir, "args.txt")

	script := `#!/bin/sh
echo "$@" > "` + argsFile + `"
OUTPUT_DIR="` + outputDir + `"
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/Test Video.webm"
echo "$OUTPUT_DIR/Test Video.webm"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: outputDir,
		Format:    "best[height<=720]",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "best[height<=720]") {
		t.Errorf("expected custom format in args: %s", string(args))
	}
}

func TestDownloader_Download_ContextCancellation(t *testing.T) {
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

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: dir,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestDownloader_Download_CreatesOutputDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "nested", "output", "dir")

	script := `#!/bin/sh
touch "` + outputDir + `/Test.mp4"
echo "` + outputDir + `/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: outputDir,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Error("expected output directory to be created")
	}
}

func TestJsonMarshalIndent(t *testing.T) {
	metadata := &VideoMetadata{
		ID:    "test123",
		Title: "Test Video",
	}

	data, err := jsonMarshalIndent(metadata)
	if err != nil {
		t.Fatalf("jsonMarshalIndent() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("jsonMarshalIndent() returned empty data")
	}

	// Should contain indentation
	if !strings.Contains(string(data), "  ") {
		t.Error("expected indented JSON output")
	}
}

func TestSaveMetadataToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")

	metadata := &VideoMetadata{
		ID:          "test123",
		Title:       "Test Video",
		Description: "A test description",
	}

	err := saveMetadataToFile(metadata, path)
	if err != nil {
		t.Fatalf("saveMetadataToFile() error = %v", err)
	}

	// Verify file exists and has content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}

	if !strings.Contains(string(data), "test123") {
		t.Error("metadata file should contain video ID")
	}
	if !strings.Contains(string(data), "Test Video") {
		t.Error("metadata file should contain video title")
	}
}

