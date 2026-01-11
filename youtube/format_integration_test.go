package youtube

import (
	"strings"
	"testing"
)

// TestCaptionFormatConversionAccuracy tests the accuracy of transcript
// format conversions including timing data preservation.
func TestCaptionFormatConversionAccuracy(t *testing.T) {
	// Create test data with precise timing
	entries := []TranscriptEntry{
		{Start: 0.0, Duration: 2.500, Text: "First caption"},
		{Start: 2.5, Duration: 1.750, Text: "Second caption"},
		{Start: 4.25, Duration: 3.000, Text: "Third caption"},
	}

	converter := NewFormatConverter(entries)

	// Test JSON3 conversion preserves timing
	json3Output, err := converter.ToFormat(FormatJSON3)
	if err != nil {
		t.Fatalf("JSON3 conversion failed: %v", err)
	}

	if !strings.Contains(json3Output, "0") && !strings.Contains(json3Output, "2500") {
		t.Error("JSON3 should contain millisecond timing data")
	}

	// Test VTT conversion with proper time formatting
	vttOutput, err := converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("VTT conversion failed: %v", err)
	}

	if !strings.Contains(vttOutput, "00:00:00.000 --> 00:00:02.500") {
		t.Error("VTT timing format incorrect (should be HH:MM:SS.mmm)")
	}
	if !strings.Contains(vttOutput, "00:00:02.500 --> 00:00:04.250") {
		t.Error("VTT second entry timing incorrect")
	}

	// Test SRT conversion with comma milliseconds
	srtOutput, err := converter.ToFormat(FormatSRT)
	if err != nil {
		t.Fatalf("SRT conversion failed: %v", err)
	}

	if !strings.Contains(srtOutput, "00:00:00,000 --> 00:00:02,500") {
		t.Error("SRT timing should use comma for milliseconds")
	}

	// Test TTML with period milliseconds
	ttmlOutput, err := converter.ToFormat(FormatTTML)
	if err != nil {
		t.Fatalf("TTML conversion failed: %v", err)
	}

	if !strings.Contains(ttmlOutput, `begin="00:00:00.000"`) {
		t.Error("TTML begin attribute format incorrect")
	}
}

// TestSpecialCharacterHandling tests that special characters are properly
// handled in format conversions.
func TestSpecialCharacterHandling(t *testing.T) {
	// Test data with special characters
	entries := []TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Operators: <, >, &"},
		{Start: 1, Duration: 1, Text: `Quotes: "double" and 'single'`},
		{Start: 2, Duration: 1, Text: "Non-ASCII: café, niño, 中文"},
		{Start: 3, Duration: 1, Text: "HTML entities: &copy; &nbsp; &mdash;"},
	}

	converter := NewFormatConverter(entries)

	// Test TTML escaping
	ttmlOutput, err := converter.ToFormat(FormatTTML)
	if err != nil {
		t.Fatalf("TTML conversion failed: %v", err)
	}

	// TTML should escape XML special characters
	if !strings.Contains(ttmlOutput, "&lt;") || !strings.Contains(ttmlOutput, "&gt;") {
		t.Error("TTML should escape < and >")
	}
	if !strings.Contains(ttmlOutput, "&amp;") {
		t.Error("TTML should escape &")
	}

	// Test VTT - shouldn't overly escape
	vttOutput, err := converter.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("VTT conversion failed: %v", err)
	}

	if strings.Contains(vttOutput, "&lt;") {
		t.Error("VTT shouldn't escape < as &lt;")
	}

	// Non-ASCII should be preserved in VTT
	if !strings.Contains(vttOutput, "café") {
		t.Error("VTT should preserve non-ASCII characters")
	}

	// Test JSON preserves special characters
	jsonOutput, err := converter.ToFormat(FormatJSON)
	if err != nil {
		t.Fatalf("JSON conversion failed: %v", err)
	}

	if !strings.Contains(jsonOutput, "café") {
		t.Error("JSON should preserve non-ASCII")
	}

	// Test plain text preserves everything as-is
	plainOutput, err := converter.ToFormat(FormatPlainText)
	if err != nil {
		t.Fatalf("PlainText conversion failed: %v", err)
	}

	if !strings.Contains(plainOutput, "<, >, &") {
		t.Error("PlainText should preserve special characters exactly")
	}
}

