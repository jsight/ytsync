package youtube

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// Format represents a supported caption format.
type Format string

const (
	// FormatJSON3 is YouTube's internal JSON3 format
	FormatJSON3 Format = "json3"
	// FormatJSON is standard JSON format
	FormatJSON Format = "json"
	// FormatVTT is the WebVTT format
	FormatVTT Format = "vtt"
	// FormatSRT is the SubRip format
	FormatSRT Format = "srt"
	// FormatTTML is the TTML/XML Timed Text Markup Language format
	FormatTTML Format = "ttml"
	// FormatSRT1 is YouTube's SRT variant 1
	FormatSRT1 Format = "srv1"
	// FormatSRT2 is YouTube's SRT variant 2
	FormatSRT2 Format = "srv2"
	// FormatSRT3 is YouTube's SRT variant 3
	FormatSRT3 Format = "srv3"
	// FormatPlainText is plain text format (one entry per line)
	FormatPlainText Format = "txt"
)

// FormatConverter handles conversion between different caption formats.
type FormatConverter struct {
	// entries is the internal representation
	entries []TranscriptEntry
}

// NewFormatConverter creates a new format converter with the given entries.
func NewFormatConverter(entries []TranscriptEntry) *FormatConverter {
	return &FormatConverter{entries: entries}
}

// ToFormat converts the transcript to the specified format.
func (fc *FormatConverter) ToFormat(format Format) (string, error) {
	switch format {
	case FormatJSON3:
		return fc.toJSON3(), nil
	case FormatJSON:
		return fc.toJSON(), nil
	case FormatVTT:
		return fc.toVTT(), nil
	case FormatSRT:
		return fc.toSRT(), nil
	case FormatTTML:
		return fc.toTTML(), nil
	case FormatSRT1, FormatSRT2, FormatSRT3:
		return fc.toSRT(), nil // All SRT variants use same format
	case FormatPlainText:
		return fc.toPlainText(), nil
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}

// toJSON3 converts to YouTube's JSON3 format.
func (fc *FormatConverter) toJSON3() string {
	type event struct {
		TStartMs      string            `json:"tStartMs"`
		DDurationMs   string            `json:"dDurationMs"`
		Segs          []map[string]string `json:"segs,omitempty"`
	}

	events := make([]event, len(fc.entries))
	for i, entry := range fc.entries {
		startMs := int64(entry.Start * 1000)
		durationMs := int64(entry.Duration * 1000)

		segs := []map[string]string{
			{"utf8": entry.Text},
		}

		events[i] = event{
			TStartMs:    fmt.Sprintf("%d", startMs),
			DDurationMs: fmt.Sprintf("%d", durationMs),
			Segs:        segs,
		}
	}

	response := map[string]interface{}{
		"events": events,
	}

	data, _ := json.MarshalIndent(response, "", "  ")
	return string(data)
}

// toJSON converts to standard JSON format (similar to JSON3 but cleaner).
func (fc *FormatConverter) toJSON() string {
	response := map[string]interface{}{
		"entries": fc.entries,
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return string(data)
}

// toVTT converts to WebVTT format.
func (fc *FormatConverter) toVTT() string {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")

	for _, entry := range fc.entries {
		startTime := formatVTTTime(entry.Start)
		endTime := formatVTTTime(entry.Start + entry.Duration)

		sb.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))
		sb.WriteString(entry.Text)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// toSRT converts to SubRip (SRT) format.
func (fc *FormatConverter) toSRT() string {
	var sb strings.Builder

	for i, entry := range fc.entries {
		// Sequence number
		sb.WriteString(fmt.Sprintf("%d\n", i+1))

		// Timestamp
		startTime := formatSRTTime(entry.Start)
		endTime := formatSRTTime(entry.Start + entry.Duration)
		sb.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))

		// Text (escape XML/HTML if needed for SubRip)
		sb.WriteString(entry.Text)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// toTTML converts to TTML format.
func (fc *FormatConverter) toTTML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="en">` + "\n")
	sb.WriteString(`  <body>` + "\n")
	sb.WriteString(`    <div>` + "\n")

	for _, entry := range fc.entries {
		startTime := formatTTMLTime(entry.Start)
		endTime := formatTTMLTime(entry.Start + entry.Duration)

		// Escape XML special characters
		text := escapeXML(entry.Text)

		sb.WriteString(fmt.Sprintf(`      <p begin="%s" end="%s">%s</p>`+"\n",
			startTime, endTime, text))
	}

	sb.WriteString(`    </div>` + "\n")
	sb.WriteString(`  </body>` + "\n")
	sb.WriteString(`</tt>` + "\n")

	return sb.String()
}

// toPlainText converts to plain text format (one entry per line).
func (fc *FormatConverter) toPlainText() string {
	var sb strings.Builder

	for _, entry := range fc.entries {
		sb.WriteString(entry.Text)
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatVTTTime formats a time duration in seconds to WebVTT format (HH:MM:SS.mmm).
func formatVTTTime(seconds float64) string {
	duration := time.Duration(seconds * float64(time.Second))
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	secs := int(duration.Seconds()) % 60
	millis := int(duration.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, secs, millis)
}

// formatSRTTime formats a time duration in seconds to SRT format (HH:MM:SS,mmm).
func formatSRTTime(seconds float64) string {
	// SRT uses comma instead of period for milliseconds
	vttTime := formatVTTTime(seconds)
	// Replace the last period with comma
	return strings.Replace(vttTime, ".", ",", -1)
}

// formatTTMLTime formats a time duration in seconds to TTML format (HH:MM:SS.mmm).
func formatTTMLTime(seconds float64) string {
	// TTML uses period like VTT
	return formatVTTTime(seconds)
}

// escapeXML escapes special XML characters.
func escapeXML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

// ParseFormat parses a transcript from the specified format.
// This is the inverse of ToFormat.
func ParseFormat(content string, format Format) ([]TranscriptEntry, error) {
	switch format {
	case FormatJSON3:
		return parseJSON3(content)
	case FormatJSON:
		return parseJSON(content)
	case FormatVTT:
		return parseVTT(content)
	case FormatSRT:
		return parseSRT(content)
	case FormatTTML:
		return parseTTML(content)
	case FormatPlainText:
		return parsePlainText(content)
	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
}

// parseJSON3 parses YouTube's JSON3 format.
func parseJSON3(content string) ([]TranscriptEntry, error) {
	var result struct {
		Events []struct {
			TStartMs  string `json:"tStartMs"`
			DDuration string `json:"dDurationMs"`
			Segs      []struct {
				UTF8 string `json:"utf8"`
			} `json:"segs"`
		} `json:"events"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse JSON3: %w", err)
	}

	var entries []TranscriptEntry
	for _, event := range result.Events {
		var startMs, durationMs int64
		fmt.Sscanf(event.TStartMs, "%d", &startMs)
		fmt.Sscanf(event.DDuration, "%d", &durationMs)

		var text strings.Builder
		for _, seg := range event.Segs {
			text.WriteString(seg.UTF8)
		}

		entries = append(entries, TranscriptEntry{
			Start:    float64(startMs) / 1000.0,
			Duration: float64(durationMs) / 1000.0,
			Text:     text.String(),
		})
	}

	return entries, nil
}

