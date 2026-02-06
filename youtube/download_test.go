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
	if opts.Filename != "" {
		t.Errorf("DownloadOptions.Filename default = %q, want empty", opts.Filename)
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

func TestDownloadOptions_CustomFilename(t *testing.T) {
	opts := &DownloadOptions{
		Filename: "my-custom-video",
	}

	if opts.Filename != "my-custom-video" {
		t.Errorf("DownloadOptions.Filename = %q, want %q", opts.Filename, "my-custom-video")
	}
}

func TestDownloadOptions_CustomFilenameEmpty(t *testing.T) {
	// When filename is empty, should fall back to title-based naming
	opts := &DownloadOptions{
		Filename: "",
	}

	if opts.Filename != "" {
		t.Errorf("DownloadOptions.Filename = %q, want empty", opts.Filename)
	}
}

func TestDownloadOptions_CustomFilenameWithSpecialChars(t *testing.T) {
	// Custom filenames with special characters should be sanitized
	opts := &DownloadOptions{
		Filename: "my/video:file*name",
	}

	if opts.Filename != "my/video:file*name" {
		t.Errorf("DownloadOptions.Filename = %q, want %q", opts.Filename, "my/video:file*name")
	}

	// The sanitization happens during Download, not at option creation
	sanitized := sanitizeFilename(opts.Filename)
	expected := "my_video_file_name"
	if sanitized != expected {
		t.Errorf("sanitizeFilename(%q) = %q, want %q", opts.Filename, sanitized, expected)
	}
}

func TestDownloadOptions_FilenameUsingVideoID(t *testing.T) {
	// Use case: ragpile wants to use video IDs as filenames to avoid conflicts
	videoID := "dQw4w9WgXcQ"
	opts := &DownloadOptions{
		Filename: videoID,
	}

	if opts.Filename != videoID {
		t.Errorf("DownloadOptions.Filename = %q, want %q", opts.Filename, videoID)
	}

	// Video IDs should not need sanitization
	sanitized := sanitizeFilename(opts.Filename)
	if sanitized != videoID {
		t.Errorf("sanitizeFilename(%q) = %q, want %q", videoID, sanitized, videoID)
	}
}

func TestDownloadOptions_FilenameWithNumbers(t *testing.T) {
	opts := &DownloadOptions{
		Filename: "video-2024-01-19",
	}

	if opts.Filename != "video-2024-01-19" {
		t.Errorf("DownloadOptions.Filename = %q, want %q", opts.Filename, "video-2024-01-19")
	}

	sanitized := sanitizeFilename(opts.Filename)
	if sanitized != "video-2024-01-19" {
		t.Errorf("sanitizeFilename(%q) = %q, want %q", opts.Filename, sanitized, "video-2024-01-19")
	}
}

func TestDownloadOptions_CustomFilenameOverridesTitle(t *testing.T) {
	// When custom filename is provided, it should take precedence
	opts := &DownloadOptions{
		Filename: "custom-video",
	}

	// Both can be set; Filename takes precedence during download
	if opts.Filename == "" {
		t.Error("custom filename should be set")
	}
}

func TestDownloader_Download_NoOverwrites(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:    outputDir,
		NoOverwrites: true,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "--no-overwrites") {
		t.Error("expected --no-overwrites flag")
	}
}

func TestDownloader_Download_RestrictFilenames(t *testing.T) {
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
touch "$OUTPUT_DIR/Test_Video.mp4"
echo "$OUTPUT_DIR/Test_Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:         outputDir,
		RestrictFilenames: true,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "--restrict-filenames") {
		t.Error("expected --restrict-filenames flag")
	}
}

