package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	httpclient "ytsync/http"
)

// TimedtextClient provides direct access to YouTube's timedtext API.
// This is used as a fallback when yt-dlp is unavailable.
type TimedtextClient struct {
	httpClient *httpclient.Client
	baseURL    string
}

// NewTimedtextClient creates a new timedtext API client.
func NewTimedtextClient() *TimedtextClient {
	return &TimedtextClient{
		httpClient: httpclient.New(&httpclient.Config{
			Timeout:       30 * time.Second,
			MaxConcurrent: 10,
			UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		}),
		baseURL: "https://www.youtube.com/api/timedtext",
	}
}

// TimedtextResponse represents the raw timedtext API response.
type TimedtextResponse struct {
	Events []TimedtextEvent `json:"events"`
}

// TimedtextEvent represents a single timed event in the timedtext response.
type TimedtextEvent struct {
	TStartMs  int64                `json:"tStartMs,string"`
	DDuration int64                `json:"dDurationMs,string"`
	Segs      []TimedtextSegment   `json:"segs,omitempty"`
	WpWinId   int                  `json:"wpWinId,omitempty"`
	Waves     []TimedtextWave      `json:"wWinId,omitempty"`
}

// TimedtextSegment represents text in a timedtext event.
type TimedtextSegment struct {
	UTF8 string `json:"utf8"`
	ACode string `json:"aCode,omitempty"`
}

// TimedtextWave is alternative wave data (not used for transcripts).
type TimedtextWave struct{}

// FetchCaptions fetches captions for a video from the timedtext API.
// This queries YouTube's /api/timedtext endpoint directly.
func (tc *TimedtextClient) FetchCaptions(ctx context.Context, videoID string, langCode string) ([]TranscriptEntry, error) {
	if videoID == "" {
		return nil, fmt.Errorf("video ID is required")
	}
	if langCode == "" {
		langCode = "en"
	}

	// Build query parameters
	params := url.Values{}
	params.Set("v", videoID)
	params.Set("lang", langCode)

	apiURL := fmt.Sprintf("%s?%s", tc.baseURL, params.Encode())

	// Fetch caption data
	response, err := tc.httpClient.Get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("timedtext request failed: %w", err)
	}

	// Check status code
	switch response.StatusCode {
	case 200: // http.StatusOK
		// Success
	case 404: // http.StatusNotFound
		return nil, fmt.Errorf("captions not found for video %s in language %s", videoID, langCode)
	case 403: // http.StatusForbidden
		return nil, fmt.Errorf("access denied: video region restricted or captions disabled")
	case 429: // http.StatusTooManyRequests
		return nil, fmt.Errorf("rate limited by YouTube")
	default:
		return nil, fmt.Errorf("timedtext API returned status %d", response.StatusCode)
	}

	// Parse the JSON response
	entries, err := tc.parseTimedtext(response.Body) // response.Body is []byte from our custom client
	if err != nil {
		return nil, fmt.Errorf("parse timedtext response: %w", err)
	}

	return entries, nil
}

// parseTimedtext parses the timedtext JSON response.
func (tc *TimedtextClient) parseTimedtext(data []byte) ([]TranscriptEntry, error) {
	var resp TimedtextResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal timedtext JSON: %w", err)
	}

	var entries []TranscriptEntry
	for _, event := range resp.Events {
		// Skip wave events
		if len(event.Segs) == 0 {
			continue
		}

		// Combine text from segments
		var text strings.Builder
		for _, seg := range event.Segs {
			text.WriteString(seg.UTF8)
		}

		entry := TranscriptEntry{
			Start:    float64(event.TStartMs) / 1000.0,
			Duration: float64(event.DDuration) / 1000.0,
			Text:     text.String(),
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ListAvailableLanguages fetches available caption languages for a video.
// This requires parsing the video page to find language options.
func (tc *TimedtextClient) ListAvailableLanguages(ctx context.Context, videoID string) ([]LanguageInfo, error) {
	if videoID == "" {
		return nil, fmt.Errorf("video ID is required")
	}

	// Construct the timedtext tracks URL which lists available languages
	params := url.Values{}
	params.Set("v", videoID)

	apiURL := fmt.Sprintf("%s?%s", tc.baseURL, params.Encode())

	response, err := tc.httpClient.Get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch language list failed: %w", err)
	}

	// Parse the response to extract language information
	languages, err := tc.extractLanguagesFromResponse(response.Body) // response.Body is []byte from our custom client
	if err != nil {
		return nil, fmt.Errorf("extract languages: %w", err)
	}

	return languages, nil
}

// extractLanguagesFromResponse extracts language info from timedtext response.
// The timedtext endpoint returns language-specific data which we use to infer available languages.
func (tc *TimedtextClient) extractLanguagesFromResponse(data []byte) ([]LanguageInfo, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		// If we can't parse it, return empty list
		return []LanguageInfo{}, nil
	}

	// The response doesn't directly list languages, but we can detect the language from successful fetch
	// For now, return empty since language detection requires video page parsing
	return []LanguageInfo{}, nil
}

// LanguageInfo contains information about an available caption language.
type LanguageInfo struct {
	// Code is the ISO 639-1 language code (e.g., "en", "es").
	Code string
	// Name is the human-readable language name.
	Name string
	// IsAutoGenerated indicates if this is an auto-generated caption track.
	IsAutoGenerated bool
}

// Close closes the timedtext client and releases resources.
func (tc *TimedtextClient) Close() error {
	if tc.httpClient != nil {
		return tc.httpClient.Close()
	}
	return nil
}
