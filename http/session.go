package http

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionManager manages HTTP sessions with persistent cookies.
type SessionManager struct {
	jar      http.CookieJar
	client   *http.Client
	cookiePath string
	mu       sync.RWMutex
	config   SessionConfig
}

// SessionConfig configures session behavior.
type SessionConfig struct {
	// PersistCookies enables saving/loading cookies from disk
	PersistCookies bool

	// CookieFile is the path to save cookies (if PersistCookies is true)
	CookieFile string

	// UserAgent for HTTP requests
	UserAgent string

	// RefererURL to use in requests (helps with YouTube)
	RefererURL string

	// HeadersToAdd are custom headers to include in all requests
	HeadersToAdd map[string]string

	// CookieJarOptions for cookiejar.New (nil uses defaults)
	CookieJarOptions *cookiejar.Options
}

// DefaultSessionConfig returns sensible defaults.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		PersistCookies: false,
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		RefererURL:     "https://www.youtube.com",
		HeadersToAdd:   make(map[string]string),
	}
}

// NewSessionManager creates a new session manager.
func NewSessionManager(cfg SessionConfig) (*SessionManager, error) {
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultSessionConfig().UserAgent
	}

	// Create cookie jar
	var jar http.CookieJar
	var err error

	if cfg.CookieJarOptions != nil {
		jar, err = cookiejar.New(cfg.CookieJarOptions)
	} else {
		jar, err = cookiejar.New(nil)
	}

	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	sm := &SessionManager{
		jar:        jar,
		cookiePath: cfg.CookieFile,
		config:     cfg,
	}

	// Load cookies from file if configured
	if cfg.PersistCookies && cfg.CookieFile != "" {
		if err := sm.LoadCookies(); err != nil {
			// Log but don't fail - cookies file may not exist yet
			fmt.Printf("Warning: Failed to load cookies: %v\n", err)
		}
	}

	return sm, nil
}

// GetClient returns an HTTP client configured with session cookies and headers.
func (sm *SessionManager) GetClient(baseConfig *Config) *Client {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if baseConfig == nil {
		baseConfig = DefaultConfig()
	}

	// Create HTTP client with cookie jar
	httpClient := &http.Client{
		Timeout: baseConfig.Timeout,
		Jar:     sm.jar,
		Transport: &http.Transport{
			MaxIdleConns:        baseConfig.Transport.MaxIdleConns,
			MaxIdleConnsPerHost: baseConfig.Transport.MaxIdleConnsPerHost,
			MaxConnsPerHost:     baseConfig.Transport.MaxConnsPerHost,
			IdleConnTimeout:     baseConfig.Transport.IdleConnTimeout,
			ForceAttemptHTTP2:   baseConfig.Transport.ForceAttemptHTTP2,
			DisableKeepAlives:   baseConfig.Transport.DisableKeepAlives,
		},
	}

	// Wrap with our custom client
	client := &Client{
		base:        httpClient,
		config:      baseConfig,
		rateLimiter: NewRateLimiter(baseConfig.RateLimiter),
		session:     sm,
	}

	return client
}

// AddHeader adds a header to be included in all requests.
func (sm *SessionManager) AddHeader(key, value string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.config.HeadersToAdd[key] = value
}

// GetHeaders returns the headers to add to requests.
func (sm *SessionManager) GetHeaders() map[string]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	headers := make(map[string]string)
	for k, v := range sm.config.HeadersToAdd {
		headers[k] = v
	}

	// Add standard headers
	headers["User-Agent"] = sm.config.UserAgent
	if sm.config.RefererURL != "" {
		headers["Referer"] = sm.config.RefererURL
	}

	return headers
}

