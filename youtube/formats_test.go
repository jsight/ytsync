package youtube

import (
	"strings"
	"testing"
)

func TestNewFormatConverter(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
		{Start: 2, Duration: 2, Text: "World"},
	}
	fc := NewFormatConverter(entries)
	if fc == nil {
		t.Fatal("NewFormatConverter returned nil")
	}
}

func TestToJSON3(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
		{Start: 2, Duration: 2, Text: "World"},
	}
	fc := NewFormatConverter(entries)

	output, err := fc.ToFormat(FormatJSON3)
	if err != nil {
		t.Fatalf("ToFormat(JSON3) failed: %v", err)
	}

	if !strings.Contains(output, "tStartMs") || !strings.Contains(output, "dDurationMs") {
		t.Error("JSON3 output missing expected fields")
	}
}

func TestToJSON(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
	}
	fc := NewFormatConverter(entries)

	output, err := fc.ToFormat(FormatJSON)
	if err != nil {
		t.Fatalf("ToFormat(JSON) failed: %v", err)
	}

	if !strings.Contains(output, "entries") {
		t.Error("JSON output missing 'entries' field")
	}
}

func TestToVTT(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
		{Start: 2, Duration: 2, Text: "World"},
	}
	fc := NewFormatConverter(entries)

	output, err := fc.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("ToFormat(VTT) failed: %v", err)
	}

	if !strings.Contains(output, "WEBVTT") {
		t.Error("VTT output missing WEBVTT header")
	}
	if !strings.Contains(output, " --> ") {
		t.Error("VTT output missing timestamp separator")
	}
	if !strings.Contains(output, "Hello") {
		t.Error("VTT output missing text content")
	}
}

func TestToSRT(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
		{Start: 2, Duration: 2, Text: "World"},
	}
	fc := NewFormatConverter(entries)

	output, err := fc.ToFormat(FormatSRT)
	if err != nil {
		t.Fatalf("ToFormat(SRT) failed: %v", err)
	}

	if !strings.Contains(output, "1") { // Sequence number
		t.Error("SRT output missing sequence numbers")
	}
	if !strings.Contains(output, ",") { // SRT uses comma for milliseconds
		t.Error("SRT output should use comma for milliseconds")
	}
}

func TestToTTML(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
	}
	fc := NewFormatConverter(entries)

	output, err := fc.ToFormat(FormatTTML)
	if err != nil {
		t.Fatalf("ToFormat(TTML) failed: %v", err)
	}

	if !strings.Contains(output, "<?xml") {
		t.Error("TTML output missing XML declaration")
	}
	if !strings.Contains(output, "<p") {
		t.Error("TTML output missing p element")
	}
	if !strings.Contains(output, "begin=") {
		t.Error("TTML output missing begin attribute")
	}
}

func TestToPlainText(t *testing.T) {
	entries := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
		{Start: 2, Duration: 2, Text: "World"},
	}
	fc := NewFormatConverter(entries)

	output, err := fc.ToFormat(FormatPlainText)
	if err != nil {
		t.Fatalf("ToFormat(PlainText) failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("PlainText output has %d lines, want 2", len(lines))
	}
	if lines[0] != "Hello" || lines[1] != "World" {
		t.Error("PlainText output has wrong content")
	}
}

func TestFormatVTTTime(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "00:00:00.000"},
		{1.5, "00:00:01.500"},
		{61.5, "00:01:01.500"},
		{3661.5, "01:01:01.500"},
	}

	for _, tt := range tests {
		got := formatVTTTime(tt.seconds)
		if got != tt.want {
			t.Errorf("formatVTTTime(%f) = %s, want %s", tt.seconds, got, tt.want)
		}
	}
}

