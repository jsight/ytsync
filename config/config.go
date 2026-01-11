// Package config manages application configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// yt-dlp settings
	YtdlpPath    string        `json:"ytdlp_path"`
	YtdlpTimeout time.Duration `json:"ytdlp_timeout"`

	// Extraction settings
	MaxVideos   int       `json:"max_videos"`
	IncludeShorts bool    `json:"include_shorts"`
	IncludeLive bool     `json:"include_live"`
	DateAfter   time.Time `json:"date_after"`
	DateBefore  time.Time `json:"date_before"`

	// Retry settings
	MaxRetries       int           `json:"max_retries"`
	InitialBackoff   time.Duration `json:"initial_backoff"`
	MaxBackoff       time.Duration `json:"max_backoff"`
	BackoffMultiplier float64      `json:"backoff_multiplier"`
}

// DefaultConfig returns configuration with safe defaults.
func DefaultConfig() *Config {
	return &Config{
		YtdlpPath:         "yt-dlp",
		YtdlpTimeout:      5 * time.Minute,
		MaxVideos:         0,
		IncludeShorts:     true,
		IncludeLive:       true,
		MaxRetries:        5,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// Load loads configuration from environment variables, config file, and applies defaults.
// Priority: env vars > config file > defaults
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Try to load from config file
	if err := cfg.loadFromFile(); err != nil {
		// Config file is optional
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load config file: %w", err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadFromFile attempts to load config from ytsync.json in current directory or home directory.
func (c *Config) loadFromFile() error {
	paths := []string{
		"ytsync.json",
		filepath.Join(os.Getenv("HOME"), ".config", "ytsync", "ytsync.json"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		if err := json.Unmarshal(data, c); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		return nil
	}

	return os.ErrNotExist
}

// loadFromEnv overrides config with environment variables.
func (c *Config) loadFromEnv() {
	if v := os.Getenv("YTSYNC_YTDLP_PATH"); v != "" {
		c.YtdlpPath = v
	}
	if v := os.Getenv("YTSYNC_YTDLP_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.YtdlpTimeout = d
		}
	}
	if v := os.Getenv("YTSYNC_MAX_VIDEOS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxVideos = n
		}
	}
	if v := os.Getenv("YTSYNC_INCLUDE_SHORTS"); v != "" {
		c.IncludeShorts = v == "true" || v == "1"
	}
	if v := os.Getenv("YTSYNC_INCLUDE_LIVE"); v != "" {
		c.IncludeLive = v == "true" || v == "1"
	}
	if v := os.Getenv("YTSYNC_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxRetries = n
		}
	}
	if v := os.Getenv("YTSYNC_INITIAL_BACKOFF"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.InitialBackoff = d
		}
	}
	if v := os.Getenv("YTSYNC_MAX_BACKOFF"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.MaxBackoff = d
		}
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	if c.YtdlpTimeout <= 0 {
		return fmt.Errorf("ytdlp_timeout must be positive")
	}
	if c.MaxVideos < 0 {
		return fmt.Errorf("max_videos must be non-negative")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}
	if c.InitialBackoff <= 0 {
		return fmt.Errorf("initial_backoff must be positive")
	}
	if c.MaxBackoff <= 0 {
		return fmt.Errorf("max_backoff must be positive")
	}
	if c.MaxBackoff < c.InitialBackoff {
		return fmt.Errorf("max_backoff must be >= initial_backoff")
	}
	if c.BackoffMultiplier <= 1 {
		return fmt.Errorf("backoff_multiplier must be > 1")
	}
	return nil
}