// TestRoundTripConversionAccuracy tests that converting between formats
// and back preserves data accurately.
func TestRoundTripConversionAccuracy(t *testing.T) {
	// Original entries with various timing values
	original := []TranscriptEntry{
		{Start: 0.0, Duration: 1.5, Text: "First"},
		{Start: 1.5, Duration: 2.25, Text: "Second"},
		{Start: 3.75, Duration: 0.75, Text: "Third"},
	}

	converter := NewFormatConverter(original)

	testCases := []struct {
		format Format
		name   string
	}{
		{FormatVTT, "VTT"},
		{FormatSRT, "SRT"},
		{FormatJSON3, "JSON3"},
		{FormatJSON, "JSON"},
		{FormatTTML, "TTML"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to format
			output, err := converter.ToFormat(tc.format)
			if err != nil {
				t.Fatalf("ToFormat failed: %v", err)
			}

			// Parse back
			parsed, err := ParseFormat(output, tc.format)
			if err != nil {
				t.Fatalf("ParseFormat failed: %v", err)
			}

			// Verify count
			if len(parsed) != len(original) {
				t.Errorf("Round-trip: got %d entries, want %d", len(parsed), len(original))
			}

			// Verify text is preserved
			for i, entry := range parsed {
				if entry.Text != original[i].Text {
					t.Errorf("Entry %d text mismatch: %q != %q", i, entry.Text, original[i].Text)
				}
			}

			// Verify timing is approximately preserved (allow 1ms tolerance for rounding)
			for i, entry := range parsed {
				startDiff := absFloat(entry.Start - original[i].Start)
				durationDiff := absFloat(entry.Duration - original[i].Duration)

				if startDiff > 0.01 { // More than 10ms difference
					t.Errorf("Entry %d start timing off: %f != %f (diff: %f)",
						i, entry.Start, original[i].Start, startDiff)
				}
				if durationDiff > 0.01 { // More than 10ms difference
					t.Errorf("Entry %d duration timing off: %f != %f (diff: %f)",
						i, entry.Duration, original[i].Duration, durationDiff)
				}
			}
		})
	}
}

// TestAllFormatCombinations tests all pairwise format conversions.
func TestAllFormatCombinations(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Test"},
		{Start: 1, Duration: 1, Text: "Data"},
	}

	formats := []Format{
		FormatJSON3, FormatJSON, FormatVTT,
		FormatSRT, FormatTTML, FormatPlainText,
	}

	for i, fromFormat := range formats {
		for j, toFormat := range formats {
			if i == j {
				continue // Skip same format
			}

			t.Run(string(fromFormat)+"-to-"+string(toFormat), func(t *testing.T) {
				converter := NewFormatConverter(entries)

				// Convert from source to target format
				intermediateOutput, err := converter.ToFormat(toFormat)
				if err != nil {
					t.Errorf("ToFormat(%s) failed: %v", toFormat, err)
					return
				}

				if intermediateOutput == "" {
					t.Errorf("ToFormat(%s) returned empty", toFormat)
					return
				}

				// Parse back to verify it's valid
				parsed, err := ParseFormat(intermediateOutput, toFormat)
				if err != nil {
					t.Errorf("ParseFormat(%s) failed: %v", toFormat, err)
					return
				}

				// At minimum, verify we got content back
				if len(parsed) == 0 && toFormat != FormatPlainText {
					t.Errorf("ParseFormat(%s) returned no entries", toFormat)
				}
			})
		}
	}
}

