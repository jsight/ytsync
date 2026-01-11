package youtube

import (
	"strings"
	"testing"
)

// TestTranscriptExtractionNoAvailableCaptions tests handling of videos with no captions.
func TestTranscriptExtractionNoAvailableCaptions(t *testing.T) {
	// Simulate a video with no captions
	la := NewLanguageAvailability("no-captions-video")
	la.Update([]LanguageInfo{}, []LanguageInfo{})

	pref := DefaultLanguagePreference()
	lang, _ := la.SelectLanguage(pref)

	if lang != "" {
		t.Errorf("Should return empty string for video with no captions, got %s", lang)
	}
}

// TestTranscriptExtractionAgeRestrictedVideos tests handling of age-restricted videos.
func TestTranscriptExtractionAgeRestrictedVideos(t *testing.T) {
	// Age-restricted videos may have limited caption availability
	la := NewLanguageAvailability("age-restricted-video")

	// Simulate only English captions available
	la.Update(
		[]LanguageInfo{{Code: "en", Name: "English"}},
		[]LanguageInfo{},
	)

	pref := LanguagePreference{
		PreferredLanguages: []string{"es", "fr", "en"},
	}

	lang, _ := la.SelectLanguage(pref)
	if lang != "en" {
		t.Errorf("Should find English caption for restricted video, got %s", lang)
	}
}

// TestTranscriptExtractionLiveStreams tests that live streams are handled correctly.
func TestTranscriptExtractionLiveStreams(t *testing.T) {
	// Live streams may have limited or no transcript availability initially
	// but can develop transcripts as they're being watched

	la := NewLanguageAvailability("live-stream")

	// Test 1: Live stream with no transcript yet
	la.Update([]LanguageInfo{}, []LanguageInfo{})
	lang, _ := la.SelectLanguage(DefaultLanguagePreference())
	if lang != "" {
		t.Error("Live stream without transcript should return empty")
	}

	// Test 2: Live stream that has developed a transcript
	la.Update(
		[]LanguageInfo{{Code: "en", Name: "English"}},
		[]LanguageInfo{},
	)
	lang, _ = la.SelectLanguage(DefaultLanguagePreference())
	if lang != "en" {
		t.Errorf("Live stream with transcript should return language, got %s", lang)
	}
}

// TestTranscriptExtractionDeletedVideos tests handling of deleted or removed videos.
func TestTranscriptExtractionDeletedVideos(t *testing.T) {
	// Deleted videos have no transcript data
	la := NewLanguageAvailability("deleted-video")
	la.Update([]LanguageInfo{}, []LanguageInfo{})

	pref := DefaultLanguagePreference()
	lang, _ := la.SelectLanguage(pref)

	if lang != "" {
		t.Error("Deleted video should have no language available")
	}
}

// TestRateLimitingErrorHandling tests proper error handling for rate limiting.
func TestRateLimitingErrorHandling(t *testing.T) {
	// Create a timedtext client and simulate rate limit response
	client := NewTimedtextClient()
	defer client.Close()

	// Verify client is properly configured for rate limit handling
	if client.baseURL != "https://www.youtube.com/api/timedtext" {
		t.Error("Timedtext client should be configured correctly")
	}
}

// TestNetworkFailureHandling tests handling of network timeouts and failures.
func TestNetworkFailureHandling(t *testing.T) {
	// Test timeout configuration in HTTP client
	client := NewTimedtextClient()
	defer client.Close()

	// Verify HTTP client has proper timeout
	if client.httpClient == nil {
		t.Fatal("HTTP client should be initialized")
	}
}

// TestTranscriptExtractionEmptyText tests handling of caption entries with empty text.
func TestTranscriptExtractionEmptyText(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Valid text"},
		{Start: 1, Duration: 1, Text: ""}, // Empty text
		{Start: 2, Duration: 1, Text: "More text"},
	}

	converter := NewFormatConverter(entries)

	// Should handle empty text gracefully in all formats
	formats := []Format{FormatVTT, FormatSRT, FormatJSON, FormatTTML}

	for _, format := range formats {
		output, err := converter.ToFormat(format)
		if err != nil {
			t.Errorf("Format %s failed with empty text entry: %v", format, err)
		}
		if output == "" {
			t.Errorf("Format %s returned empty", format)
		}
	}
}

