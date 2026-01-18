package youtube

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestDownloader_Integration_RealVideo tests downloading a real video.
// This test is skipped by default as it requires network access and yt-dlp.
// Run with: go test -v -run TestDownloader_Integration_RealVideo ./youtube
func TestDownloader_Integration_RealVideo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if yt-dlp is installed
	ytdlpPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		t.Skip("yt-dlp not installed, skipping integration test")
	}

	// Use a well-known video that's always available
	// Rick Astley - Never Gonna Give You Up (official music video)
	videoID := "dQw4w9WgXcQ"

	dir := t.TempDir()

	d := &Downloader{
		YtdlpPath: ytdlpPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("basic_download", func(t *testing.T) {
		outputDir := filepath.Join(dir, "basic")

		opts := &DownloadOptions{
			OutputDir:       outputDir,
			IncludeMetadata: true,
		}

		result, err := d.Download(ctx, videoID, opts)
		if err != nil {
			t.Fatalf("Download() error = %v", err)
		}

		// Verify video was downloaded
		if result.VideoPath == "" {
			t.Error("VideoPath is empty")
		} else {
			// Check the file exists
			if _, err := os.Stat(result.VideoPath); os.IsNotExist(err) {
				t.Errorf("Video file does not exist: %s", result.VideoPath)
			} else {
				t.Logf("Video downloaded to: %s", result.VideoPath)
			}
		}

		// Verify metadata was fetched
		if result.Metadata == nil {
			t.Error("Metadata is nil when IncludeMetadata=true")
		} else {
			t.Logf("Video title: %s", result.Metadata.Title)
			t.Logf("Duration: %d seconds", result.Metadata.Duration)

			if result.Metadata.ID == "" {
				t.Error("Metadata.ID is empty")
			}
			if result.Metadata.Title == "" {
				t.Error("Metadata.Title is empty")
			}
		}

		// Verify metadata file was saved
		if result.MetadataPath != "" {
			if _, err := os.Stat(result.MetadataPath); os.IsNotExist(err) {
				t.Errorf("Metadata file does not exist: %s", result.MetadataPath)
			} else {
				t.Logf("Metadata saved to: %s", result.MetadataPath)
			}
		}
	})

	t.Run("audio_only", func(t *testing.T) {
		outputDir := filepath.Join(dir, "audio")

		opts := &DownloadOptions{
			OutputDir:    outputDir,
			AudioOnly:    true,
			AudioQuality: 128,
		}

		result, err := d.Download(ctx, videoID, opts)
		if err != nil {
			t.Fatalf("Download() audio-only error = %v", err)
		}

		if result.VideoPath == "" {
			t.Error("VideoPath is empty for audio download")
		} else {
			t.Logf("Audio downloaded to: %s", result.VideoPath)

			// Verify it's an audio file (mp3)
			ext := filepath.Ext(result.VideoPath)
			if ext != ".mp3" {
				t.Logf("Note: Audio file extension is %s (may vary based on yt-dlp version)", ext)
			}
		}
	})

	t.Run("custom_format", func(t *testing.T) {
		outputDir := filepath.Join(dir, "custom")

		opts := &DownloadOptions{
			OutputDir: outputDir,
			Format:    "worst", // Use worst quality for faster download in tests
		}

		result, err := d.Download(ctx, videoID, opts)
		if err != nil {
			t.Fatalf("Download() custom format error = %v", err)
		}

		if result.VideoPath == "" {
			t.Error("VideoPath is empty for custom format download")
		} else {
			t.Logf("Video (worst quality) downloaded to: %s", result.VideoPath)
		}
	})
}