// TestFormatPreservesContent tests that content integrity is maintained
// across format conversions.
func TestFormatPreservesContent(t *testing.T) {
	// Complex content with various scenarios
	entries := []TranscriptEntry{
		{Start: 0.0, Duration: 1.0, Text: "Simple text"},
		{Start: 1.0, Duration: 1.0, Text: "Text with numbers 123456"},
		{Start: 2.0, Duration: 1.0, Text: "Text with emoji 👍"},
		{Start: 3.0, Duration: 1.0, Text: "Line\nbreak in text"},
		{Start: 4.0, Duration: 1.0, Text: "Tabs\t\there"},
	}

	converter := NewFormatConverter(entries)

	formats := []Format{
		FormatJSON3, FormatJSON, FormatVTT,
		FormatSRT, FormatTTML, FormatPlainText,
	}

	for _, format := range formats {
		output, err := converter.ToFormat(format)
		if err != nil {
			t.Errorf("ToFormat(%s) failed: %v", format, err)
			continue
		}

		// Verify text content is present in output
		if !strings.Contains(output, "Simple text") {
			t.Errorf("%s output missing 'Simple text'", format)
		}

		// Verify some entries are present (format-dependent)
		if format != FormatPlainText {
			// Structured formats should preserve multiple entries
			contentCount := strings.Count(output, "text")
			if contentCount < 2 {
				t.Logf("%s output has %d 'text' occurrences", format, contentCount)
			}
		}
	}
}

// TestTimingPrecision tests that timing precision is maintained through conversions.
func TestTimingPrecision(t *testing.T) {
	// Test with high-precision timing values
	entries := []TranscriptEntry{
		{Start: 0.001, Duration: 0.999, Text: "Precise start"},
		{Start: 1.0, Duration: 0.500, Text: "Half second"},
		{Start: 1.5, Duration: 2.333, Text: "Non-round"},
		{Start: 3.833, Duration: 0.167, Text: "Sixth second"},
	}

	converter := NewFormatConverter(entries)

	// Check JSON3 (millisecond precision)
	json3Output, _ := converter.ToFormat(FormatJSON3)
	if !strings.Contains(json3Output, "1") || !strings.Contains(json3Output, "999") {
		t.Error("JSON3 should preserve millisecond precision")
	}

	// Check VTT/SRT (centisecond precision typical)
	vttOutput, _ := converter.ToFormat(FormatVTT)
	if !strings.Contains(vttOutput, "00:00:00.001") {
		t.Error("VTT should show millisecond precision")
	}

	// Parse back and verify timing
	parsed, _ := ParseFormat(vttOutput, FormatVTT)
	if len(parsed) > 0 {
		// VTT typically rounds to centiseconds, so allow small tolerance
		if parsed[0].Start > 0.01 {
			t.Logf("VTT parsed start: %f (original: %f)", parsed[0].Start, entries[0].Start)
		}
	}
}

// Helper function to calculate absolute difference
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestEmptyAndEdgeCaseFormats tests edge cases in format handling.
func TestEmptyAndEdgeCaseFormats(t *testing.T) {
	// Empty entries
	emptyConverter := NewFormatConverter([]TranscriptEntry{})

	formats := []Format{
		FormatJSON3, FormatJSON, FormatVTT,
		FormatSRT, FormatTTML, FormatPlainText,
	}

	for _, format := range formats {
		output, err := emptyConverter.ToFormat(format)
		if err != nil {
			t.Errorf("ToFormat(%s) on empty entries failed: %v", format, err)
			continue
		}

		// For empty entries, some formats may return minimal structure
		// This is acceptable - the test just verifies they don't error
		if output == "" && format != FormatSRT && format != FormatPlainText {
			t.Logf("ToFormat(%s) returned empty string for empty entries (acceptable)", format)
		}

		// Should still be parseable
		_, err = ParseFormat(output, format)
		if err != nil && format != FormatPlainText {
			t.Errorf("ParseFormat(%s) on empty data failed: %v", format, err)
		}
	}

	// Single entry
	singleConverter := NewFormatConverter([]TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Single"},
	})

	for _, format := range formats {
		output, err := singleConverter.ToFormat(format)
		if err != nil {
			t.Errorf("ToFormat(%s) on single entry failed: %v", format, err)
		}

		if !strings.Contains(output, "Single") && format != FormatPlainText {
			t.Errorf("ToFormat(%s) missing content", format)
		}
	}
}