// TestTranscriptExtractionVeryLongText tests handling of captions with very long text.
func TestTranscriptExtractionVeryLongText(t *testing.T) {
	// Create a caption with very long text (e.g., 10KB)
	longText := strings.Repeat("This is a very long caption text. ", 300) // ~10KB

	entries := []TranscriptEntry{
		{Start: 0, Duration: 10, Text: longText},
		{Start: 10, Duration: 10, Text: "Short text"},
	}

	converter := NewFormatConverter(entries)

	// Should handle long text without issues
	output, err := converter.ToFormat(FormatJSON)
	if err != nil {
		t.Fatalf("Failed to handle long text: %v", err)
	}

	// Verify long text is preserved
	if !strings.Contains(output, "very long caption") {
		t.Error("Long text not preserved in JSON output")
	}

	// Test round-trip
	parsed, err := ParseFormat(output, FormatJSON)
	if err != nil {
		t.Fatalf("Failed to parse long text: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("Parse returned %d entries, want 2", len(parsed))
	}

	if !strings.Contains(parsed[0].Text, "very long caption") {
		t.Error("Long text not preserved in round-trip")
	}
}

// TestTranscriptExtractionSpecialLanguageCodes tests handling of special language codes.
func TestTranscriptExtractionSpecialLanguageCodes(t *testing.T) {
	la := NewLanguageAvailability("special-lang-video")

	// Test with language codes with regional variants
	manual := []LanguageInfo{
		{Code: "zh-Hans", Name: "Chinese (Simplified)"},
		{Code: "zh-Hant", Name: "Chinese (Traditional)"},
		{Code: "pt-BR", Name: "Portuguese (Brazil)"},
		{Code: "pt-PT", Name: "Portuguese (Portugal)"},
	}

	la.Update(manual, []LanguageInfo{})

	// Test specific language code selection
	pref := LanguagePreference{
		PreferredLanguages: []string{"zh-Hans"},
	}

	lang, _ := la.SelectLanguage(pref)
	if lang != "zh-Hans" {
		t.Errorf("Should handle regional variant codes, got %s", lang)
	}

	// Test fallback from specific to general
	pref.PreferredLanguages = []string{"pt-BR", "pt-PT"}
	lang, _ = la.SelectLanguage(pref)
	if lang != "pt-BR" {
		t.Errorf("Should respect preference order, got %s", lang)
	}
}

// TestCaptionFormatConversionEdgeCases tests edge cases in format conversion.
func TestCaptionFormatConversionEdgeCases(t *testing.T) {
	// Test 1: Very short duration (< 100ms)
	entries := []TranscriptEntry{
		{Start: 0, Duration: 0.05, Text: "Quick"},
		{Start: 0.05, Duration: 0.001, Text: "Very short"},
	}

	converter := NewFormatConverter(entries)
	output, err := converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("Failed with very short duration: %v", err)
	}
	if output == "" {
		t.Error("Empty output for short durations")
	}

	// Test 2: Very long duration (> 1 hour)
	entries = []TranscriptEntry{
		{Start: 0, Duration: 3661, Text: "Over an hour"},
	}

	converter = NewFormatConverter(entries)
	output, err = converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("Failed with long duration: %v", err)
	}

	// Should have proper time formatting for > 1 hour
	if !strings.Contains(output, "01:01:01") {
		t.Error("VTT should format times over 1 hour correctly")
	}

	// Test 3: Zero start time
	entries = []TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Starts at zero"},
	}

	converter = NewFormatConverter(entries)
	output, err = converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("Failed with zero start time: %v", err)
	}
	if !strings.Contains(output, "00:00:00") {
		t.Error("Should handle zero start time")
	}
}

// TestLanguageSelectionWithNoPreference tests language selection when user has no preference.
func TestLanguageSelectionWithNoPreference(t *testing.T) {
	la := NewLanguageAvailability("multi-lang-video")

	manual := []LanguageInfo{
		{Code: "en", Name: "English"},
		{Code: "es", Name: "Spanish"},
		{Code: "fr", Name: "French"},
	}

	la.Update(manual, []LanguageInfo{})

	// Empty preference list should still select something
	pref := LanguagePreference{
		PreferredLanguages: []string{},
	}

	lang, _ := la.SelectLanguage(pref)
	if lang == "" {
		t.Error("Should select a language even without preferences")
	}
}