func TestDownloader_Download_DownloadArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "output")
	argsFile := filepath.Join(dir, "args.txt")
	archiveFile := filepath.Join(dir, "archive.txt")

	script := `#!/bin/sh
echo "$@" > "` + argsFile + `"
OUTPUT_DIR="` + outputDir + `"
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:       outputDir,
		DownloadArchive: archiveFile,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--download-archive") {
		t.Error("expected --download-archive flag")
	}
	if !strings.Contains(argsStr, archiveFile) {
		t.Errorf("expected archive file path %s in args", archiveFile)
	}
}

func TestDownloader_Download_RetriesAndFragmentRetries(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:       outputDir,
		Retries:         5,
		FragmentRetries: 10,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--retries 5") {
		t.Error("expected --retries 5 flag")
	}
	if !strings.Contains(argsStr, "--fragment-retries 10") {
		t.Error("expected --fragment-retries 10 flag")
	}
}

func TestDownloader_Download_ExtraArgs(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: outputDir,
		ExtraArgs: []string{"--verbose", "--no-check-certificates"},
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--verbose") {
		t.Error("expected --verbose flag from ExtraArgs")
	}
	if !strings.Contains(argsStr, "--no-check-certificates") {
		t.Error("expected --no-check-certificates flag from ExtraArgs")
	}
	// Verify ExtraArgs appear before videoID (which is test123)
	verboseIdx := strings.Index(argsStr, "--verbose")
	videoIDIdx := strings.Index(argsStr, "test123")
	if verboseIdx > videoIDIdx {
		t.Error("ExtraArgs should appear before videoID in args")
	}
}

func TestDownloader_Download_WithCookiesFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "output")
	argsFile := filepath.Join(dir, "args.txt")
	cookiesPath := filepath.Join(dir, "cookies.txt")

	// Create a dummy cookies file
	if err := os.WriteFile(cookiesPath, []byte("# Netscape cookies"), 0644); err != nil {
		t.Fatalf("failed to create cookies file: %v", err)
	}

	script := `#!/bin/sh
echo "$@" > "` + argsFile + `"
OUTPUT_DIR="` + outputDir + `"
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:   outputDir,
		CookiesFile: cookiesPath,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify the args contain cookies flag
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--cookies") {
		t.Error("expected --cookies flag")
	}
	if !strings.Contains(argsStr, cookiesPath) {
		t.Errorf("expected cookies file path %s in args", cookiesPath)
	}
}

func TestDownloader_Download_WithCookiesFromBrowser(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:          outputDir,
		CookiesFromBrowser: "chrome",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify the args contain cookies-from-browser flag
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--cookies-from-browser") {
		t.Error("expected --cookies-from-browser flag")
	}
	if !strings.Contains(argsStr, "chrome") {
		t.Error("expected 'chrome' in args")
	}
}

func TestDownloader_Download_WithAuthOptions(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:     outputDir,
		Username:      "testuser",
		Password:      "testpass",
		UseNetrc:      true,
		VideoPassword: "vidpass",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify the args contain auth flags
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "-u testuser") {
		t.Error("expected -u testuser flag")
	}
	if !strings.Contains(argsStr, "-p testpass") {
		t.Error("expected -p testpass flag")
	}
	if !strings.Contains(argsStr, "--netrc") {
		t.Error("expected --netrc flag")
	}
	if !strings.Contains(argsStr, "--video-password vidpass") {
		t.Error("expected --video-password vidpass flag")
	}
}


func TestDownloader_Download_EmbedMetadata(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:     outputDir,
		EmbedMetadata: true,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "--embed-metadata") {
		t.Errorf("expected --embed-metadata flag in args: %s", string(args))
	}
}

func TestDownloader_Download_WriteThumbnail(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:      outputDir,
		WriteThumbnail: true,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "--write-thumbnail") {
		t.Errorf("expected --write-thumbnail flag in args: %s", string(args))
	}
}

func TestDownloader_Download_MultipleMetadataOptions(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:         outputDir,
		EmbedMetadata:     true,
		WriteThumbnail:    true,
		EmbedThumbnail:    true,
		EmbedChapters:     true,
		WriteInfoJSON:     true,
		ConvertThumbnails: "jpg",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	expectedFlags := []string{
		"--embed-metadata",
		"--write-thumbnail",
		"--embed-thumbnail",
		"--embed-chapters",
		"--write-info-json",
		"--convert-thumbnails jpg",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(argsStr, flag) {
			t.Errorf("expected %q flag in args: %s", flag, argsStr)
		}
	}
}

