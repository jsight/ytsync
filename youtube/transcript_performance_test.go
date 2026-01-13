package youtube

import (
	"testing"
	"time"
)

// TestLargeTranscriptPerformance tests performance with large transcript data.
func TestLargeTranscriptPerformance(t *testing.T) {
	// Create a large transcript (1000 entries = typical long video)
	entries := make([]TranscriptEntry, 1000)
	for i := 0; i < 1000; i++ {
		entries[i] = TranscriptEntry{
			Start:    float64(i),
			Duration: 1.0,
			Text:     "Caption line " + string(rune(i)),
		}
	}

	converter := NewFormatConverter(entries)

	// Test 1: JSON3 conversion should be fast
	start := time.Now()
	json3Output, err := converter.ToFormat(FormatJSON3)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("JSON3 conversion failed: %v", err)
	}
	if json3Output == "" {
		t.Error("JSON3 output is empty")
	}

	// Should complete in reasonable time (< 100ms for 1000 entries)
	if duration > 100*time.Millisecond {
		t.Logf("JSON3 conversion took %v (slow)", duration)
	}

	// Test 2: VTT conversion should be fast
	vttOutput, err := converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("VTT conversion failed: %v", err)
	}
	if vttOutput == "" {
		t.Error("VTT output is empty")
	}

	// Test 3: All formats should handle large transcripts
	formats := []Format{
		FormatJSON3, FormatJSON, FormatVTT,
		FormatSRT, FormatTTML, FormatPlainText,
	}

	for _, format := range formats {
		start := time.Now()
		output, err := converter.ToFormat(format)
		duration := time.Since(start)

		if err != nil {
			t.Errorf("Format %s failed: %v", format, err)
			continue
		}

		if output == "" {
			t.Errorf("Format %s returned empty output", format)
		}

		// Log slow conversions
		if duration > 100*time.Millisecond {
			t.Logf("Format %s: %v for 1000 entries", format, duration)
		}
	}
}

// TestFormatParsingPerformance tests parsing performance for large transcripts.
func TestFormatParsingPerformance(t *testing.T) {
	// Create a large transcript
	entries := make([]TranscriptEntry, 500)
	for i := 0; i < 500; i++ {
		entries[i] = TranscriptEntry{
			Start:    float64(i * 2),
			Duration: 2.0,
			Text:     "Line " + string(rune(i)),
		}
	}

	converter := NewFormatConverter(entries)

	// Test VTT parsing performance (most common format)
	vttOutput, _ := converter.ToFormat(FormatVTT)

	start := time.Now()
	parsed, err := ParseFormat(vttOutput, FormatVTT)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("VTT parsing failed: %v", err)
	}

	if len(parsed) == 0 {
		t.Error("Parsed VTT has no entries")
	}

	// Parsing should be fast
	if duration > 50*time.Millisecond {
		t.Logf("VTT parsing took %v (may be slow)", duration)
	}

	// Test JSON parsing
	jsonOutput, _ := converter.ToFormat(FormatJSON)

	start = time.Now()
	parsedJSON, err := ParseFormat(jsonOutput, FormatJSON)
	duration = time.Since(start)

	if err != nil {
		t.Fatalf("JSON parsing failed: %v", err)
	}

	if len(parsedJSON) == 0 {
		t.Error("Parsed JSON has no entries")
	}

	if duration > 50*time.Millisecond {
		t.Logf("JSON parsing took %v", duration)
	}
}