// SaveCookies saves cookies to file.
func (sm *SessionManager) SaveCookies() error {
	if !sm.config.PersistCookies || sm.cookiePath == "" {
		return nil
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Get all cookies from YouTube domain
	youtubeURL, _ := url.Parse("https://www.youtube.com")
	var cookies []*http.Cookie
	if youtubeURL != nil {
		cookies = sm.jar.Cookies(youtubeURL)
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cookies: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(sm.cookiePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cookie directory: %w", err)
	}

	// Write to file with restricted permissions
	if err := ioutil.WriteFile(sm.cookiePath, data, 0600); err != nil {
		return fmt.Errorf("write cookie file: %w", err)
	}

	return nil
}

// LoadCookies loads cookies from file.
func (sm *SessionManager) LoadCookies() error {
	if !sm.config.PersistCookies || sm.cookiePath == "" {
		return nil
	}

	// Check if file exists
	if _, err := os.Stat(sm.cookiePath); os.IsNotExist(err) {
		return nil // File doesn't exist yet, not an error
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Read file
	data, err := ioutil.ReadFile(sm.cookiePath)
	if err != nil {
		return fmt.Errorf("read cookie file: %w", err)
	}

	// Deserialize cookies
	var cookies []*http.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("unmarshal cookies: %w", err)
	}

	// Set cookies in jar for multiple domains
	domains := []string{"https://www.youtube.com", "https://youtube.com", "https://www.googleapis.com"}
	for _, domain := range domains {
		u, err := url.Parse(domain)
		if err == nil && u != nil {
			sm.jar.SetCookies(u, cookies)
		}
	}

	return nil
}

// ClearCookies removes all cookies from the session.
func (sm *SessionManager) ClearCookies() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Create new cookie jar to clear all cookies
	var jar http.CookieJar
	var err error
	if sm.config.CookieJarOptions != nil {
		jar, _ = cookiejar.New(sm.config.CookieJarOptions)
	} else {
		jar, _ = cookiejar.New(nil)
	}
	if err == nil {
		sm.jar = jar
	}
}

// SetReferer sets the referer URL.
func (sm *SessionManager) SetReferer(url string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.config.RefererURL = url
}

// GetReferer returns the current referer URL.
func (sm *SessionManager) GetReferer() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.config.RefererURL
}

// SessionExpiry checks if session cookies are expired.
func (sm *SessionManager) SessionExpiry() (time.Time, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Get all cookies from YouTube domain
	youtubeURL, _ := url.Parse("https://www.youtube.com")
	var cookies []*http.Cookie
	if youtubeURL != nil {
		cookies = sm.jar.Cookies(youtubeURL)
	}

	if len(cookies) == 0 {
		return time.Time{}, false
	}

	// Find earliest expiration
	var earliest time.Time
	found := false

	for _, cookie := range cookies {
		if cookie.Expires.IsZero() {
			continue // Session cookie, expires with browser
		}

		if !found || cookie.Expires.Before(earliest) {
			earliest = cookie.Expires
			found = true
		}
	}

	return earliest, found
}



// Close saves cookies and cleans up resources.
func (sm *SessionManager) Close() error {
	return sm.SaveCookies()
}

// CookieStore provides an interface for custom cookie storage implementations.
type CookieStore interface {
	// Load returns cookies from storage
	Load() ([]*http.Cookie, error)

	// Save persists cookies to storage
	Save(cookies []*http.Cookie) error

	// Clear removes all stored cookies
	Clear() error
}

// FileCookieStore implements CookieStore with file-based persistence.
type FileCookieStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileCookieStore creates a file-based cookie store.
func NewFileCookieStore(path string) *FileCookieStore {
	return &FileCookieStore{path: path}
}

// Load loads cookies from file.
func (fcs *FileCookieStore) Load() ([]*http.Cookie, error) {
	fcs.mu.RLock()
	defer fcs.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(fcs.path); os.IsNotExist(err) {
		return []*http.Cookie{}, nil
	}

	data, err := ioutil.ReadFile(fcs.path)
	if err != nil {
		return nil, fmt.Errorf("read cookie file: %w", err)
	}

	var cookies []*http.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("unmarshal cookies: %w", err)
	}

	return cookies, nil
}

// Save saves cookies to file.
func (fcs *FileCookieStore) Save(cookies []*http.Cookie) error {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()

	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cookies: %w", err)
	}

	dir := filepath.Dir(fcs.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := ioutil.WriteFile(fcs.path, data, 0600); err != nil {
		return fmt.Errorf("write cookie file: %w", err)
	}

	return nil
}

// Clear deletes the cookie file.
func (fcs *FileCookieStore) Clear() error {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()

	if err := os.Remove(fcs.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cookie file: %w", err)
	}

	return nil
}