func TestFormatSRTTime(t *testing.T) {
	// SRT uses comma instead of period
	result := formatSRTTime(1.5)
	if !strings.Contains(result, ",") {
		t.Error("SRT time should use comma for milliseconds")
	}
}

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input  string
		want   string
	}{
		{"<tag>", "&lt;tag&gt;"},
		{"&", "&amp;"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"normal text", "normal text"},
	}

	for _, tt := range tests {
		got := escapeXML(tt.input)
		if got != tt.want {
			t.Errorf("escapeXML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseVTTTimestamp(t *testing.T) {
	tests := []struct {
		ts   string
		want float64
	}{
		{"00:00:00.000", 0},
		{"00:00:01.500", 1.5},
		{"00:01:01.500", 61.5},
		{"01:01:01.500", 3661.5},
		{"01:30.500", 90.5}, // Without hours
	}

	for _, tt := range tests {
		got, err := parseVTTTimestamp(tt.ts)
		if err != nil {
			t.Fatalf("parseVTTTimestamp(%q) failed: %v", tt.ts, err)
		}
		if got != tt.want {
			t.Errorf("parseVTTTimestamp(%q) = %f, want %f", tt.ts, got, tt.want)
		}
	}
}

func TestRoundTripVTT(t *testing.T) {
	original := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello world"},
		{Start: 2, Duration: 2, Text: "How are you?"},
	}

	fc := NewFormatConverter(original)
	vttOutput, err := fc.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("ToFormat(VTT) failed: %v", err)
	}

	parsed, err := ParseFormat(vttOutput, FormatVTT)
	if err != nil {
		t.Fatalf("ParseFormat(VTT) failed: %v", err)
	}

	if len(parsed) != len(original) {
		t.Errorf("Round-trip VTT: got %d entries, want %d", len(parsed), len(original))
	}

	for i, entry := range parsed {
		if entry.Text != original[i].Text {
			t.Errorf("Entry %d text mismatch: %q != %q", i, entry.Text, original[i].Text)
		}
	}
}

func TestRoundTripSRT(t *testing.T) {
	original := []TranscriptEntry{
		{Start: 0, Duration: 2, Text: "Hello"},
		{Start: 2, Duration: 2, Text: "World"},
	}

	fc := NewFormatConverter(original)
	srtOutput, err := fc.ToFormat(FormatSRT)
	if err != nil {
		t.Fatalf("ToFormat(SRT) failed: %v", err)
	}

	parsed, err := ParseFormat(srtOutput, FormatSRT)
	if err != nil {
		t.Fatalf("ParseFormat(SRT) failed: %v", err)
	}

	if len(parsed) != len(original) {
		t.Errorf("Round-trip SRT: got %d entries, want %d", len(parsed), len(original))
	}
}

func TestInvalidFormat(t *testing.T) {
	fc := NewFormatConverter([]TranscriptEntry{})
	_, err := fc.ToFormat(Format("invalid"))
	if err == nil {
		t.Fatal("ToFormat should reject invalid format")
	}

	_, err = ParseFormat("", Format("invalid"))
	if err == nil {
		t.Fatal("ParseFormat should reject invalid format")
	}
}

func TestParsePlainText(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3"
	entries, err := parsePlainText(content)
	if err != nil {
		t.Fatalf("parsePlainText failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("parsePlainText returned %d entries, want 3", len(entries))
	}

	if entries[0].Text != "Line 1" {
		t.Errorf("Entry 0 text = %q, want %q", entries[0].Text, "Line 1")
	}
}

func TestEdgeCases(t *testing.T) {
	// Empty entries
	fc := NewFormatConverter([]TranscriptEntry{})
	output, err := fc.ToFormat(FormatVTT)
	if err != nil {
		t.Fatalf("ToFormat on empty entries failed: %v", err)
	}
	if !strings.Contains(output, "WEBVTT") {
		t.Error("VTT header should be present even for empty entries")
	}

	// Special characters in text
	entries := []TranscriptEntry{
		{Start: 0, Duration: 1, Text: "Hello & goodbye <tag>"},
	}
	fc = NewFormatConverter(entries)
	ttmlOutput, err := fc.ToFormat(FormatTTML)
	if err != nil {
		t.Fatalf("ToFormat(TTML) with special chars failed: %v", err)
	}
	if !strings.Contains(ttmlOutput, "&amp;") || !strings.Contains(ttmlOutput, "&lt;") {
		t.Error("Special characters should be escaped in TTML")
	}
}
