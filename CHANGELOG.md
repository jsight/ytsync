# Changelog

All notable changes to ytsync will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - 2026-02-06

### Added
- **Cookie and authentication support** - New authentication options for accessing restricted content
  - `CookiesFile` - Load cookies from a Netscape format file
  - `CookiesFromBrowser` - Extract cookies directly from browser profiles (Chrome, Firefox, Safari, Edge)
  - `Username`/`Password` - Basic authentication credentials
  - `Netrc` - Use .netrc file for authentication
  - `VideoPassword` - Password-protected video access
- **Network configuration options** - Fine-grained control over network behavior
  - `Proxy` - HTTP/HTTPS/SOCKS proxy support
  - `RateLimit` - Bandwidth limiting (e.g., "50K", "4.2M")
  - `SourceAddress` - Bind to specific local IP address
  - `ForceIPv4` - Force IPv4 connections
  - `SocketTimeout` - Custom socket timeout duration
  - `ConcurrentFragments` - Parallel fragment downloads
  - `Impersonate` - Browser impersonation for anti-bot measures
- **Format discovery API** - Programmatic access to available video formats
  - `ListFormats()` - Query all available video/audio formats for a video
  - `FormatInfo` struct with resolution, codec, filesize, and quality information
  - CLI `formats` subcommand for listing formats interactively
- **Subtitle download and embedding** - Comprehensive subtitle support
  - `ListSubtitles()` - Discover available subtitle languages
  - `WriteSubtitles` - Download manual subtitles
  - `WriteAutoSubtitles` - Download auto-generated subtitles
  - `SubtitleLanguages` - Language preference list
  - `SubtitleFormat` - Output format selection (SRT, VTT, etc.)
  - `ConvertSubtitles` - Convert subtitles to target format
  - `EmbedSubtitles` - Embed subtitles into video file
- **Download behavior controls** - Advanced download management
  - `Retries` - Number of retry attempts
  - `FragmentRetries` - Retries for fragmented downloads
  - `NoOverwrites` - Skip existing files
  - `Continue` - Resume partial downloads
  - `RestrictFilenames` - ASCII-only filenames
  - `DownloadArchive` - Track downloaded videos to skip duplicates
  - `BreakOnExisting` - Stop playlist download on first existing file
  - `MaxDownloads` - Limit number of downloads from playlist
  - `ExtraArgs` - Pass arbitrary yt-dlp arguments
- **Metadata embedding options** - Enhanced metadata handling
  - `WriteThumbnail` - Download video thumbnail
  - `EmbedMetadata` - Embed metadata into video file
  - `EmbedThumbnail` - Embed thumbnail into video file
  - `EmbedChapters` - Embed chapter markers
  - `WriteInfoJSON` - Write metadata JSON file
  - `ConvertThumbnails` - Convert thumbnails to target format
- **Post-processing options** - Video manipulation after download
  - `RemuxVideo` - Remux video to different container (e.g., MKV to MP4)
  - `RecodeVideo` - Re-encode video to different format
  - `SponsorBlockMark` - Mark SponsorBlock segments in video
  - `SponsorBlockRemove` - Remove SponsorBlock segments from video
  - `FFmpegLocation` - Custom FFmpeg binary path
  - `DownloadSections` - Download specific time ranges
- **Content filtering options** - Advanced video selection
  - `MatchFilters` - Filter videos by metadata (e.g., "duration > 60")
  - `DateAfter` - Download videos uploaded after date
  - `DateBefore` - Download videos uploaded before date
  - `AgeLimit` - Skip videos with age restrictions
  - `XForwardedFor` - Bypass geo-restrictions with custom IP
  - `PlaylistItems` - Download specific playlist items (e.g., "1-5,10,15-20")
- **Format selection options** - Control format selection behavior
  - `FormatSort` - Custom format sorting criteria
  - `MergeOutputFormat` - Container format for merged video+audio
- **Enhanced CLI download command** - New flags for common operations
  - `--cookies` - Load cookies from file
  - `--cookies-from-browser` - Extract cookies from browser
  - `--embed-metadata` - Embed metadata into downloaded file
  - `--write-thumbnail` - Download and save thumbnail
  - `--no-overwrites` - Skip existing files
  - `--restrict-filenames` - Use ASCII-only filenames
  - `--download-archive` - Track downloaded videos in archive file
  - `--write-subs` - Download subtitles
  - `--sub-langs` - Subtitle language preferences
  - `--remux-video` - Remux to different container
  - `--sponsorblock-remove` - Remove SponsorBlock segments
  - `--proxy` - Use HTTP/HTTPS/SOCKS proxy
  - `--limit-rate` - Limit download bandwidth
  - `--match-filter` - Filter by metadata criteria
  - `--date-after` - Only download videos after date
- **Cookie-aware metadata and transcript APIs** - Authentication support for metadata extraction
  - `MetadataOptions` struct for passing authentication options
  - `FetchMetadataWithOptions()` - Fetch metadata with cookie/auth support for age-restricted content
  - Cookie propagation to `metadata.go` and `transcript.go` yt-dlp calls

## [1.2.0] - 2026-01-19

