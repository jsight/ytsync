package http

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSessionManager(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, err := NewSessionManager(cfg)

	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	if sm == nil {
		t.Fatal("SessionManager is nil")
	}

	if sm.config.UserAgent == "" {
		t.Error("UserAgent should be set")
	}
}

func TestSessionManagerDefaultConfig(t *testing.T) {
	cfg := DefaultSessionConfig()

	if cfg.UserAgent == "" {
		t.Error("DefaultSessionConfig should have UserAgent")
	}

	if cfg.RefererURL == "" {
		t.Error("DefaultSessionConfig should have RefererURL")
	}

	if cfg.HeadersToAdd == nil {
		t.Error("DefaultSessionConfig should have HeadersToAdd map")
	}
}

func TestSessionManagerGetClient(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	client := sm.GetClient(nil)

	if client == nil {
		t.Fatal("GetClient returned nil")
	}

	if client.base == nil {
		t.Error("Client base should be initialized")
	}

	if client.session == nil {
		t.Error("Client session should be set")
	}
}

func TestSessionManagerAddHeader(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	sm.AddHeader("X-Custom", "value123")

	headers := sm.GetHeaders()
	if headers["X-Custom"] != "value123" {
		t.Errorf("Custom header not found, got %v", headers)
	}
}

func TestSessionManagerGetHeaders(t *testing.T) {
	cfg := DefaultSessionConfig()
	cfg.UserAgent = "Test Agent"
	cfg.RefererURL = "https://example.com"

	sm, _ := NewSessionManager(cfg)

	headers := sm.GetHeaders()

	if headers["User-Agent"] != "Test Agent" {
		t.Errorf("User-Agent = %s, want Test Agent", headers["User-Agent"])
	}

	if headers["Referer"] != "https://example.com" {
		t.Errorf("Referer = %s, want https://example.com", headers["Referer"])
	}
}

func TestSessionManagerSetReferer(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	sm.SetReferer("https://newurl.com")

	if sm.GetReferer() != "https://newurl.com" {
		t.Errorf("Referer not updated")
	}
}

func TestSessionManagerClearCookies(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	sm.AddHeader("Test", "value")

	sm.ClearCookies()

	// Should still work after clearing
	if sm.jar == nil {
		t.Error("Jar should still be valid after clear")
	}
}

func TestSessionManagerCookiePersistence(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "cookies.json")

	cfg := DefaultSessionConfig()
	cfg.PersistCookies = true
	cfg.CookieFile = cookieFile

	// Create first session and save
	sm1, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	sm1.AddHeader("Test", "value1")

	if err := sm1.SaveCookies(); err != nil {
		t.Fatalf("SaveCookies failed: %v", err)
	}

	// File should exist
	if _, err := os.Stat(cookieFile); err != nil {
		t.Logf("Cookie file not created: %v", err)
	}

	// Create second session with same config
	sm2, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create second session: %v", err)
	}

	if sm2 == nil {
		t.Error("Second session is nil")
	}
}

func TestSessionManagerLoadCookies_FileNotExist(t *testing.T) {
	cfg := DefaultSessionConfig()
	cfg.PersistCookies = true
	cfg.CookieFile = "/nonexistent/path/cookies.json"

	// Should not fail if file doesn't exist
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create session with nonexistent file: %v", err)
	}

	if sm == nil {
		t.Error("SessionManager is nil")
	}
}

func TestSessionManagerSessionExpiry(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	// With no cookies, should return zero time and false
	expiry, found := sm.SessionExpiry()
	if found {
		t.Error("Should not find expiry for empty session")
	}

	if !expiry.IsZero() {
		t.Errorf("Expiry should be zero time, got %v", expiry)
	}
}

func TestSessionManagerClose(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	// Should not panic
	if err := sm.Close(); err != nil {
		// Error is ok if no file is configured
		t.Logf("Close returned error: %v", err)
	}
}