// parseJSON parses standard JSON format.
func parseJSON(content string) ([]TranscriptEntry, error) {
	var result struct {
		Entries []TranscriptEntry `json:"entries"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return result.Entries, nil
}

// parseVTT parses WebVTT format.
func parseVTT(content string) ([]TranscriptEntry, error) {
	lines := strings.Split(content, "\n")
	var entries []TranscriptEntry

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Look for timestamp line
		if strings.Contains(line, " --> ") {
			parts := strings.Split(line, " --> ")
			if len(parts) != 2 {
				continue
			}

			start, err := parseVTTTimestamp(strings.TrimSpace(parts[0]))
			if err != nil {
				continue
			}

			end, err := parseVTTTimestamp(strings.TrimSpace(parts[1]))
			if err != nil {
				continue
			}

			// Collect text lines until empty line
			var text strings.Builder
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
				if text.Len() > 0 {
					text.WriteString(" ")
				}
				text.WriteString(strings.TrimSpace(lines[i]))
				i++
			}

			entries = append(entries, TranscriptEntry{
				Start:    start,
				Duration: end - start,
				Text:     text.String(),
			})
		}
	}

	return entries, nil
}

// parseSRT parses SubRip (SRT) format.
func parseSRT(content string) ([]TranscriptEntry, error) {
	// SRT format is similar to VTT, just use comma instead of period
	vttContent := strings.ReplaceAll(content, ",", ".")
	return parseVTT(vttContent)
}

// parseTTML parses TTML format.
func parseTTML(content string) ([]TranscriptEntry, error) {
	// Simple TTML parsing - extract p elements with begin/end attributes
	var entries []TranscriptEntry

	// Use regex to find p elements
	re := regexp.MustCompile(`<p\s+begin="([^"]+)"\s+end="([^"]+)">([^<]*)</p>`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) != 4 {
			continue
		}

		start, err := parseVTTTimestamp(match[1])
		if err != nil {
			continue
		}

		end, err := parseVTTTimestamp(match[2])
		if err != nil {
			continue
		}

		text := html.UnescapeString(match[3])

		entries = append(entries, TranscriptEntry{
			Start:    start,
			Duration: end - start,
			Text:     text,
		})
	}

	return entries, nil
}

// parsePlainText parses plain text format (one entry per line).
func parsePlainText(content string) ([]TranscriptEntry, error) {
	var entries []TranscriptEntry
	lines := strings.Split(content, "\n")

	currentTime := 0.0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		entries = append(entries, TranscriptEntry{
			Start:    currentTime,
			Duration: 1.0, // Default 1 second per line
			Text:     line,
		})
		currentTime += 1.0
	}

	return entries, nil
}

// parseVTTTimestamp parses a VTT timestamp (HH:MM:SS.mmm or MM:SS.mmm).
func parseVTTTimestamp(ts string) (float64, error) {
	parts := strings.Split(ts, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid timestamp format: %s", ts)
	}

	var hours, minutes, seconds float64

	if len(parts) == 3 {
		// HH:MM:SS.mmm
		fmt.Sscanf(parts[0], "%f", &hours)
		fmt.Sscanf(parts[1], "%f", &minutes)
		// Handle seconds with period or comma
		secondsPart := strings.NewReplacer(",", ".").Replace(parts[2])
		fmt.Sscanf(secondsPart, "%f", &seconds)
	} else {
		// MM:SS.mmm
		fmt.Sscanf(parts[0], "%f", &minutes)
		secondsPart := strings.NewReplacer(",", ".").Replace(parts[1])
		fmt.Sscanf(secondsPart, "%f", &seconds)
	}

	totalSeconds := hours*3600 + minutes*60 + seconds
	return totalSeconds, nil
}