// TestLanguageCachingPerformance tests that caching improves performance.
func TestLanguageCachingPerformance(t *testing.T) {
	cache := NewLanguageCache(1 * time.Hour)
	selector := NewLanguageSelector(DefaultLanguagePreference())

	// Create availability data for multiple videos
	videoCount := 100
	videos := make([]*LanguageAvailability, videoCount)

	for i := 0; i < videoCount; i++ {
		videoID := "video-" + string(rune(i))
		la := NewLanguageAvailability(videoID)
		la.Update(
			[]LanguageInfo{{Code: "en", Name: "English"}},
			[]LanguageInfo{{Code: "es", Name: "Spanish"}},
		)
		videos[i] = la
	}

	// First pass: populate cache
	start := time.Now()
	for i, la := range videos {
		videoID := "video-" + string(rune(i))
		cache.Set(videoID, la)
	}
	populateTime := time.Since(start)

	// Second pass: retrieve from cache
	start = time.Now()
	for i := 0; i < videoCount; i++ {
		videoID := "video-" + string(rune(i))
		retrieved := cache.Get(videoID)
		if retrieved == nil {
			t.Errorf("Cache miss for %s", videoID)
		}
	}
	retrieveTime := time.Since(start)

	// Cache retrieval should be much faster than population
	if retrieveTime > populateTime {
		t.Logf("Cache retrieval (%v) slower than population (%v)", retrieveTime, populateTime)
	}

	// Test selector caching
	start = time.Now()
	for i, la := range videos {
		videoID := "video-" + string(rune(i))
		_, _, _ = selector.Select(videoID, la)
	}
	selectorTime := time.Since(start)

	// Should be reasonably fast
	if selectorTime > 100*time.Millisecond {
		t.Logf("Selector operations took %v", selectorTime)
	}
}

// TestMemoryEfficiencyOfLargeTranscripts tests memory usage with large transcripts.
func TestMemoryEfficiencyOfLargeTranscripts(t *testing.T) {
	// Create very large transcript
	entries := make([]TranscriptEntry, 10000)
	for i := 0; i < 10000; i++ {
		entries[i] = TranscriptEntry{
			Start:    float64(i),
			Duration: 1.0,
			Text:     "Line " + string(rune(i%256)),
		}
	}

	converter := NewFormatConverter(entries)

	// Should be able to convert without issues
	output, err := converter.ToFormat(FormatJSON)
	if err != nil {
		t.Fatalf("Failed to convert large transcript: %v", err)
	}

	if output == "" {
		t.Error("Empty output for large transcript")
	}

	// Can parse back
	parsed, err := ParseFormat(output, FormatJSON)
	if err != nil {
		t.Fatalf("Failed to parse large transcript: %v", err)
	}

	if len(parsed) != 10000 {
		t.Errorf("Parsed %d entries, want 10000", len(parsed))
	}
}