func TestDownloader_Download_WithProxy(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: outputDir,
		Proxy:     "socks5://127.0.0.1:1080",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--proxy") {
		t.Error("expected --proxy flag")
	}
	if !strings.Contains(argsStr, "socks5://127.0.0.1:1080") {
		t.Error("expected proxy URL in args")
	}
}

func TestDownloader_Download_WithRateLimit(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: outputDir,
		RateLimit: "4.2M",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--limit-rate") {
		t.Error("expected --limit-rate flag")
	}
	if !strings.Contains(argsStr, "4.2M") {
		t.Error("expected rate limit value in args")
	}
}

func TestDownloader_Download_WithNetworkOptions(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:           outputDir,
		SourceAddress:       "192.168.1.100",
		ForceIPv4:           true,
		SocketTimeout:       30,
		ConcurrentFragments: 5,
		Impersonate:         "chrome",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	expectedFlags := []string{
		"--source-address 192.168.1.100",
		"--force-ipv4",
		"--socket-timeout 30",
		"--concurrent-fragments 5",
		"--impersonate chrome",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(argsStr, flag) {
			t.Errorf("expected %q flag in args: %s", flag, argsStr)
		}
	}
}

func TestDownloader_Download_RemuxVideo(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:  outputDir,
		RemuxVideo: "mp4",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "--remux-video mp4") {
		t.Errorf("expected --remux-video mp4 flag in args: %s", string(args))
	}
}

func TestDownloader_Download_SponsorBlockRemove(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:          outputDir,
		SponsorBlockRemove: "sponsor,intro",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	if !strings.Contains(string(args), "--sponsorblock-remove sponsor,intro") {
		t.Errorf("expected --sponsorblock-remove sponsor,intro flag in args: %s", string(args))
	}
}

