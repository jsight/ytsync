package innertube

import (
	"testing"
	"time"
)

func TestContinuationState_NewAndReset(t *testing.T) {
	channelID := "UCtest123"
	state := NewContinuationState(channelID)

	if state.ChannelID != channelID {
		t.Errorf("expected ChannelID %q, got %q", channelID, state.ChannelID)
	}
	if state.Token != "" {
		t.Error("expected empty token on new state")
	}
	if state.VideosRetrieved != 0 {
		t.Errorf("expected 0 videos, got %d", state.VideosRetrieved)
	}
	if state.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Update state
	state.UpdateToken("token123", "video456")
	state.IncrementVideos(10)

	if state.Token != "token123" {
		t.Errorf("expected token %q, got %q", "token123", state.Token)
	}
	if state.LastVideoID != "video456" {
		t.Errorf("expected last video %q, got %q", "video456", state.LastVideoID)
	}
	if state.VideosRetrieved != 10 {
		t.Errorf("expected 10 videos, got %d", state.VideosRetrieved)
	}

	// Reset
	state.Reset()
	if state.Token != "" {
		t.Error("expected empty token after reset")
	}
	if state.VideosRetrieved != 0 {
		t.Errorf("expected 0 videos after reset, got %d", state.VideosRetrieved)
	}
}

func TestContinuationState_HasMore(t *testing.T) {
	state := NewContinuationState("UCtest")

	if state.HasMore() {
		t.Error("expected HasMore() to be false with empty token")
	}

	state.UpdateToken("sometoken", "")
	if !state.HasMore() {
		t.Error("expected HasMore() to be true with token set")
	}

	state.UpdateToken("", "")
	if state.HasMore() {
		t.Error("expected HasMore() to be false with empty token")
	}
}

func TestContinuationState_IsExpired(t *testing.T) {
	state := NewContinuationState("UCtest")

	// No token, not expired
	if state.IsExpired() {
		t.Error("expected IsExpired() to be false with no token")
	}

	// Set token, not expired yet
	state.UpdateToken("token", "")
	if state.IsExpired() {
		t.Error("expected IsExpired() to be false with fresh token")
	}

	// Manually set expired time
	state.ExpiresAt = time.Now().Add(-1 * time.Hour)
	if !state.IsExpired() {
		t.Error("expected IsExpired() to be true with past expiry")
	}
}

func TestContinuationState_JSONSerialization(t *testing.T) {
	state := NewContinuationState("UCtest123")
	state.UpdateToken("mytoken", "lastvideo")
	state.IncrementVideos(50)

	// Serialize
	jsonStr, err := state.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Deserialize
	restored, err := ContinuationStateFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("ContinuationStateFromJSON failed: %v", err)
	}

	if restored.ChannelID != state.ChannelID {
		t.Errorf("ChannelID mismatch: got %q, want %q", restored.ChannelID, state.ChannelID)
	}
	if restored.Token != state.Token {
		t.Errorf("Token mismatch: got %q, want %q", restored.Token, state.Token)
	}
	if restored.LastVideoID != state.LastVideoID {
		t.Errorf("LastVideoID mismatch: got %q, want %q", restored.LastVideoID, state.LastVideoID)
	}
	if restored.VideosRetrieved != state.VideosRetrieved {
		t.Errorf("VideosRetrieved mismatch: got %d, want %d", restored.VideosRetrieved, state.VideosRetrieved)
	}
}

func TestExtractContinuationToken_Empty(t *testing.T) {
	token := ExtractContinuationToken(nil)
	if token != "" {
		t.Errorf("expected empty token from nil response, got %q", token)
	}

	token = ExtractContinuationToken(&BrowseResponse{})
	if token != "" {
		t.Errorf("expected empty token from empty response, got %q", token)
	}
}

