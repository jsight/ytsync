# ytsync - YouTube Channel Video Lister

ytsync is a tool to list videos from YouTube channels with robust error handling and retry logic.

## Features

- **Video listing** from YouTube channels using yt-dlp or RSS feeds
- **Exponential backoff retry** for transient failures
- **Configuration management** via config file or environment variables
- **Filtering** by date and video count

## Installation

### Build from source

```bash
git clone <repo>
cd ytsync
go build -o ytsync .
```

### Requirements

- Go 1.25+
- `yt-dlp` (for full channel history) - [install](https://github.com/yt-dlp/yt-dlp)

## Usage

### Basic listing

List all videos from a channel:

```bash
./ytsync https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw
```

Supported channel URL formats:
- `https://www.youtube.com/channel/UCxxxxxx`
- `https://www.youtube.com/c/channelname` (yt-dlp will resolve)
- Channel ID directly: `UCxxxxxx`

### Limit videos

Get only the 10 most recent videos:

```bash
./ytsync --max 10 https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw
```

### Filter by date

Only videos published after 2024-01-15:

```bash
./ytsync --since 2024-01-15T00:00:00Z https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw
```

### Use RSS feed (faster, limited to 15 videos)

For incremental syncs, RSS is faster but only returns the 15 most recent videos:

```bash
./ytsync --rss UCuAXFkgsw1L7xaCfnd5JJOw
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
export YTSYNC_MAX_VIDEOS=0
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

## Output Format

Videos are displayed in a table format:

```
VIDEO ID      TITLE                                    PUBLISHED   DURATION  VIEWS
dQw4w9WgXcQ   Never Gonna Give You Up                  2009-10-25  3:32      1000000000
xQw4w9WgXcZ   Never Gonna Let You Down                 2009-10-26  3:45      500000000

Total: 2 videos
```

## Error Handling

The tool automatically retries transient failures (network timeouts, rate limits) with exponential backoff:

- Initial delay: 1 second
- Max delay: 30 seconds
- Multiplier: 2x per attempt
- Max retries: 5 (configurable)
- Jitter: ±20% to prevent thundering herd

Permanent errors (channel not found, invalid URL) fail immediately without retrying.

## Architecture

```
main.go                    - CLI entry point
├── internal/config/       - Configuration management
├── internal/retry/        - Exponential backoff retry logic
├── internal/youtube/
│   ├── lister.go         - VideoLister interface
│   ├── ytdlp.go          - yt-dlp subprocess wrapper
│   └── rss.go            - YouTube RSS feed parser
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

## Future Features

- Transcript extraction (ytsync-3c5)
- Persistent storage of channel/video metadata
- Incremental sync with pagination state
- Direct YouTube Data API v3 integration