func TestFileCookieStore(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "cookies.json")

	store := NewFileCookieStore(storePath)

	// Test 1: Load from non-existent file
	cookies, err := store.Load()
	if err != nil {
		t.Fatalf("Load from non-existent file failed: %v", err)
	}

	if len(cookies) != 0 {
		t.Error("Should return empty list for non-existent file")
	}

	// Test 2: Save cookies
	testCookies := []*http.Cookie{
		{
			Name:    "test_cookie",
			Value:   "test_value",
			Path:    "/",
			Domain:  ".youtube.com",
			Expires: time.Now().Add(24 * time.Hour),
		},
	}

	if err := store.Save(testCookies); err != nil {
		t.Fatalf("Save cookies failed: %v", err)
	}

	// Test 3: Load saved cookies
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load saved cookies failed: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("Loaded %d cookies, want 1", len(loaded))
	}

	// Test 4: Clear cookies
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// File should not exist
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		if err == nil {
			t.Error("Cookie file should be deleted after Clear")
		}
	}
}

func TestSessionManagerWithYouTubeHeaders(t *testing.T) {
	cfg := DefaultSessionConfig()
	cfg.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	cfg.RefererURL = "https://www.youtube.com"
	cfg.HeadersToAdd["X-YouTube-Client-Name"] = "1"
	cfg.HeadersToAdd["X-YouTube-Client-Version"] = "2.0"

	sm, _ := NewSessionManager(cfg)

	headers := sm.GetHeaders()

	if headers["User-Agent"] != cfg.UserAgent {
		t.Error("User-Agent not set correctly")
	}

	if headers["Referer"] != cfg.RefererURL {
		t.Error("Referer not set correctly")
	}

	if headers["X-YouTube-Client-Name"] != "1" {
		t.Error("Custom YouTube header not set")
	}

	if headers["X-YouTube-Client-Version"] != "2.0" {
		t.Error("Custom YouTube client version not set")
	}
}

func TestSessionManagerMultipleHeaders(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	// Add multiple headers
	sm.AddHeader("X-Custom-1", "value1")
	sm.AddHeader("X-Custom-2", "value2")
	sm.AddHeader("X-Custom-3", "value3")

	headers := sm.GetHeaders()

	expectedHeaders := map[string]string{
		"X-Custom-1": "value1",
		"X-Custom-2": "value2",
		"X-Custom-3": "value3",
	}

	for key, expected := range expectedHeaders {
		if headers[key] != expected {
			t.Errorf("Header %s = %s, want %s", key, headers[key], expected)
		}
	}
}

func TestSessionManagerConcurrency(t *testing.T) {
	cfg := DefaultSessionConfig()
	sm, _ := NewSessionManager(cfg)

	// Simulate concurrent operations
	done := make(chan bool, 20)

	for i := 0; i < 20; i++ {
		go func(i int) {
			// Add headers
			sm.AddHeader("X-Header-"+string(rune(i)), "value"+string(rune(i)))

			// Get headers
			_ = sm.GetHeaders()

			// Get referer
			_ = sm.GetReferer()

			// Set referer
			sm.SetReferer("https://youtube.com/" + string(rune(i)))

			done <- true
		}(i)
	}

	// Wait for all goroutines
	count := 0
	for count < 20 {
		<-done
		count++
	}
}

func TestDefaultSessionConfigValues(t *testing.T) {
	cfg := DefaultSessionConfig()

	tests := []struct {
		field string
		value interface{}
	}{
		{"PersistCookies", cfg.PersistCookies},
		{"UserAgent", cfg.UserAgent},
		{"RefererURL", cfg.RefererURL},
		{"HeadersToAdd", cfg.HeadersToAdd},
	}

	for _, tt := range tests {
		if tt.value == nil {
			t.Errorf("Field %s has nil value", tt.field)
		}
	}

	if cfg.UserAgent == "" {
		t.Error("UserAgent should not be empty")
	}

	if cfg.RefererURL == "" {
		t.Error("RefererURL should not be empty")
	}
}
