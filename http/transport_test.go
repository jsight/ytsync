package http

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultTransportConfig(t *testing.T) {
	cfg := DefaultTransportConfig()

	if cfg.MaxIdleConns != 20 {
		t.Errorf("MaxIdleConns = %d, want 20", cfg.MaxIdleConns)
	}
	if cfg.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", cfg.MaxIdleConnsPerHost)
	}
	if cfg.MaxConnsPerHost != 20 {
		t.Errorf("MaxConnsPerHost = %d, want 20", cfg.MaxConnsPerHost)
	}
	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", cfg.IdleConnTimeout)
	}

	if !cfg.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
	if cfg.DisableKeepAlives {
		t.Error("DisableKeepAlives should be false")
	}
}

func TestTransportConfiguration(t *testing.T) {
	cfg := &Config{
		Timeout:    30 * time.Second,
		UserAgent:  "test/1.0",
		Transport: TransportConfig{
			MaxIdleConns:        15,
			MaxIdleConnsPerHost: 8,
			MaxConnsPerHost:     16,
			IdleConnTimeout:     60 * time.Second,
			ForceAttemptHTTP2:   true,
			DisableKeepAlives:   false,
		},
	}

	client := New(cfg)
	defer client.Close()

	// Get the transport
	base := client.base
	if base == nil {
		t.Fatal("base client is nil")
	}

	transport, ok := base.Transport.(*http.Transport)
	if !ok {
		t.Fatal("transport is not *http.Transport")
	}

	// Verify transport settings
	if transport.MaxIdleConns != 15 {
		t.Errorf("MaxIdleConns = %d, want 15", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 8 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 8", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 16 {
		t.Errorf("MaxConnsPerHost = %d, want 16", transport.MaxConnsPerHost)
	}
	if transport.IdleConnTimeout != 60*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 60s", transport.IdleConnTimeout)
	}
	if transport.DisableKeepAlives {
		t.Error("DisableKeepAlives should be false")
	}
}

func TestDefaultConfig_TransportSettings(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Transport.MaxIdleConns == 0 {
		t.Error("DefaultConfig Transport.MaxIdleConns not set")
	}
	if cfg.Transport.MaxConnsPerHost == 0 {
		t.Error("DefaultConfig Transport.MaxConnsPerHost not set")
	}
}

func TestNewClient_HasHTTP2Support(t *testing.T) {
	cfg := DefaultConfig()
	client := New(cfg)
	defer client.Close()

	transport := client.base.Transport.(*http.Transport)
	if !transport.ForceAttemptHTTP2 {
		t.Error("HTTP/2 support should be enabled")
	}
}

func TestNewClient_KeepAliveEnabled(t *testing.T) {
	cfg := DefaultConfig()
	client := New(cfg)
	defer client.Close()

	transport := client.base.Transport.(*http.Transport)
	if transport.DisableKeepAlives {
		t.Error("Keep-alives should be enabled (DisableKeepAlives = false)")
	}
}

func TestClientGetTransportConfig(t *testing.T) {
	customCfg := TransportConfig{
		MaxIdleConns:        25,
		MaxIdleConnsPerHost: 12,
		MaxConnsPerHost:     25,
		IdleConnTimeout:     60 * time.Second,
	}

	cfg := &Config{
		Timeout:    30 * time.Second,
		UserAgent:  "test/1.0",
		Transport: customCfg,
	}

	client := New(cfg)
	defer client.Close()

	retrieved := client.GetTransportConfig()
	if retrieved.MaxIdleConns != 25 {
		t.Errorf("GetTransportConfig MaxIdleConns = %d, want 25", retrieved.MaxIdleConns)
	}
	if retrieved.MaxIdleConnsPerHost != 12 {
		t.Errorf("GetTransportConfig MaxIdleConnsPerHost = %d, want 12", retrieved.MaxIdleConnsPerHost)
	}
}

func TestConnectionPoolLimits(t *testing.T) {
	// Test that we can configure tight limits
	cfg := &Config{
		Timeout: 30 * time.Second,
		Transport: TransportConfig{
			MaxIdleConns:        5,
			MaxIdleConnsPerHost: 2,
			MaxConnsPerHost:     3,
		},
	}

	client := New(cfg)
	defer client.Close()

	transport := client.base.Transport.(*http.Transport)
	if transport.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 2 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 2", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 3 {
		t.Errorf("MaxConnsPerHost = %d, want 3", transport.MaxConnsPerHost)
	}
}

func TestClientClose(t *testing.T) {
	cfg := DefaultConfig()
	client := New(cfg)

	// Should not panic
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Closing again should be safe
	if err := client.Close(); err != nil {
		t.Errorf("Close again returned error: %v", err)
	}
}
