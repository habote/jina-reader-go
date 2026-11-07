package config

import (
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Clear any environment variables that might be set in the environment
	t.Setenv("PORT", "")
	t.Setenv("FETCH_TIMEOUT", "")
	t.Setenv("MAX_BODY_BYTES", "")
	t.Setenv("CACHE_BACKEND", "")
	t.Setenv("CACHE_TTL", "")
	t.Setenv("USER_AGENT", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("CACHE_MAX_ITEMS", "")

	cfg := LoadConfig()

	if cfg.Port != "8080" {
		t.Errorf("expected Port 8080, got %s", cfg.Port)
	}
	if cfg.FetchTimeout != 8*time.Second {
		t.Errorf("expected FetchTimeout 8s, got %v", cfg.FetchTimeout)
	}
	if cfg.MaxBodyBytes != 5242880 {
		t.Errorf("expected MaxBodyBytes 5242880, got %d", cfg.MaxBodyBytes)
	}
	if cfg.CacheBackend != "memory" {
		t.Errorf("expected CacheBackend memory, got %s", cfg.CacheBackend)
	}
	if cfg.CacheTTL != 10*time.Minute {
		t.Errorf("expected CacheTTL 10m, got %v", cfg.CacheTTL)
	}
	if cfg.UserAgent != DefaultUserAgent {
		t.Errorf("expected UserAgent %s, got %s", DefaultUserAgent, cfg.UserAgent)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("expected RedisURL redis://localhost:6379, got %s", cfg.RedisURL)
	}
	if cfg.CacheMaxItems != 10000 {
		t.Errorf("expected CacheMaxItems 10000, got %d", cfg.CacheMaxItems)
	}
}

func TestLoadConfig_Custom(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("FETCH_TIMEOUT", "15s")
	t.Setenv("MAX_BODY_BYTES", "1048576")
	t.Setenv("CACHE_BACKEND", "redis")
	t.Setenv("CACHE_TTL", "30m")
	t.Setenv("USER_AGENT", "CustomBot/1.0")
	t.Setenv("REDIS_URL", "redis://user:pass@10.0.0.1:6379/1")
	t.Setenv("CACHE_MAX_ITEMS", "5000")

	cfg := LoadConfig()

	if cfg.Port != "9090" {
		t.Errorf("expected Port 9090, got %s", cfg.Port)
	}
	if cfg.FetchTimeout != 15*time.Second {
		t.Errorf("expected FetchTimeout 15s, got %v", cfg.FetchTimeout)
	}
	if cfg.MaxBodyBytes != 1048576 {
		t.Errorf("expected MaxBodyBytes 1048576, got %d", cfg.MaxBodyBytes)
	}
	if cfg.CacheBackend != "redis" {
		t.Errorf("expected CacheBackend redis, got %s", cfg.CacheBackend)
	}
	if cfg.CacheTTL != 30*time.Minute {
		t.Errorf("expected CacheTTL 30m, got %v", cfg.CacheTTL)
	}
	if cfg.UserAgent != "CustomBot/1.0" {
		t.Errorf("expected UserAgent CustomBot/1.0, got %s", cfg.UserAgent)
	}
	if cfg.RedisURL != "redis://user:pass@10.0.0.1:6379/1" {
		t.Errorf("expected RedisURL redis://user:pass@10.0.0.1:6379/1, got %s", cfg.RedisURL)
	}
	if cfg.CacheMaxItems != 5000 {
		t.Errorf("expected CacheMaxItems 5000, got %d", cfg.CacheMaxItems)
	}
}

func TestLoadConfig_InvalidFallbacks(t *testing.T) {
	t.Setenv("FETCH_TIMEOUT", "invalid-duration")
	t.Setenv("MAX_BODY_BYTES", "not-a-number")
	t.Setenv("CACHE_MAX_ITEMS", "not-a-number")

	cfg := LoadConfig()

	if cfg.FetchTimeout != 8*time.Second {
		t.Errorf("expected fallback FetchTimeout 8s, got %v", cfg.FetchTimeout)
	}
	if cfg.MaxBodyBytes != 5242880 {
		t.Errorf("expected fallback MaxBodyBytes 5242880, got %d", cfg.MaxBodyBytes)
	}
	if cfg.CacheMaxItems != 10000 {
		t.Errorf("expected fallback CacheMaxItems 10000, got %d", cfg.CacheMaxItems)
	}
}

func TestLoadConfig_ScraperDefaults(t *testing.T) {
	t.Setenv("SCRAPER_MAX_DEPTH", "")
	t.Setenv("SCRAPER_ASYNC", "")
	t.Setenv("SCRAPER_RESPECT_ROBOTS", "")
	cfg := LoadConfig()
	if cfg.ScraperMaxDepth != 1 {
		t.Errorf("expected ScraperMaxDepth 1, got %d", cfg.ScraperMaxDepth)
	}
	if cfg.ScraperAsync {
		t.Errorf("expected ScraperAsync false")
	}
	if cfg.ScraperRespectRobots {
		t.Errorf("expected ScraperRespectRobots false")
	}
}

func TestLoadConfig_ScraperCustom(t *testing.T) {
	t.Setenv("SCRAPER_MAX_DEPTH", "3")
	t.Setenv("SCRAPER_ASYNC", "true")
	t.Setenv("SCRAPER_RESPECT_ROBOTS", "true")
	cfg := LoadConfig()
	if cfg.ScraperMaxDepth != 3 {
		t.Errorf("expected ScraperMaxDepth 3, got %d", cfg.ScraperMaxDepth)
	}
	if !cfg.ScraperAsync {
		t.Errorf("expected ScraperAsync true")
	}
	if !cfg.ScraperRespectRobots {
		t.Errorf("expected ScraperRespectRobots true")
	}
}