func TestExtractContinuationToken_FromOnResponseReceived(t *testing.T) {
	resp := &BrowseResponse{
		OnResponseReceived: []OnResponseAction{
			{
				AppendContinuationItemsAction: &AppendContinuationItemsAction{
					ContinuationItems: []ContinuationItem{
						{
							RichItemRenderer: &RichItemRenderer{
								Content: &RichItemContent{
									VideoRenderer: &VideoRenderer{VideoID: "video1"},
								},
							},
						},
						{
							ContinuationItemRenderer: &ContinuationItemRenderer{
								ContinuationEndpoint: &ContinuationEndpoint{
									ContinuationCommand: &ContinuationCommand{
										Token: "next_page_token",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	token := ExtractContinuationToken(resp)
	if token != "next_page_token" {
		t.Errorf("expected token %q, got %q", "next_page_token", token)
	}
}

func TestExtractContinuationToken_FromRichGrid(t *testing.T) {
	resp := &BrowseResponse{
		Contents: &Contents{
			TwoColumnBrowseResultsRenderer: &TwoColumnBrowseResultsRenderer{
				Tabs: []Tab{
					{
						TabRenderer: &TabRenderer{
							Content: &TabContent{
								RichGridRenderer: &RichGridRenderer{
									Contents: []RichGridContent{
										{
											RichItemRenderer: &RichItemRenderer{
												Content: &RichItemContent{
													VideoRenderer: &VideoRenderer{VideoID: "video1"},
												},
											},
										},
										{
											ContinuationItemRenderer: &ContinuationItemRenderer{
												ContinuationEndpoint: &ContinuationEndpoint{
													ContinuationCommand: &ContinuationCommand{
														Token: "grid_token",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	token := ExtractContinuationToken(resp)
	if token != "grid_token" {
		t.Errorf("expected token %q, got %q", "grid_token", token)
	}
}

func TestExtractVideos(t *testing.T) {
	resp := &BrowseResponse{
		Metadata: &ChannelMetadata{
			ChannelMetadataRenderer: &ChannelMetadataRenderer{
				Title:      "Test Channel",
				ExternalID: "UCtest123",
			},
		},
		Contents: &Contents{
			TwoColumnBrowseResultsRenderer: &TwoColumnBrowseResultsRenderer{
				Tabs: []Tab{
					{
						TabRenderer: &TabRenderer{
							Content: &TabContent{
								RichGridRenderer: &RichGridRenderer{
									Contents: []RichGridContent{
										{
											RichItemRenderer: &RichItemRenderer{
												Content: &RichItemContent{
													VideoRenderer: &VideoRenderer{
														VideoID: "video1",
														Title: &TextRuns{
															Runs: []TextRun{{Text: "First Video"}},
														},
														PublishedTimeText: &SimpleText{SimpleText: "2 days ago"},
													},
												},
											},
										},
										{
											RichItemRenderer: &RichItemRenderer{
												Content: &RichItemContent{
													VideoRenderer: &VideoRenderer{
														VideoID: "video2",
														Title: &TextRuns{
															SimpleText: "Second Video",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	videos := ExtractVideos(resp, "", "")

	if len(videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(videos))
	}

	if videos[0].VideoID != "video1" {
		t.Errorf("expected video1, got %s", videos[0].VideoID)
	}
	if videos[0].Title != "First Video" {
		t.Errorf("expected 'First Video', got %s", videos[0].Title)
	}
	if videos[0].ChannelName != "Test Channel" {
		t.Errorf("expected 'Test Channel', got %s", videos[0].ChannelName)
	}

	if videos[1].VideoID != "video2" {
		t.Errorf("expected video2, got %s", videos[1].VideoID)
	}
	if videos[1].Title != "Second Video" {
		t.Errorf("expected 'Second Video', got %s", videos[1].Title)
	}
}

func TestIsValidContinuationToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{"empty", "", false},
		{"valid base64", "SGVsbG8gV29ybGQ", true},
		{"valid url-safe base64", "SGVsbG8tV29ybGRf", true},
		{"valid padded base64", "SGVsbG8gV29ybGQ=", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidContinuationToken(tt.token)
			if got != tt.valid {
				t.Errorf("IsValidContinuationToken(%q) = %v, want %v", tt.token, got, tt.valid)
			}
		})
	}
}

func TestGetText(t *testing.T) {
	tests := []struct {
		name     string
		textRuns *TextRuns
		expected string
	}{
		{"nil", nil, ""},
		{"simple text", &TextRuns{SimpleText: "Hello"}, "Hello"},
		{"runs", &TextRuns{Runs: []TextRun{{Text: "Hello "}, {Text: "World"}}}, "Hello World"},
		{"simple takes precedence", &TextRuns{SimpleText: "Simple", Runs: []TextRun{{Text: "Run"}}}, "Simple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.textRuns.GetText()
			if got != tt.expected {
				t.Errorf("GetText() = %q, want %q", got, tt.expected)
			}
		})
	}
}
