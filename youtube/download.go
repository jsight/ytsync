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
	// IncludeMetadata saves video metadata to a JSON file alongside the video
	// by making a separate yt-dlp -J call and saving the result via Go.
	// This is distinct from WriteInfoJSON which uses yt-dlp's built-in metadata writing.
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

	// Authentication and cookies
	// CookiesFile is the path to a Netscape-format cookies file.
	CookiesFile string
	// CookiesFromBrowser extracts cookies from a browser (e.g., 'chrome', 'firefox:profile-name').
	CookiesFromBrowser string
	// Username for authentication.
	Username string
	// Password for authentication.
	Password string
	// UseNetrc uses .netrc for authentication.
	UseNetrc bool
	// NetrcLocation specifies a custom .netrc file location.
	NetrcLocation string
	// VideoPassword for password-protected videos.
	VideoPassword string

	// Metadata and thumbnail options
	// WriteThumbnail writes the thumbnail image to a file.
	WriteThumbnail bool
	// EmbedMetadata embeds metadata (title, uploader, etc.) into the video file.
	EmbedMetadata bool
	// EmbedThumbnail embeds the thumbnail in the video file as cover art.
	EmbedThumbnail bool
	// EmbedChapters embeds chapter markers from the video description into the file.
	EmbedChapters bool
	// WriteInfoJSON writes video metadata to a .info.json file using yt-dlp's
	// built-in --write-info-json flag. This produces a more comprehensive metadata
	// file than IncludeMetadata (which uses a separate yt-dlp -J call).
	WriteInfoJSON bool
	// ConvertThumbnails converts thumbnails to the specified format (e.g., "jpg", "png").
	// Empty string means no conversion.
	ConvertThumbnails string

	// Download behavior options
	// Retries specifies the number of retries for failed downloads.
	Retries int
	// FragmentRetries specifies the number of retries for failed fragments.
	FragmentRetries int
	// NoOverwrites prevents overwriting existing files.
	NoOverwrites bool
	// Continue resumes partially downloaded files.
	Continue bool
	// RestrictFilenames restricts filenames to ASCII characters.
	RestrictFilenames bool
	// DownloadArchive specifies a file path for download archive tracking.
	DownloadArchive string
	// BreakOnExisting stops downloading when encountering already-downloaded videos.
	BreakOnExisting bool
	// MaxDownloads limits the number of videos to download.
	MaxDownloads int
	// ExtraArgs specifies additional yt-dlp arguments to append.
	ExtraArgs []string

	// Format selection and merging
	// FormatSort specifies the sort order for format selection.
	// Maps to --format-sort flag.
	FormatSort string
	// MergeOutputFormat specifies the container format for merged video+audio.
	// Maps to --merge-output-format flag (e.g., "mp4", "mkv", "webm").
	MergeOutputFormat string

	// Subtitle options
	// WriteSubtitles enables downloading subtitles.
	// Maps to --write-subs flag.
	WriteSubtitles bool
	// WriteAutoSubtitles enables downloading auto-generated subtitles.
	// Maps to --write-auto-subs flag.
	WriteAutoSubtitles bool
	// SubtitleLanguages specifies comma-separated subtitle languages to download.
	// Maps to --sub-langs flag (e.g., "en,es,fr").
	SubtitleLanguages string
	// SubtitleFormat specifies the subtitle format to download.
	// Maps to --sub-format flag (e.g., "srt", "vtt").
	SubtitleFormat string
	// ConvertSubtitles specifies the format to convert subtitles to.
	// Maps to --convert-subs flag (e.g., "srt", "vtt").
	ConvertSubtitles string
	// EmbedSubtitles enables embedding subtitles into the video file.
	// Maps to --embed-subs flag.
	EmbedSubtitles bool

	// Filtering and geo-restriction options
	// MatchFilters applies generic video filters (e.g., "like_count > 100").
	// Each entry results in a separate --match-filters flag.
	MatchFilters []string
	// DateAfter downloads only videos uploaded after this date (YYYYMMDD or relative like "now-1week").
	DateAfter string
	// DateBefore downloads only videos uploaded before this date (YYYYMMDD).
	DateBefore string
	// AgeLimit downloads only videos suitable for the given age.
	AgeLimit int
	// XForwardedFor uses the given country code or CIDR block for geo-restriction bypass.
	XForwardedFor string
	// PlaylistItems specifies which playlist items to download (e.g., "1-5,7,10-20").
	PlaylistItems string

	// Post-processing options
	// RemuxVideo remuxes the video into the specified container format (e.g., "mp4", "mkv").
	// Maps to --remux-video flag.
	RemuxVideo string
	// RecodeVideo re-encodes the video to the specified format (e.g., "mp4", "webm").
	// Maps to --recode-video flag.
	RecodeVideo string
	// SponsorBlockMark marks sponsor segments with chapter markers (comma-separated categories).
	// Maps to --sponsorblock-mark flag (e.g., "sponsor,intro,outro").
	SponsorBlockMark string
	// SponsorBlockRemove removes sponsor segments from the video (comma-separated categories).
	// Maps to --sponsorblock-remove flag (e.g., "sponsor,intro").
	SponsorBlockRemove string
	// FFmpegLocation specifies the path to ffmpeg/avconv binaries.
	// Maps to --ffmpeg-location flag.
	FFmpegLocation string
	// DownloadSections specifies time ranges to download (e.g., "*10:00-20:00").
	// Maps to --download-sections flag.
	DownloadSections string

	// Network options
	// Proxy specifies a proxy URL to use (e.g., "socks5://127.0.0.1:1080").
	// Maps to --proxy flag.
	Proxy string
	// RateLimit limits download rate in bytes per second (e.g., "50K", "4.2M").
	// Maps to --limit-rate flag.
	RateLimit string
	// SourceAddress specifies client-side IP address to bind to.
	// Maps to --source-address flag.
	SourceAddress string
	// ForceIPv4 forces all connections through IPv4.
	// Maps to --force-ipv4 flag.
	ForceIPv4 bool
	// SocketTimeout specifies socket timeout in seconds.
	// Maps to --socket-timeout flag.
	SocketTimeout int
	// ConcurrentFragments specifies number of fragments to download concurrently.
	// Maps to --concurrent-fragments flag.
	ConcurrentFragments int
	// Impersonate specifies a client to impersonate (e.g., 'chrome', 'firefox').
	// Maps to --impersonate flag.
	Impersonate string
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
		// Pass cookie options to metadata fetch
		metadataOpts := &MetadataOptions{
			YtdlpPath:          ytdlpPath,
			CookiesFile:        opts.CookiesFile,
			CookiesFromBrowser: opts.CookiesFromBrowser,
		}
		metadata, err := FetchMetadataWithOptions(ctx, videoID, metadataOpts)
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

	// Authentication and cookie options
	if opts.CookiesFile != "" {
		ytdlpArgs = append(ytdlpArgs, "--cookies", opts.CookiesFile)
	}
	if opts.CookiesFromBrowser != "" {
		ytdlpArgs = append(ytdlpArgs, "--cookies-from-browser", opts.CookiesFromBrowser)
	}
	if opts.Username != "" {
		ytdlpArgs = append(ytdlpArgs, "-u", opts.Username)
	}
	if opts.Password != "" {
		ytdlpArgs = append(ytdlpArgs, "-p", opts.Password)
	}
	if opts.UseNetrc {
		ytdlpArgs = append(ytdlpArgs, "--netrc")
	}
	if opts.NetrcLocation != "" {
		ytdlpArgs = append(ytdlpArgs, "--netrc-location", opts.NetrcLocation)
	}
	if opts.VideoPassword != "" {
		ytdlpArgs = append(ytdlpArgs, "--video-password", opts.VideoPassword)
	}

	// Download behavior options
	if opts.Retries > 0 {
		ytdlpArgs = append(ytdlpArgs, "--retries", fmt.Sprintf("%d", opts.Retries))
	}
	if opts.FragmentRetries > 0 {
		ytdlpArgs = append(ytdlpArgs, "--fragment-retries", fmt.Sprintf("%d", opts.FragmentRetries))
	}
	if opts.NoOverwrites {
		ytdlpArgs = append(ytdlpArgs, "--no-overwrites")
	}
	if opts.Continue {
		ytdlpArgs = append(ytdlpArgs, "--continue")
	}
	if opts.RestrictFilenames {
		ytdlpArgs = append(ytdlpArgs, "--restrict-filenames")
	}
	if opts.DownloadArchive != "" {
		ytdlpArgs = append(ytdlpArgs, "--download-archive", opts.DownloadArchive)
	}
	if opts.BreakOnExisting {
		ytdlpArgs = append(ytdlpArgs, "--break-on-existing")
	}
	if opts.MaxDownloads > 0 {
		ytdlpArgs = append(ytdlpArgs, "--max-downloads", fmt.Sprintf("%d", opts.MaxDownloads))
	}

	// Metadata and thumbnail options
	if opts.WriteThumbnail {
		ytdlpArgs = append(ytdlpArgs, "--write-thumbnail")
	}
	if opts.EmbedMetadata {
		ytdlpArgs = append(ytdlpArgs, "--embed-metadata")
	}
	if opts.EmbedThumbnail {
		ytdlpArgs = append(ytdlpArgs, "--embed-thumbnail")
	}
	if opts.EmbedChapters {
		ytdlpArgs = append(ytdlpArgs, "--embed-chapters")
	}
	if opts.WriteInfoJSON {
		ytdlpArgs = append(ytdlpArgs, "--write-info-json")
	}
	if opts.ConvertThumbnails != "" {
		ytdlpArgs = append(ytdlpArgs, "--convert-thumbnails", opts.ConvertThumbnails)
	}

	// Format selection and merging options
	if opts.FormatSort != "" {
		ytdlpArgs = append(ytdlpArgs, "--format-sort", opts.FormatSort)
	}
	if opts.MergeOutputFormat != "" {
		ytdlpArgs = append(ytdlpArgs, "--merge-output-format", opts.MergeOutputFormat)
	}

	// Subtitle options
	if opts.WriteSubtitles {
		ytdlpArgs = append(ytdlpArgs, "--write-subs")
	}
	if opts.WriteAutoSubtitles {
		ytdlpArgs = append(ytdlpArgs, "--write-auto-subs")
	}
	if opts.SubtitleLanguages != "" {
		ytdlpArgs = append(ytdlpArgs, "--sub-langs", opts.SubtitleLanguages)
	}
	if opts.SubtitleFormat != "" {
		ytdlpArgs = append(ytdlpArgs, "--sub-format", opts.SubtitleFormat)
	}
	if opts.ConvertSubtitles != "" {
		ytdlpArgs = append(ytdlpArgs, "--convert-subs", opts.ConvertSubtitles)
	}
	if opts.EmbedSubtitles {
		ytdlpArgs = append(ytdlpArgs, "--embed-subs")
	}

	// Filtering and geo-restriction options
	for _, filter := range opts.MatchFilters {
		ytdlpArgs = append(ytdlpArgs, "--match-filters", filter)
	}
	if opts.DateAfter != "" {
		ytdlpArgs = append(ytdlpArgs, "--dateafter", opts.DateAfter)
	}
	if opts.DateBefore != "" {
		ytdlpArgs = append(ytdlpArgs, "--datebefore", opts.DateBefore)
	}
	if opts.AgeLimit > 0 {
		ytdlpArgs = append(ytdlpArgs, "--age-limit", fmt.Sprintf("%d", opts.AgeLimit))
	}
	if opts.XForwardedFor != "" {
		ytdlpArgs = append(ytdlpArgs, "--xff", opts.XForwardedFor)
	}
	if opts.PlaylistItems != "" {
		ytdlpArgs = append(ytdlpArgs, "--playlist-items", opts.PlaylistItems)
	}

	// Post-processing options
	if opts.RemuxVideo != "" {
		ytdlpArgs = append(ytdlpArgs, "--remux-video", opts.RemuxVideo)
	}
	if opts.RecodeVideo != "" {
		ytdlpArgs = append(ytdlpArgs, "--recode-video", opts.RecodeVideo)
	}
	if opts.SponsorBlockMark != "" {
		ytdlpArgs = append(ytdlpArgs, "--sponsorblock-mark", opts.SponsorBlockMark)
	}
	if opts.SponsorBlockRemove != "" {
		ytdlpArgs = append(ytdlpArgs, "--sponsorblock-remove", opts.SponsorBlockRemove)
	}
	if opts.FFmpegLocation != "" {
		ytdlpArgs = append(ytdlpArgs, "--ffmpeg-location", opts.FFmpegLocation)
	}
	if opts.DownloadSections != "" {
		ytdlpArgs = append(ytdlpArgs, "--download-sections", opts.DownloadSections)
	}

	// Network options
	if opts.Proxy != "" {
		ytdlpArgs = append(ytdlpArgs, "--proxy", opts.Proxy)
	}
	if opts.RateLimit != "" {
		ytdlpArgs = append(ytdlpArgs, "--limit-rate", opts.RateLimit)
	}
	if opts.SourceAddress != "" {
		ytdlpArgs = append(ytdlpArgs, "--source-address", opts.SourceAddress)
	}
	if opts.ForceIPv4 {
		ytdlpArgs = append(ytdlpArgs, "--force-ipv4")
	}
	if opts.SocketTimeout > 0 {
		ytdlpArgs = append(ytdlpArgs, "--socket-timeout", fmt.Sprintf("%d", opts.SocketTimeout))
	}
	if opts.ConcurrentFragments > 0 {
		ytdlpArgs = append(ytdlpArgs, "--concurrent-fragments", fmt.Sprintf("%d", opts.ConcurrentFragments))
	}
	if opts.Impersonate != "" {
		ytdlpArgs = append(ytdlpArgs, "--impersonate", opts.Impersonate)
	}

	// Append extra args before the video ID
	if len(opts.ExtraArgs) > 0 {
		ytdlpArgs = append(ytdlpArgs, opts.ExtraArgs...)
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
