package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// FormatInfo represents information about a video format.
type FormatInfo struct {
	FormatID     string  `json:"format_id"`
	Extension    string  `json:"ext"`
	Resolution   string  `json:"resolution"`
	FPS          float64 `json:"fps"`
	VideoCodec   string  `json:"vcodec"`
	AudioCodec   string  `json:"acodec"`
	Filesize     int64   `json:"filesize"`
	TotalBitrate float64 `json:"tbr"`
	AudioBitrate float64 `json:"abr"`
	VideoBitrate float64 `json:"vbr"`
	Note         string  `json:"format_note"`
}

// SubtitleInfo represents information about available subtitles.
type SubtitleInfo struct {
	Language     string `json:"language"`
	LanguageName string `json:"name"`
	IsAutomatic  bool   `json:"is_automatic"`
}

// ListFormats retrieves available video formats for a video.
func ListFormats(ctx context.Context, videoID string, ytdlpPath string) ([]FormatInfo, error) {
	if ytdlpPath == "" {
		ytdlpPath = "yt-dlp"
	}

	// Run yt-dlp with -J to get JSON output
	cmd := exec.CommandContext(ctx, ytdlpPath, "-J", videoID)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run yt-dlp: %w", err)
	}

	// Parse the JSON output
	var result struct {
		Formats []FormatInfo `json:"formats"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}

	return result.Formats, nil
}

// ListSubtitles retrieves available subtitle languages for a video.
func ListSubtitles(ctx context.Context, videoID string, ytdlpPath string) ([]SubtitleInfo, error) {
	if ytdlpPath == "" {
		ytdlpPath = "yt-dlp"
	}

	// Run yt-dlp with -J to get JSON output
	cmd := exec.CommandContext(ctx, ytdlpPath, "-J", videoID)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run yt-dlp: %w", err)
	}

	// Parse the JSON output
	var result struct {
		Subtitles          map[string]interface{} `json:"subtitles"`
		AutomaticCaptions map[string]interface{} `json:"automatic_captions"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse yt-dlp output: %w", err)
	}

	var subtitles []SubtitleInfo

	// Add manual subtitles
	for lang := range result.Subtitles {
		subtitles = append(subtitles, SubtitleInfo{
			Language:     lang,
			LanguageName: lang, // yt-dlp doesn't provide full names in the key
			IsAutomatic:  false,
		})
	}

	// Add automatic captions
	for lang := range result.AutomaticCaptions {
		subtitles = append(subtitles, SubtitleInfo{
			Language:     lang,
			LanguageName: lang,
			IsAutomatic:  true,
		})
	}

	return subtitles, nil
}