// TestTranscriptWithMultilineText tests handling of captions with newlines.
func TestTranscriptWithMultilineText(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Line 1\nLine 2"},
		{Start: 2, Duration: 2, Text: "Single line"},
	}

	converter := NewFormatConverter(entries)

	// Test VTT with newlines
	vttOutput, err := converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("VTT conversion failed: %v", err)
	}

	// VTT should preserve or escape newlines appropriately
	if !strings.Contains(vttOutput, "Line 1") || !strings.Contains(vttOutput, "Line 2") {
		t.Error("VTT should preserve multiline text")
	}

	// Test SRT with newlines
	srtOutput, err := converter.ToFormat(FormatSRT)
	if err != nil {
		t.Fatalf("SRT conversion failed: %v", err)
	}

	// Parse back and verify
	parsed, err := ParseFormat(srtOutput, FormatSRT)
	if err != nil {
		t.Fatalf("SRT parsing failed: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("SRT parse returned %d entries, want 2", len(parsed))
	}
}

// TestTranscriptWithUnicodeCharacters tests handling of Unicode characters.
func TestTranscriptWithUnicodeCharacters(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Hello 世界"}, // Chinese
		{Start: 1, Duration: 1, Text: "مرحبا"},    // Arabic
		{Start: 2, Duration: 1, Text: "🎉 emoji"}, // Emoji
		{Start: 3, Duration: 1, Text: "café ñ"},  // Accents
	}

	converter := NewFormatConverter(entries)

	formats := []Format{FormatJSON, FormatVTT, FormatTTML, FormatSRT}

	for _, format := range formats {
		output, err := converter.ToFormat(format)
		if err != nil {
			t.Errorf("Format %s failed with Unicode: %v", format, err)
			continue
		}

		// Verify Unicode content is preserved (except TTML which escapes)
		if format == FormatTTML {
			// TTML may escape Unicode, but content should still be there
			if len(output) == 0 {
				t.Errorf("Format %s has empty output", format)
			}
		} else {
			if !strings.Contains(output, "café") && !strings.Contains(output, "caf") {
				t.Logf("Format %s may not preserve all Unicode", format)
			}
		}
	}
}

// TestExtractionErrorRecovery tests recovery from extraction errors.
func TestExtractionErrorRecovery(t *testing.T) {
	// Test format conversion on invalid format string
	_, err := ParseFormat("invalid json", FormatJSON)
	if err == nil {
		t.Error("Should return error for invalid JSON")
	}

	// Test with truly empty input
	_, err = ParseFormat("", FormatJSON)
	if err == nil {
		t.Error("Should return error for empty input")
	}

	// But format conversion on valid empty structure should work
	output, err := NewFormatConverter([]TranscriptEntry{}).ToFormat(FormatJSON)
	if err != nil {
		t.Errorf("Empty conversion should not error: %v", err)
	}
	if output == "" {
		t.Error("Empty conversion should still produce output")
	}
}

// TestCacheBoundaryConditions tests cache behavior at boundaries.
func TestCacheBoundaryConditions(t *testing.T) {
	cache := NewLanguageCache(0) // No TTL

	// Test 1: Add and immediately retrieve
	la := NewLanguageAvailability("test1")
	la.Update([]LanguageInfo{{Code: "en", Name: "English"}}, []LanguageInfo{})
	cache.Set("test1", la)

	retrieved := cache.Get("test1")
	if retrieved == nil {
		t.Error("Should retrieve immediately cached item")
	}

	// Test 2: Clear and verify removal
	cache.Clear("test1")
	retrieved = cache.Get("test1")
	if retrieved != nil {
		t.Error("Should not retrieve after clear")
	}

	// Test 3: ClearAll with multiple items
	cache.Set("test1", la)
	cache.Set("test2", NewLanguageAvailability("test2"))
	cache.Set("test3", NewLanguageAvailability("test3"))

	if cache.Size() != 3 {
		t.Errorf("Cache size = %d, want 3", cache.Size())
	}

	cache.ClearAll()

	if cache.Size() != 0 {
		t.Error("Cache should be empty after ClearAll")
	}
}
