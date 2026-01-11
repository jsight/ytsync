# ytsync - YouTube Video Downloader & Transcript Extractor

ytsync is a command-line tool to list, download, and extract transcripts from YouTube videos. It uses yt-dlp for robust YouTube interaction with automatic retry logic and intelligent error handling.

## Features

- **Video listing** from YouTube channels (yt-dlp or RSS feeds)
- **Video downloading** with metadata JSON (video + subtitle tracks)
- **Transcript extraction** with timestamps from any video
- **Live streams support** alongside regular videos
- **Robust retry logic** with exponential backoff + jitter
- **Configuration management** via config file or environment variables
- **Smart error reporting** for rate limits, blocks, and network issues

## Installation

### Build from source

```bash
git clone https://github.com/jsight/ytsync.git
cd ytsync
go build -o ytsync .
```

### Requirements

- Go 1.25+
- `yt-dlp` (recommended) - [install](https://github.com/yt-dlp/yt-dlp)
  - For listing videos: required
  - For downloading: required
  - For transcripts: required to fetch metadata

## Quick Start

### List videos from a channel
```bash
./ytsync https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw
./ytsync --max 10 https://www.youtube.com/@channelname
./ytsync --type both https://www.youtube.com/channel/UCxxxxx  # videos + streams
```

### Extract transcript with timestamps
```bash
./ytsync transcript dQw4w9WgXcQ
./ytsync transcript dQw4w9WgXcQ --lang en,es  # prefer English or Spanish
./ytsync transcript dQw4w9WgXcQ --no-auto     # skip auto-generated captions
```

### Download video with metadata
```bash
./ytsync download dQw4w9WgXcQ                 # download video
./ytsync download --audio-only dQw4w9WgXcQ    # MP3 only
./ytsync download --dir ~/Videos dQw4w9WgXcQ  # custom directory
```

## Commands

### list
List videos from a channel.

```bash
ytsync list [flags] <youtube-url>
ytsync [flags] <youtube-url>  # shorthand (default)
```

**Flags:**
- `-rss`: Use RSS feed (fast, 15 videos max)
- `-type`: `videos`, `streams`, or `both` (default: `videos`)
- `-max N`: Limit results to N videos
- `-since DATE`: Only videos after DATE (RFC3339 format)

**Examples:**
```bash
./ytsync https://www.youtube.com/channel/UCxxxxx
./ytsync --type both --max 25 @channelname
./ytsync --since 2024-01-15T00:00:00Z https://youtube.com/channel/UCxxxxx
./ytsync --rss UCxxxxx  # fast listing, 15 most recent
```

### transcript
Extract and display transcript with timestamps.

```bash
ytsync transcript [flags] <video-id>
```

**Flags:**
- `-lang LANGS`: Comma-separated language codes (e.g., `en,es,fr`)
- `-no-auto`: Skip auto-generated captions

**Output:**
Shows transcript with format: `[HH:MM:SS +duration] text`

**Examples:**
```bash
./ytsync transcript dQw4w9WgXcQ
./ytsync transcript dQw4w9WgXcQ --lang en
./ytsync transcript dQw4w9WgXcQ --no-auto
```

### download
Download video with metadata JSON.

```bash
ytsync download [flags] <video-id>
```

**Flags:**
- `-audio-only`: Extract audio as MP3
- `-dir PATH`: Output directory (default: `.`)
- `-format FORMAT`: Video format (default: `best[height<=1080]`)
- `-no-metadata`: Skip fetching metadata JSON

**Output:**
Creates two files:
- `video_title.mp4` (or `.mp3`, `.webm`, etc.)
- `video_title.json` (metadata with title, description, uploader, tags, etc.)

**Examples:**
```bash
./ytsync download dQw4w9WgXcQ
./ytsync download --audio-only dQw4w9WgXcQ
./ytsync download --dir ~/Downloads dQw4w9WgXcQ
./ytsync download --format best[height<=720] dQw4w9WgXcQ
```

## Configuration

Configuration is loaded in this order (highest priority first):

1. **Environment variables** - `YTSYNC_*` prefix
2. **Config file** - `ytsync.json` in current directory or `~/.config/ytsync/ytsync.json`
3. **Defaults**

### Environment Variables

```bash
# yt-dlp settings
export YTSYNC_YTDLP_PATH=/usr/local/bin/yt-dlp
export YTSYNC_YTDLP_TIMEOUT=10m

# Retry settings
export YTSYNC_MAX_RETRIES=5
export YTSYNC_INITIAL_BACKOFF=1s
export YTSYNC_MAX_BACKOFF=30s

# Extraction options
export YTSYNC_MAX_VIDEOS=100
export YTSYNC_INCLUDE_SHORTS=true
export YTSYNC_INCLUDE_LIVE=true
```

### Config File

Create `ytsync.json`:

```json
{
  "ytdlp_path": "yt-dlp",
  "ytdlp_timeout": "5m",
  "max_videos": 0,
  "include_shorts": true,
  "include_live": true,
  "max_retries": 5,
  "initial_backoff": "1s",
  "max_backoff": "30s",
  "backoff_multiplier": 2.0
}
```

See `ytsync.json.example` for a template.

## Output Formats

### List Output
Compact table with: VIDEO ID, TITLE, DURATION, VIEWS, TYPE
```
VIDEO ID      TITLE                                    DURATION  VIEWS           TYPE
dQw4w9WgXcQ   Never Gonna Give You Up                  3:32      1000000000      video
xQw4w9WgXcZ   Live Stream with QA                      0:00      250000          stream
```

### Transcript Output
Each line shows: `[timestamp +duration] text`
```
[0:01 +0:01] [♪♪♪]
[0:18 +0:03] ♪ We're no strangers to love ♪
[0:22 +0:04] ♪ You know the rules and so do I ♪
```

### Metadata (JSON)
```json
{
  "id": "dQw4w9WgXcQ",
  "title": "Rick Astley - Never Gonna Give You Up",
  "description": "...",
  "duration": 213,
  "view_count": 1731046982,
  "upload_date": "20091025",
  "uploader": "Rick Astley",
  "uploader_id": "@RickAstleyYT",
  "uploader_url": "https://www.youtube.com/@RickAstleyYT",
  "tags": ["rick astley", "music", ...],
  "categories": ["Music"],
  "fetched_at": "2024-01-10T20:15:30Z"
}
```

## Error Handling

### Smart Error Messages

The tool provides clear, actionable error messages:

**Rate Limited:**
```
Error: rate limited: too many requests to YouTube. Wait a few minutes and try again
```

**Access Denied:**
```
Error: access denied: YouTube blocked this request (rate limited or region restricted)
```

**Network Timeout:**
```
Error: Request timed out. YouTube may have blocked the request or signature expired.
Try again in a few minutes, or check if the video has captions.
```

### Automatic Retry Logic

The tool automatically retries transient failures with exponential backoff:

- **Initial delay:** 1 second
- **Max delay:** 30 seconds  
- **Multiplier:** 2x per attempt
- **Max retries:** 5 (configurable)
- **Jitter:** ±20% to prevent thundering herd

Permanent errors (channel not found, invalid URL) fail immediately.

## Architecture

```
main.go                    - CLI entry point with subcommands
├── internal/config/       - Configuration management
├── internal/retry/        - Exponential backoff retry logic
├── internal/youtube/
│   ├── lister.go         - VideoLister interface
│   ├── ytdlp.go          - yt-dlp subprocess wrapper
│   ├── rss.go            - YouTube RSS feed parser
│   ├── transcript.go      - Transcript extraction + parsing
│   └── metadata.go        - Video metadata fetching
└── internal/storage/      - (Future) Persistent storage
```

## Testing

Run tests:

```bash
go test ./...
```

With coverage:

```bash
go test ./... -cover
```

Coverage goals:
- `youtube` package: 70%+
- `retry` package: 80%+
- `config` package: N/A (simple config loading)

## Use Cases

### Content Creator Tools
- List all videos from your channel
- Batch download videos for processing
- Extract metadata for catalog management

### Research & Archival
- Download educational content
- Extract transcripts for analysis
- Create metadata indexes

### Transcription Workflows
- Download audio with `--audio-only`
- Extract existing captions as text
- Process with Whisper or other transcription tools
- Store metadata for attribution

### Monitoring
- List recent videos from channels
- Track upload dates with metadata
- Filter by date range

## Limitations

- **Requires yt-dlp:** All operations depend on yt-dlp being installed
- **Rate Limiting:** YouTube may block heavy usage; retry logic helps but limits exist
- **Live Streams:** Limited metadata available during live broadcasts
- **Shorts:** YouTube Shorts are treated as regular videos but have limited metadata
- **Private Videos:** Cannot access private/unlisted content (intentional)

## Contributing

Contributions welcome! Areas for improvement:

- [ ] YouTube Data API v3 fallback support
- [ ] Incremental sync with pagination state
- [ ] Direct subtitle file downloads (VTT, SRT)
- [ ] Batch operations (list of video IDs)
- [ ] Cloud storage integration (S3, GCS)

## License

MIT

## Disclaimer

This tool respects YouTube's terms of service. It uses public APIs/scraping for legitimate purposes only. Users are responsible for respecting copyright and terms of service when downloading content.