### Added
- **Custom filename support in DownloadOptions** - New `Filename` field for custom output filename specification (Closes #4)
  - Allows specifying custom output filenames (without extension) to avoid conflicts from reused video titles
  - Useful for applications like ragpile that use video IDs as filenames for uniqueness
  - Filename takes precedence over title-based naming when provided
  - Invalid characters are automatically sanitized
  - Comprehensive unit tests covering custom filenames with special characters, video IDs, and edge cases

## [1.1.1] - 2026-01-19

### Fixed
- **VideoInfo.Published date parsing** - Improved upload_date parsing from yt-dlp output (Closes #3)
  - Added comprehensive test coverage for YYYYMMDD format parsing
  - Verified correct handling of edge cases (old videos, invalid dates, multiple fallback sources)
  - Enhanced unit tests with 11 new test cases covering timestamp fallbacks and invalid inputs
  - Existing parsing logic verified to be working correctly for all date sources

## [1.1.0] - 2026-01-18

### Added
- **Public Download API** - New `DownloadVideo()` and `DownloadVideoWithOptions()` functions (Closes #2)
  - Download videos programmatically from Go code
  - Support for audio-only extraction (MP3) with configurable quality
  - Optional metadata JSON file alongside downloaded media
  - Custom output directory and format selection
  - Context support for cancellation and timeouts

### Changed
- Improved video format selection to use fallback chain for better compatibility
  - Default format now: `bestvideo[height<=1080]+bestaudio/best[height<=1080]/best`

## [1.0.1] - 2026-01-13

### Fixed
- Cross-platform file locking for Windows build
  - Split `filelock.go` into `filelock_unix.go` and `filelock_windows.go`
  - Uses `syscall.Flock` on Unix, `LockFileEx` on Windows

## [1.0.0] - 2026-01-12

### Initial Release

ytsync is a Go library and CLI tool for interacting with YouTube. It provides programmatic access to video listing, downloading, transcript extraction, and metadata fetching.

### Features

#### Core Functionality
- **Video Listing** - List videos from any YouTube channel
  - Support for channel IDs, channel URLs, and @handles
  - Multiple backends: yt-dlp (full history) and RSS feeds (fast, recent 15)
  - Filter by content type: videos, live streams, or both
  - Date filtering with `--since` flag

- **Transcript Extraction** - Extract captions with timestamps
  - Support for manual and auto-generated captions
  - Language preference with fallback chain
  - Multiple output formats (JSON3, VTT, SRT, TTML, plain text)

- **Video Download** - Download videos with metadata
  - Full video or audio-only (MP3) extraction
  - Automatic metadata JSON alongside media files
  - Configurable output directory and format

#### HTTP Client Infrastructure
- **Rate Limiting** - Token bucket rate limiter with domain isolation
  - Separate limits for Innertube API, RSS feeds, and transcripts
  - Dynamic backoff on 429/503 responses

- **Circuit Breaker** - Fault tolerance for API calls
  - Automatic circuit opening after consecutive failures
  - Half-open state for recovery testing
  - Per-domain circuit isolation

- **Connection Pooling** - Efficient HTTP connection reuse
  - Configurable pool sizes and timeouts
  - Keep-alive support

- **Session Management** - Cookie persistence and header management
  - Save/load cookies across sessions
  - User-Agent rotation support

#### YouTube API Support
- **YouTube Data API v3** - Optional official API integration
  - Quota tracking and management
  - Automatic fallback to yt-dlp when quota exhausted
  - Resumable pagination with state persistence

- **Innertube API** - Direct access to YouTube's internal API
  - Continuation token-based pagination
  - Channel browsing and video listing

#### Developer Experience
- **Go Library** - Clean, documented API for embedding
  - High-level convenience functions (`ListVideos`, `ExtractTranscript`)
  - Comprehensive error types with `errors.As()` support
  - Full godoc documentation with examples

- **Configuration** - Flexible configuration system
  - Environment variables (`YTSYNC_*`)
  - JSON config file support
  - Sensible defaults

- **Retry Logic** - Robust error handling
  - Exponential backoff with jitter
  - Configurable retry attempts and delays
  - Smart error classification (retryable vs permanent)

#### CLI Tool
- `ytsync list` - List videos from channels
- `ytsync transcript` - Extract video transcripts
- `ytsync download` - Download videos with metadata
- Tabular output format for easy parsing

### Technical Details

- **Go Version**: 1.24+
- **Dependencies**: Requires [yt-dlp](https://github.com/yt-dlp/yt-dlp) for video operations
- **Platforms**: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)

### Installation

#### As a Library
```bash
go get github.com/jsight/ytsync
```

#### CLI Binary
Download from the [releases page](https://github.com/jsight/ytsync/releases) or build from source:
```bash
git clone https://github.com/jsight/ytsync.git
cd ytsync
go build -o ytsync ./cli
```

### Quick Start

```bash
# List videos from a channel
ytsync list @Fireship
ytsync list --max 10 https://www.youtube.com/channel/UCsBjURrPoezykLs9EqgamOA

# Extract transcript
ytsync transcript dQw4w9WgXcQ --lang en

# Download video
ytsync download dQw4w9WgXcQ --dir ~/Downloads
ytsync download --audio-only dQw4w9WgXcQ
```

### Known Limitations

- Requires yt-dlp to be installed separately
- YouTube may rate limit heavy usage
- Private/unlisted videos are not accessible
- RSS feeds limited to 15 most recent videos

[1.3.0]: https://github.com/jsight/ytsync/releases/tag/v1.3.0
[1.2.0]: https://github.com/jsight/ytsync/releases/tag/v1.2.0
[1.1.1]: https://github.com/jsight/ytsync/releases/tag/v1.1.1
[1.1.0]: https://github.com/jsight/ytsync/releases/tag/v1.1.0
[1.0.1]: https://github.com/jsight/ytsync/releases/tag/v1.0.1
[1.0.0]: https://github.com/jsight/ytsync/releases/tag/v1.0.0
