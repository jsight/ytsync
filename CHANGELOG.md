# Changelog

All notable changes to ytsync will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[1.0.0]: https://github.com/jsight/ytsync/releases/tag/v1.0.0
