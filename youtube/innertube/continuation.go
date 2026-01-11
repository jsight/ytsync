package innertube

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// ContinuationState represents the state of a pagination session.
// It tracks the current continuation token and metadata for resumable syncs.
type ContinuationState struct {
	// Token is the current continuation token for the next page.
	Token string `json:"token,omitempty"`

	// ChannelID is the YouTube channel ID being paginated.
	ChannelID string `json:"channel_id"`

	// VideosRetrieved is the count of videos retrieved so far.
	VideosRetrieved int `json:"videos_retrieved"`

	// LastVideoID is the ID of the last video retrieved (for deduplication).
	LastVideoID string `json:"last_video_id,omitempty"`

	// CreatedAt is when this pagination session started.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this state was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// ExpiresAt is when this continuation token is expected to expire.
	// Innertube tokens typically expire after a few hours.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

const (
	// DefaultTokenTTL is the default time-to-live for continuation tokens.
	// Innertube tokens typically expire within 2-4 hours.
	DefaultTokenTTL = 2 * time.Hour
)

// NewContinuationState creates a new continuation state for a channel.
func NewContinuationState(channelID string) *ContinuationState {
	now := time.Now()
	return &ContinuationState{
		ChannelID: channelID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// UpdateToken sets a new continuation token and updates metadata.
func (s *ContinuationState) UpdateToken(token string, lastVideoID string) {
	s.Token = token
	s.LastVideoID = lastVideoID
	s.UpdatedAt = time.Now()

	if token != "" {
		// Set expiry based on when the token was obtained
		s.ExpiresAt = s.UpdatedAt.Add(DefaultTokenTTL)
	} else {
		// No token means pagination is complete
		s.ExpiresAt = time.Time{}
	}
}

// IncrementVideos adds to the video count.
func (s *ContinuationState) IncrementVideos(count int) {
	s.VideosRetrieved += count
	s.UpdatedAt = time.Now()
}

// HasMore returns true if there are more pages to fetch.
func (s *ContinuationState) HasMore() bool {
	return s.Token != ""
}

// IsExpired returns true if the continuation token has expired.
func (s *ContinuationState) IsExpired() bool {
	if s.Token == "" {
		return false
	}
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// Reset clears the continuation state for a fresh start.
func (s *ContinuationState) Reset() {
	s.Token = ""
	s.LastVideoID = ""
	s.VideosRetrieved = 0
	s.UpdatedAt = time.Now()
	s.ExpiresAt = time.Time{}
}

// ToJSON serializes the state to JSON for storage.
func (s *ContinuationState) ToJSON() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ContinuationStateFromJSON deserializes state from JSON.
func ContinuationStateFromJSON(data string) (*ContinuationState, error) {
	var state ContinuationState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// ExtractContinuationToken extracts the continuation token from a browse response.
// It checks multiple locations where tokens can appear in the response structure.
func ExtractContinuationToken(resp *BrowseResponse) string {
	if resp == nil {
		return ""
	}

	// Check onResponseReceivedActions (used for continuation requests)
	for _, action := range resp.OnResponseReceived {
		if action.AppendContinuationItemsAction != nil {
			for _, item := range action.AppendContinuationItemsAction.ContinuationItems {
				if token := extractTokenFromContinuationItem(&item); token != "" {
					return token
				}
			}
		}
	}

	// Check initial page response structure
	if resp.Contents != nil && resp.Contents.TwoColumnBrowseResultsRenderer != nil {
		for _, tab := range resp.Contents.TwoColumnBrowseResultsRenderer.Tabs {
			if tab.TabRenderer != nil && tab.TabRenderer.Content != nil {
				// Check RichGridRenderer
				if tab.TabRenderer.Content.RichGridRenderer != nil {
					if token := extractTokenFromRichGrid(tab.TabRenderer.Content.RichGridRenderer); token != "" {
						return token
					}
				}
				// Check SectionListRenderer
				if tab.TabRenderer.Content.SectionListRenderer != nil {
					if token := extractTokenFromSectionList(tab.TabRenderer.Content.SectionListRenderer); token != "" {
						return token
					}
				}
			}
		}
	}

	return ""
}

// extractTokenFromRichGrid gets continuation token from a RichGridRenderer.
func extractTokenFromRichGrid(grid *RichGridRenderer) string {
	// Check inline continuation items
	for _, content := range grid.Contents {
		if content.ContinuationItemRenderer != nil {
			if token := extractTokenFromContinuationRenderer(content.ContinuationItemRenderer); token != "" {
				return token
			}
		}
	}

	// Check continuations array
	for _, cont := range grid.Continuations {
		if cont.NextContinuationData != nil && cont.NextContinuationData.Continuation != "" {
			return cont.NextContinuationData.Continuation
		}
	}

	return ""
}

// extractTokenFromSectionList gets continuation token from a SectionListRenderer.
func extractTokenFromSectionList(list *SectionListRenderer) string {
	for _, cont := range list.Continuations {
		if cont.NextContinuationData != nil && cont.NextContinuationData.Continuation != "" {
			return cont.NextContinuationData.Continuation
		}
	}
	return ""
}

// extractTokenFromContinuationItem extracts token from a ContinuationItem.
func extractTokenFromContinuationItem(item *ContinuationItem) string {
	if item.ContinuationItemRenderer != nil {
		return extractTokenFromContinuationRenderer(item.ContinuationItemRenderer)
	}
	return ""
}

// extractTokenFromContinuationRenderer extracts token from a ContinuationItemRenderer.
func extractTokenFromContinuationRenderer(renderer *ContinuationItemRenderer) string {
	if renderer.ContinuationEndpoint != nil &&
		renderer.ContinuationEndpoint.ContinuationCommand != nil {
		return renderer.ContinuationEndpoint.ContinuationCommand.Token
	}
	return ""
}

// ExtractVideos extracts VideoInfo data from a browse response.
// Returns videos and the channel info if available.
func ExtractVideos(resp *BrowseResponse, channelID, channelName string) []VideoData {
	if resp == nil {
		return nil
	}

	var videos []VideoData

	// Extract channel info from response if not provided
	if channelName == "" {
		channelName = extractChannelName(resp)
	}
	if channelID == "" {
		channelID = extractChannelID(resp)
	}

	// Extract from continuation response
	for _, action := range resp.OnResponseReceived {
		if action.AppendContinuationItemsAction != nil {
			for _, item := range action.AppendContinuationItemsAction.ContinuationItems {
				if v := extractVideoFromContinuationItem(&item, channelID, channelName); v != nil {
					videos = append(videos, *v)
				}
			}
		}
	}

	// Extract from initial page response
	if resp.Contents != nil && resp.Contents.TwoColumnBrowseResultsRenderer != nil {
		for _, tab := range resp.Contents.TwoColumnBrowseResultsRenderer.Tabs {
			if tab.TabRenderer != nil && tab.TabRenderer.Content != nil {
				if tab.TabRenderer.Content.RichGridRenderer != nil {
					for _, content := range tab.TabRenderer.Content.RichGridRenderer.Contents {
						if v := extractVideoFromRichGridContent(&content, channelID, channelName); v != nil {
							videos = append(videos, *v)
						}
					}
				}
				if tab.TabRenderer.Content.SectionListRenderer != nil {
					for _, section := range tab.TabRenderer.Content.SectionListRenderer.Contents {
						if section.ItemSectionRenderer != nil {
							for _, item := range section.ItemSectionRenderer.Contents {
								if v := extractVideoFromItemContent(&item, channelID, channelName); v != nil {
									videos = append(videos, *v)
								}
							}
						}
					}
				}
			}
		}
	}

	return videos
}

// VideoData represents extracted video information.
type VideoData struct {
	VideoID     string
	Title       string
	Description string
	Thumbnail   string
	Published   string
	Duration    string
	ViewCount   string
	ChannelID   string
	ChannelName string
}

// extractVideoFromContinuationItem extracts video data from a continuation item.
func extractVideoFromContinuationItem(item *ContinuationItem, channelID, channelName string) *VideoData {
	if item.RichItemRenderer != nil && item.RichItemRenderer.Content != nil {
		if item.RichItemRenderer.Content.VideoRenderer != nil {
			return videoRendererToData(item.RichItemRenderer.Content.VideoRenderer, channelID, channelName)
		}
	}
	if item.GridVideoRenderer != nil {
		return gridVideoRendererToData(item.GridVideoRenderer, channelID, channelName)
	}
	return nil
}

// extractVideoFromRichGridContent extracts video data from rich grid content.
func extractVideoFromRichGridContent(content *RichGridContent, channelID, channelName string) *VideoData {
	if content.RichItemRenderer != nil && content.RichItemRenderer.Content != nil {
		if content.RichItemRenderer.Content.VideoRenderer != nil {
			return videoRendererToData(content.RichItemRenderer.Content.VideoRenderer, channelID, channelName)
		}
	}
	return nil
}

// extractVideoFromItemContent extracts video data from item content.
func extractVideoFromItemContent(item *ItemContent, channelID, channelName string) *VideoData {
	if item.VideoRenderer != nil {
		return videoRendererToData(item.VideoRenderer, channelID, channelName)
	}
	if item.GridVideoRenderer != nil {
		return gridVideoRendererToData(item.GridVideoRenderer, channelID, channelName)
	}
	return nil
}

// videoRendererToData converts a VideoRenderer to VideoData.
func videoRendererToData(v *VideoRenderer, channelID, channelName string) *VideoData {
	if v == nil || v.VideoID == "" {
		return nil
	}

	data := &VideoData{
		VideoID:     v.VideoID,
		Title:       v.Title.GetText(),
		ChannelID:   channelID,
		ChannelName: channelName,
	}

	if v.DescriptionSnippet != nil {
		data.Description = v.DescriptionSnippet.GetText()
	}
	if v.Thumbnail != nil && len(v.Thumbnail.Thumbnails) > 0 {
		data.Thumbnail = v.Thumbnail.Thumbnails[0].URL
	}
	if v.PublishedTimeText != nil {
		data.Published = v.PublishedTimeText.SimpleText
	}
	if v.LengthText != nil {
		data.Duration = v.LengthText.SimpleText
	}
	if v.ViewCountText != nil {
		data.ViewCount = v.ViewCountText.SimpleText
	}

	return data
}

// gridVideoRendererToData converts a GridVideoRenderer to VideoData.
func gridVideoRendererToData(v *GridVideoRenderer, channelID, channelName string) *VideoData {
	if v == nil || v.VideoID == "" {
		return nil
	}

	data := &VideoData{
		VideoID:     v.VideoID,
		Title:       v.Title.GetText(),
		ChannelID:   channelID,
		ChannelName: channelName,
	}

	if v.Thumbnail != nil && len(v.Thumbnail.Thumbnails) > 0 {
		data.Thumbnail = v.Thumbnail.Thumbnails[0].URL
	}
	if v.PublishedTimeText != nil {
		data.Published = v.PublishedTimeText.SimpleText
	}
	if v.ViewCountText != nil {
		data.ViewCount = v.ViewCountText.SimpleText
	}

	return data
}

// extractChannelName gets the channel name from the response.
func extractChannelName(resp *BrowseResponse) string {
	if resp.Metadata != nil && resp.Metadata.ChannelMetadataRenderer != nil {
		return resp.Metadata.ChannelMetadataRenderer.Title
	}
	if resp.Header != nil {
		if resp.Header.C4TabbedHeaderRenderer != nil {
			return resp.Header.C4TabbedHeaderRenderer.Title
		}
		if resp.Header.PageHeaderRenderer != nil &&
			resp.Header.PageHeaderRenderer.Content != nil &&
			resp.Header.PageHeaderRenderer.Content.PageHeaderViewModel != nil &&
			resp.Header.PageHeaderRenderer.Content.PageHeaderViewModel.Title != nil &&
			resp.Header.PageHeaderRenderer.Content.PageHeaderViewModel.Title.DynamicTextViewModel != nil {
			return resp.Header.PageHeaderRenderer.Content.PageHeaderViewModel.Title.DynamicTextViewModel.Text.GetText()
		}
	}
	return ""
}

// extractChannelID gets the channel ID from the response.
func extractChannelID(resp *BrowseResponse) string {
	if resp.Metadata != nil && resp.Metadata.ChannelMetadataRenderer != nil {
		return resp.Metadata.ChannelMetadataRenderer.ExternalID
	}
	if resp.Header != nil && resp.Header.C4TabbedHeaderRenderer != nil {
		return resp.Header.C4TabbedHeaderRenderer.ChannelID
	}
	return ""
}

// IsValidContinuationToken performs basic validation on a continuation token.
// Tokens are base64-encoded protobuf messages.
func IsValidContinuationToken(token string) bool {
	if token == "" {
		return false
	}

	// Try to decode as base64 (tokens are URL-safe base64)
	_, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		// Try standard base64
		_, err = base64.StdEncoding.DecodeString(token)
	}

	return err == nil
}