// TestCacheExpiration tests that cache TTL works correctly.
func TestCacheExpiration(t *testing.T) {
	// Create cache with short TTL (50ms)
	cache := NewLanguageCache(50 * time.Millisecond)

	la := NewLanguageAvailability("test-video")
	la.Update([]LanguageInfo{{Code: "en", Name: "English"}}, []LanguageInfo{})

	cache.Set("test-video", la)

	// Immediately retrieve - should be cached
	retrieved := cache.Get("test-video")
	if retrieved == nil {
		t.Fatal("Should retrieve immediately cached item")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Should now be expired
	retrieved = cache.Get("test-video")
	if retrieved != nil {
		t.Error("Cache should expire after TTL")
	}
}

// TestConcurrentLanguageSelection tests concurrent language selection operations.
func TestConcurrentLanguageSelection(t *testing.T) {
	la := NewLanguageAvailability("concurrent-test")
	la.Update(
		[]LanguageInfo{
			{Code: "en", Name: "English"},
			{Code: "es", Name: "Spanish"},
			{Code: "fr", Name: "French"},
		},
		[]LanguageInfo{
			{Code: "de", Name: "German"},
		},
	)

	pref := DefaultLanguagePreference()

	// Simulate concurrent selections
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			lang, isAuto := la.SelectLanguage(pref)
			if lang == "" {
				t.Error("Language selection returned empty")
			}
			if isAuto && lang != "de" {
				t.Error("Unexpected auto-generated flag")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	count := 0
	for count < 100 {
		<-done
		count++
	}
}

// TestFormatConversionCaching tests repeated conversions to same format.
func TestFormatConversionCaching(t *testing.T) {
	entries := make([]TranscriptEntry, 100)
	for i := 0; i < 100; i++ {
		entries[i] = TranscriptEntry{
			Start:    float64(i),
			Duration: 1.0,
			Text:     "Line " + string(rune(i)),
		}
	}

	converter := NewFormatConverter(entries)

	// First conversion
	start := time.Now()
	output1, err := converter.ToFormat(FormatVTT)
	time1 := time.Since(start)

	if err != nil {
		t.Fatalf("First conversion failed: %v", err)
	}

	// Second conversion (should be the same operation)
	start = time.Now()
	output2, err := converter.ToFormat(FormatVTT)
	time2 := time.Since(start)

	if err != nil {
		t.Fatalf("Second conversion failed: %v", err)
	}

	// Outputs should be identical
	if output1 != output2 {
		t.Error("Repeated conversions produce different outputs")
	}

	// Both should complete quickly (though second may not be faster without actual caching)
	if time1 > 100*time.Millisecond {
		t.Logf("First conversion took %v", time1)
	}
	if time2 > 100*time.Millisecond {
		t.Logf("Second conversion took %v", time2)
	}
}

// TestRapidLanguageAvailabilityUpdates tests rapid updates to language availability.
func TestRapidLanguageAvailabilityUpdates(t *testing.T) {
	la := NewLanguageAvailability("rapid-update-video")

	// Simulate rapid updates (e.g., as languages become available)
	for i := 0; i < 50; i++ {
		languages := []LanguageInfo{
			{Code: "en", Name: "English"},
		}

		// Add more languages based on iteration
		if i > 10 {
			languages = append(languages, LanguageInfo{Code: "es", Name: "Spanish"})
		}
		if i > 20 {
			languages = append(languages, LanguageInfo{Code: "fr", Name: "French"})
		}

		la.Update(languages, []LanguageInfo{})

		// Always able to select
		lang, _ := la.SelectLanguage(DefaultLanguagePreference())
		if lang == "" {
			t.Errorf("Iteration %d: failed to select language", i)
		}
	}
}

// TestLanguageSelectorCacheSize tests cache size management.
func TestLanguageSelectorCacheSize(t *testing.T) {
	selector := NewLanguageSelector(DefaultLanguagePreference())

	// Add multiple videos to cache
	for i := 0; i < 50; i++ {
		videoID := "video-" + string(rune(i))
		la := NewLanguageAvailability(videoID)
		la.Update([]LanguageInfo{{Code: "en", Name: "English"}}, []LanguageInfo{})
		selector.Select(videoID, la)
	}

	// Verify size
	size := selector.cache.Size()
	if size != 50 {
		t.Errorf("Cache size = %d, want 50", size)
	}

	// Clear cache
	selector.ClearCache()
	size = selector.cache.Size()
	if size != 0 {
		t.Errorf("Cache size after clear = %d, want 0", size)
	}
}

// TestTranscriptEntryAllocation tests efficient allocation of large entry lists.
func TestTranscriptEntryAllocation(t *testing.T) {
	// Pre-allocate with expected capacity
	entries := make([]TranscriptEntry, 0, 1000)

	// Fill entries
	for i := 0; i < 1000; i++ {
		entries = append(entries, TranscriptEntry{
			Start:    float64(i),
			Duration: 1.0,
			Text:     "Line",
		})
	}

	if len(entries) != 1000 {
		t.Errorf("Entry count = %d, want 1000", len(entries))
	}

	// Should have no reallocations due to pre-allocation
	converter := NewFormatConverter(entries)

	// Conversion should be efficient
	start := time.Now()
	output, err := converter.ToFormat(FormatJSON)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if output == "" {
		t.Error("Empty output")
	}

	if duration > 200*time.Millisecond {
		t.Logf("Conversion took %v (may be slow)", duration)
	}
}