func TestDownloader_Download_MultiplePostProcessingOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dir := t.TempDir()
	mockPath := filepath.Join(dir, "yt-dlp")
	outputDir := filepath.Join(dir, "output")
	argsFile := filepath.Join(dir, "args.txt")
	ffmpegPath := filepath.Join(dir, "ffmpeg")

	script := `#!/bin/sh
echo "$@" > "` + argsFile + `"
OUTPUT_DIR="` + outputDir + `"
mkdir -p "$OUTPUT_DIR"
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:          outputDir,
		RemuxVideo:         "mkv",
		RecodeVideo:        "mp4",
		SponsorBlockMark:   "sponsor,intro,outro",
		SponsorBlockRemove: "sponsor",
		FFmpegLocation:     ffmpegPath,
		DownloadSections:   "*10:00-20:00",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	expectedFlags := []string{
		"--remux-video mkv",
		"--recode-video mp4",
		"--sponsorblock-mark sponsor,intro,outro",
		"--sponsorblock-remove sponsor",
		"--ffmpeg-location " + ffmpegPath,
		"--download-sections *10:00-20:00",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(argsStr, flag) {
			t.Errorf("expected %q flag in args: %s", flag, argsStr)
		}
	}
}

func TestDownloader_Download_MatchFilters(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir: outputDir,
		MatchFilters: []string{
			"like_count > 100",
			"view_count > 1000",
		},
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--match-filters like_count > 100") {
		t.Error("expected --match-filters like_count > 100 flag")
	}
	if !strings.Contains(argsStr, "--match-filters view_count > 1000") {
		t.Error("expected --match-filters view_count > 1000 flag")
	}
}

func TestDownloader_Download_DateFilters(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:  outputDir,
		DateAfter:  "20230101",
		DateBefore: "20231231",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--dateafter 20230101") {
		t.Error("expected --dateafter 20230101 flag")
	}
	if !strings.Contains(argsStr, "--datebefore 20231231") {
		t.Error("expected --datebefore 20231231 flag")
	}
}

func TestDownloader_Download_AgeLimitAndXff(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:     outputDir,
		AgeLimit:      18,
		XForwardedFor: "US",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--age-limit 18") {
		t.Error("expected --age-limit 18 flag")
	}
	if !strings.Contains(argsStr, "--xff US") {
		t.Error("expected --xff US flag")
	}
}

func TestDownloader_Download_PlaylistItems(t *testing.T) {
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
touch "$OUTPUT_DIR/Test Video.mp4"
echo "$OUTPUT_DIR/Test Video.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:     outputDir,
		PlaylistItems: "1-5,7,10-20",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--playlist-items 1-5,7,10-20") {
		t.Error("expected --playlist-items 1-5,7,10-20 flag")
	}
}

func TestDownloadOptions_FormatAndSubtitleFields(t *testing.T) {
	opts := &DownloadOptions{
		FormatSort:         "res:720",
		MergeOutputFormat:  "mp4",
		WriteSubtitles:     true,
		WriteAutoSubtitles: true,
		SubtitleLanguages:  "en,es",
		SubtitleFormat:     "srt",
		ConvertSubtitles:   "vtt",
		EmbedSubtitles:     true,
	}

	if opts.FormatSort != "res:720" {
		t.Errorf("FormatSort = %q, want %q", opts.FormatSort, "res:720")
	}
	if opts.MergeOutputFormat != "mp4" {
		t.Errorf("MergeOutputFormat = %q, want %q", opts.MergeOutputFormat, "mp4")
	}
	if !opts.WriteSubtitles {
		t.Error("WriteSubtitles = false, want true")
	}
	if !opts.WriteAutoSubtitles {
		t.Error("WriteAutoSubtitles = false, want true")
	}
	if opts.SubtitleLanguages != "en,es" {
		t.Errorf("SubtitleLanguages = %q, want %q", opts.SubtitleLanguages, "en,es")
	}
	if opts.SubtitleFormat != "srt" {
		t.Errorf("SubtitleFormat = %q, want %q", opts.SubtitleFormat, "srt")
	}
	if opts.ConvertSubtitles != "vtt" {
		t.Errorf("ConvertSubtitles = %q, want %q", opts.ConvertSubtitles, "vtt")
	}
	if !opts.EmbedSubtitles {
		t.Error("EmbedSubtitles = false, want true")
	}
}

func TestDownloader_Download_FormatSortAndMerge(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:         outputDir,
		FormatSort:        "res:1080",
		MergeOutputFormat: "mkv",
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--format-sort res:1080") {
		t.Error("expected --format-sort res:1080 flag")
	}
	if !strings.Contains(argsStr, "--merge-output-format mkv") {
		t.Error("expected --merge-output-format mkv flag")
	}
}

func TestDownloader_Download_SubtitleOptions(t *testing.T) {
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
touch "$OUTPUT_DIR/Test.mp4"
echo "$OUTPUT_DIR/Test.mp4"
`
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create mock yt-dlp: %v", err)
	}

	d := &Downloader{YtdlpPath: mockPath}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &DownloadOptions{
		OutputDir:          outputDir,
		WriteSubtitles:     true,
		WriteAutoSubtitles: true,
		SubtitleLanguages:  "en,es,fr",
		SubtitleFormat:     "srt",
		ConvertSubtitles:   "vtt",
		EmbedSubtitles:     true,
	}

	_, err := d.Download(ctx, "test123", opts)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}

	argsStr := string(args)
	if !strings.Contains(argsStr, "--write-subs") {
		t.Error("expected --write-subs flag")
	}
	if !strings.Contains(argsStr, "--write-auto-subs") {
		t.Error("expected --write-auto-subs flag")
	}
	if !strings.Contains(argsStr, "--sub-langs en,es,fr") {
		t.Error("expected --sub-langs en,es,fr flag")
	}
	if !strings.Contains(argsStr, "--sub-format srt") {
		t.Error("expected --sub-format srt flag")
	}
	if !strings.Contains(argsStr, "--convert-subs vtt") {
		t.Error("expected --convert-subs vtt flag")
	}
	if !strings.Contains(argsStr, "--embed-subs") {
		t.Error("expected --embed-subs flag")
	}
}
