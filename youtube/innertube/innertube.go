// Package innertube provides access to YouTube's internal Innertube API
// for fetching channel video lists with continuation token-based pagination.
package innertube

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"

	ythttp "ytsync/http"
	"ytsync/retry"
)

const (
	// browseEndpoint is the Innertube API endpoint for browsing channel content.
	browseEndpoint = "https://www.youtube.com/youtubei/v1/browse"

	// defaultClientName is the client identifier for web requests.
	defaultClientName = "WEB"
	// defaultClientVersion is the client version for web requests.
	defaultClientVersion = "2.20240101.00.00"

	// defaultUserAgent mimics a standard browser.
	defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Client handles Innertube API interactions with rate limiting and retry logic.
type Client struct {
	httpClient  *ythttp.Client
	retryConfig retry.Config
}

// ClientOption configures the Innertube client.
type ClientOption func(*Client)

// WithRetryConfig sets custom retry configuration.
func WithRetryConfig(cfg retry.Config) ClientOption {
	return func(c *Client) {
		c.retryConfig = cfg
	}
}

// NewClient creates a new Innertube API client.
func NewClient(httpClient *ythttp.Client, opts ...ClientOption) *Client {
	c := &Client{
		httpClient:  httpClient,
		retryConfig: retry.DefaultConfig(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// BrowseRequest represents a request to the browse endpoint.
type BrowseRequest struct {
	Context      ClientContext `json:"context"`
	BrowseID     string        `json:"browseId,omitempty"`
	Continuation string        `json:"continuation,omitempty"`
	Params       string        `json:"params,omitempty"`
}

// ClientContext contains client identification for the API request.
type ClientContext struct {
	Client InnertubeClient `json:"client"`
}

// InnertubeClient identifies the client making the request.
type InnertubeClient struct {
	ClientName    string `json:"clientName"`
	ClientVersion string `json:"clientVersion"`
	HL            string `json:"hl"`
	GL            string `json:"gl"`
}

// BrowseResponse represents the response from the browse endpoint.
type BrowseResponse struct {
	Contents           *Contents          `json:"contents,omitempty"`
	OnResponseReceived []OnResponseAction `json:"onResponseReceivedActions,omitempty"`
	Header             *ChannelHeader     `json:"header,omitempty"`
	Metadata           *ChannelMetadata   `json:"metadata,omitempty"`
}

// Contents holds the main content structure.
type Contents struct {
	TwoColumnBrowseResultsRenderer *TwoColumnBrowseResultsRenderer `json:"twoColumnBrowseResultsRenderer,omitempty"`
}

// TwoColumnBrowseResultsRenderer is the main layout renderer.
type TwoColumnBrowseResultsRenderer struct {
	Tabs []Tab `json:"tabs,omitempty"`
}

// Tab represents a channel tab (Videos, Shorts, etc.).
type Tab struct {
	TabRenderer *TabRenderer `json:"tabRenderer,omitempty"`
}

// TabRenderer contains tab content.
type TabRenderer struct {
	Title    string      `json:"title,omitempty"`
	Selected bool        `json:"selected,omitempty"`
	Content  *TabContent `json:"content,omitempty"`
	Endpoint *Endpoint   `json:"endpoint,omitempty"`
}

// TabContent holds the content within a tab.
type TabContent struct {
	RichGridRenderer    *RichGridRenderer    `json:"richGridRenderer,omitempty"`
	SectionListRenderer *SectionListRenderer `json:"sectionListRenderer,omitempty"`
}

// RichGridRenderer displays videos in a grid layout.
type RichGridRenderer struct {
	Contents      []RichGridContent `json:"contents,omitempty"`
	Continuations []Continuation    `json:"continuations,omitempty"`
}

// SectionListRenderer displays content in sections.
type SectionListRenderer struct {
	Contents      []SectionContent `json:"contents,omitempty"`
	Continuations []Continuation   `json:"continuations,omitempty"`
}

// SectionContent holds section items.
type SectionContent struct {
	ItemSectionRenderer *ItemSectionRenderer `json:"itemSectionRenderer,omitempty"`
}

// ItemSectionRenderer renders a section of items.
type ItemSectionRenderer struct {
	Contents []ItemContent `json:"contents,omitempty"`
}

// ItemContent can be various content types.
type ItemContent struct {
	GridVideoRenderer     *GridVideoRenderer     `json:"gridVideoRenderer,omitempty"`
	VideoRenderer         *VideoRenderer         `json:"videoRenderer,omitempty"`
	PlaylistVideoRenderer *PlaylistVideoRenderer `json:"playlistVideoRenderer,omitempty"`
}

// RichGridContent holds grid items.
type RichGridContent struct {
	RichItemRenderer         *RichItemRenderer         `json:"richItemRenderer,omitempty"`
	ContinuationItemRenderer *ContinuationItemRenderer `json:"continuationItemRenderer,omitempty"`
}

// RichItemRenderer wraps video content in the grid.
type RichItemRenderer struct {
	Content *RichItemContent `json:"content,omitempty"`
}

// RichItemContent holds the actual video renderer.
type RichItemContent struct {
	VideoRenderer *VideoRenderer `json:"videoRenderer,omitempty"`
}

// ContinuationItemRenderer provides pagination tokens.
type ContinuationItemRenderer struct {
	ContinuationEndpoint *ContinuationEndpoint `json:"continuationEndpoint,omitempty"`
}

// ContinuationEndpoint contains the continuation token.
type ContinuationEndpoint struct {
	ContinuationCommand *ContinuationCommand `json:"continuationCommand,omitempty"`
}

// ContinuationCommand holds the actual token.
type ContinuationCommand struct {
	Token string `json:"token,omitempty"`
}

// Continuation represents a continuation token structure.
type Continuation struct {
	NextContinuationData *NextContinuationData `json:"nextContinuationData,omitempty"`
}

// NextContinuationData holds pagination continuation data.
type NextContinuationData struct {
	Continuation string `json:"continuation,omitempty"`
}

// OnResponseAction represents actions to take on response.
type OnResponseAction struct {
	AppendContinuationItemsAction *AppendContinuationItemsAction `json:"appendContinuationItemsAction,omitempty"`
}

// AppendContinuationItemsAction contains continuation results.
type AppendContinuationItemsAction struct {
	ContinuationItems []ContinuationItem `json:"continuationItems,omitempty"`
}

// ContinuationItem can be a video or continuation token.
type ContinuationItem struct {
	RichItemRenderer         *RichItemRenderer         `json:"richItemRenderer,omitempty"`
	ContinuationItemRenderer *ContinuationItemRenderer `json:"continuationItemRenderer,omitempty"`
	GridVideoRenderer        *GridVideoRenderer        `json:"gridVideoRenderer,omitempty"`
	PlaylistVideoRenderer    *PlaylistVideoRenderer    `json:"playlistVideoRenderer,omitempty"`
}

// VideoRenderer contains video metadata.
type VideoRenderer struct {
	VideoID            string         `json:"videoId,omitempty"`
	Title              *TextRuns      `json:"title,omitempty"`
	DescriptionSnippet *TextRuns      `json:"descriptionSnippet,omitempty"`
	Thumbnail          *ThumbnailList `json:"thumbnail,omitempty"`
	PublishedTimeText  *SimpleText    `json:"publishedTimeText,omitempty"`
	LengthText         *SimpleText    `json:"lengthText,omitempty"`
	ViewCountText      *SimpleText    `json:"viewCountText,omitempty"`
	OwnerText          *TextRuns      `json:"ownerText,omitempty"`
}

// GridVideoRenderer is similar to VideoRenderer but used in grid layouts.
type GridVideoRenderer struct {
	VideoID           string         `json:"videoId,omitempty"`
	Title             *TextRuns      `json:"title,omitempty"`
	Thumbnail         *ThumbnailList `json:"thumbnail,omitempty"`
	PublishedTimeText *SimpleText    `json:"publishedTimeText,omitempty"`
	ViewCountText     *SimpleText    `json:"viewCountText,omitempty"`
}

// PlaylistVideoRenderer represents a video in a playlist.
type PlaylistVideoRenderer struct {
	VideoID    string         `json:"videoId,omitempty"`
	Title      *TextRuns      `json:"title,omitempty"`
	Thumbnail  *ThumbnailList `json:"thumbnail,omitempty"`
	LengthText *SimpleText    `json:"lengthText,omitempty"`
	Index      *SimpleText    `json:"index,omitempty"`
}

// TextRuns contains text with optional runs for formatting.
type TextRuns struct {
	Runs       []TextRun `json:"runs,omitempty"`
	SimpleText string    `json:"simpleText,omitempty"`
}

// TextRun is a segment of text.
type TextRun struct {
	Text string `json:"text,omitempty"`
}

// SimpleText holds a simple text value.
type SimpleText struct {
	SimpleText string `json:"simpleText,omitempty"`
}

// ThumbnailList contains thumbnail images.
type ThumbnailList struct {
	Thumbnails []Thumbnail `json:"thumbnails,omitempty"`
}

// Thumbnail represents a single thumbnail.
type Thumbnail struct {
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// ChannelHeader contains channel header information.
type ChannelHeader struct {
	C4TabbedHeaderRenderer *C4TabbedHeaderRenderer `json:"c4TabbedHeaderRenderer,omitempty"`
	PageHeaderRenderer     *PageHeaderRenderer     `json:"pageHeaderRenderer,omitempty"`
}

// C4TabbedHeaderRenderer contains channel info in the header.
type C4TabbedHeaderRenderer struct {
	ChannelID string         `json:"channelId,omitempty"`
	Title     string         `json:"title,omitempty"`
	Avatar    *ThumbnailList `json:"avatar,omitempty"`
}

// PageHeaderRenderer is an alternative header structure.
type PageHeaderRenderer struct {
	Content *PageHeaderContent `json:"content,omitempty"`
}

// PageHeaderContent holds page header details.
type PageHeaderContent struct {
	PageHeaderViewModel *PageHeaderViewModel `json:"pageHeaderViewModel,omitempty"`
}

// PageHeaderViewModel contains the view model for page headers.
type PageHeaderViewModel struct {
	Title *TitleWrapper `json:"title,omitempty"`
}

// TitleWrapper wraps dynamic text.
type TitleWrapper struct {
	DynamicTextViewModel *DynamicTextViewModel `json:"dynamicTextViewModel,omitempty"`
}

// DynamicTextViewModel holds dynamic text content.
type DynamicTextViewModel struct {
	Text *TextRuns `json:"text,omitempty"`
}

// ChannelMetadata contains channel metadata.
type ChannelMetadata struct {
	ChannelMetadataRenderer *ChannelMetadataRenderer `json:"channelMetadataRenderer,omitempty"`
}

// ChannelMetadataRenderer holds channel metadata details.
type ChannelMetadataRenderer struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ExternalID  string `json:"externalId,omitempty"`
}

// Endpoint represents a navigation endpoint.
type Endpoint struct {
	BrowseEndpoint *BrowseEndpointData `json:"browseEndpoint,omitempty"`
}

// BrowseEndpointData holds browse endpoint parameters.
type BrowseEndpointData struct {
	BrowseID string `json:"browseId,omitempty"`
	Params   string `json:"params,omitempty"`
}

// GetText extracts plain text from TextRuns.
func (t *TextRuns) GetText() string {
	if t == nil {
		return ""
	}
	if t.SimpleText != "" {
		return t.SimpleText
	}
	var parts []string
	for _, run := range t.Runs {
		parts = append(parts, run.Text)
	}
	return strings.Join(parts, "")
}

// Browse fetches content from a channel or continuation token.
func (c *Client) Browse(ctx context.Context, channelID string, continuation string) (*BrowseResponse, error) {
	req := &BrowseRequest{
		Context: ClientContext{
			Client: InnertubeClient{
				ClientName:    defaultClientName,
				ClientVersion: defaultClientVersion,
				HL:            "en",
				GL:            "US",
			},
		},
	}

	if continuation != "" {
		req.Continuation = continuation
	} else {
		req.BrowseID = channelID
		// Params for the Videos tab
		req.Params = "EgZ2aWRlb3PyBgQKAjoA"
	}

	var resp *BrowseResponse
	err := retry.Do(ctx, c.retryConfig, innertubeErrorClassifier, func(ctx context.Context) error {
		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		headers := map[string]string{
			"Content-Type": "application/json",
			"User-Agent":   defaultUserAgent,
			"Origin":       "https://www.youtube.com",
			"Referer":      "https://www.youtube.com/",
		}

		httpResp, err := c.httpClient.Do(ctx, http.MethodPost, browseEndpoint, bytes.NewReader(body), headers)
		if err != nil {
			return fmt.Errorf("browse request: %w", err)
		}

		if err := json.Unmarshal(httpResp.Body, &resp); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// innertubeErrorClassifier determines if an Innertube error is retryable.
func innertubeErrorClassifier(err error) bool {
	if err == nil {
		return false
	}

	// Check for rate limit errors (429, 403, 503)
	var rateLimitErr *ythttp.RateLimitError
	if stderrors.As(err, &rateLimitErr) {
		// Rate limit and bot detection errors are retryable
		// The backoff is handled by the rate limiter
		return true
	}

	// Check for HTTP errors
	var httpErr *ythttp.HTTPError
	if stderrors.As(err, &httpErr) {
		// Retry on 5xx errors and 403 (bot detection)
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == 403
	}

	// Context errors are not retryable
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Default to retryable for transient errors
	return true
}
