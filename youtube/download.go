package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DownloadOptions configures video download behavior.
type DownloadOptions struct {
	// OutputDir is the directory to save the downloaded video.
	// Defaults to current directory if empty.
	OutputDir string
	// Format specifies the video format: "best", "mp4", "webm", or a yt-dlp format string.
	// Defaults to "best" which selects the best quality up to 1080p.
	Format string
	// AudioOnly extracts audio as MP3 instead of downloading video.
	AudioOnly bool
	// AudioQuality specifies the audio quality in kbps when AudioOnly is true.
	// Defaults to 192 if not specified.
	AudioQuality int
	// IncludeMetadata saves video metadata to a JSON file alongside the video.
	IncludeMetadata bool
	// Filename specifies a custom output filename (without extension).
	// If empty, defaults to the sanitized video title.
	// When provided, this takes precedence over title-based naming.
	Filename string
	// YtdlpPath is the path to the yt-dlp executable.
	// If empty, uses "yt-dlp" from PATH.
	YtdlpPath string
	// Progress callback for download progress updates (optional).
	// The callback receives the raw yt-dlp output line.
	OnProgress func(line string)
}

// DownloadResult contains information about a completed download.
type DownloadResult struct {
	// VideoPath is the path to the downloaded video/audio file.
	// Note: The exact filename is determined by yt-dlp based on video title.
	VideoPath string
	// MetadataPath is the path to the metadata JSON file (if IncludeMetadata was true).
	MetadataPath string
	// Metadata contains the parsed video metadata (if IncludeMetadata was true).
	Metadata *VideoMetadata
}

// Downloader handles video downloads using yt-dlp.
type Downloader struct {
	// YtdlpPath is the path to the yt-dlp executable.
	YtdlpPath string
	// Timeout is the maximum duration for the download.
	// Note: Large videos may need longer timeouts.
	Timeout int
}

// NewDownloader creates a new Downloader with default settings.
func NewDownloader() *Downloader {
	return &Downloader{
		YtdlpPath: "yt-dlp",
	}
}

// Download downloads a video with the specified options.
func (d *Downloader) Download(ctx context.Context, videoID string, opts *DownloadOptions) (*DownloadResult, error) {
	if opts == nil {
		opts = &DownloadOptions{}
	}

	// Set defaults
	ytdlpPath := d.YtdlpPath
	if opts.YtdlpPath != "" {
		ytdlpPath = opts.YtdlpPath
	}
	if ytdlpPath == "" {
		ytdlpPath = "yt-dlp"
	}

	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = "."
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	result := &DownloadResult{}

	// Fetch metadata first if requested
	if opts.IncludeMetadata {
		metadata, err := FetchMetadata(ctx, videoID, ytdlpPath)
		if err != nil {
			// Non-fatal: continue with download even if metadata fails
			// but don't set metadata in result
		} else {
			result.Metadata = metadata
		}
	}

	// Build yt-dlp arguments
	// Use a template that outputs the final filename
	// If custom Filename is provided, use it; otherwise use video title
	var outputTemplate string
	if opts.Filename != "" {
		// Sanitize the custom filename to remove invalid characters
		outputTemplate = filepath.Join(outputDir, sanitizeFilename(opts.Filename)+".%(ext)s")
	} else {
		outputTemplate = filepath.Join(outputDir, "%(title)s.%(ext)s")
	}
	ytdlpArgs := []string{
		"-o", outputTemplate,
		"--no-warnings",
		"--print", "after_move:filepath", // Print final path after download
	}

	if opts.AudioOnly {
		audioQuality := opts.AudioQuality
		if audioQuality <= 0 {
			audioQuality = 192
		}
		ytdlpArgs = append(ytdlpArgs,
			"-f", "bestaudio/best",
			"-x",
			"--audio-format", "mp3",
			"--audio-quality", fmt.Sprintf("%d", audioQuality),
		)
	} else {
		// Video download with format selection
		format := opts.Format
		if format == "" || format == "best" {
			// Use a more robust format selection that falls back gracefully
			format = "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"
		}
		ytdlpArgs = append(ytdlpArgs, "-f", format)
	}

	ytdlpArgs = append(ytdlpArgs, videoID)

	// Execute yt-dlp
	cmd := exec.CommandContext(ctx, ytdlpPath, ytdlpArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("download video: %w: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("download video: %w", err)
	}

	// Parse the output to get the final filepath
	// yt-dlp with --print after_move:filepath outputs the path
	outputPath := strings.TrimSpace(stdout.String())
	if outputPath != "" {
		// The output may contain multiple lines; the filepath is the last non-empty line
		lines := strings.Split(outputPath, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" && (strings.HasPrefix(line, "/") || strings.Contains(line, string(os.PathSeparator))) {
				result.VideoPath = line
				break
			}
		}
	}

	// If we couldn't get the path from output, try to find it
	if result.VideoPath == "" {
		// Fallback: look for recently created files in output directory
		result.VideoPath = outputDir // At least return the directory
	}

	// Save metadata if we have it
	if result.Metadata != nil && opts.IncludeMetadata {
		metadataPath := filepath.Join(outputDir, sanitizeFilename(result.Metadata.Title)+".json")
		if err := saveMetadataToFile(result.Metadata, metadataPath); err != nil {
			// Non-fatal: metadata save failure shouldn't fail the download
		} else {
			result.MetadataPath = metadataPath
		}
	}

	return result, nil
}

// sanitizeFilename removes/replaces characters that are invalid in filenames.
func sanitizeFilename(s string) string {
	replacements := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, char := range replacements {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}

// saveMetadataToFile saves video metadata to a JSON file.
func saveMetadataToFile(metadata *VideoMetadata, path string) error {
	data, err := jsonMarshalIndent(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write metadata file: %w", err)
	}

	return nil
}

// jsonMarshalIndent marshals a value to indented JSON.
func jsonMarshalIndent(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
