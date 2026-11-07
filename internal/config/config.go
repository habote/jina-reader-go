package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          string        // env: PORT, default: "8080"
	FetchTimeout  time.Duration // env: FETCH_TIMEOUT, default: 8s
	MaxBodyBytes  int64         // env: MAX_BODY_BYTES, default: 5242880 (5MB)
	CacheBackend  string        // env: CACHE_BACKEND, default: "memory"
	CacheTTL      time.Duration // env: CACHE_TTL, default: 10m
	UserAgent     string        // env: USER_AGENT, default modern Chrome UA
	RedisURL      string        // env: REDIS_URL, default: "redis://localhost:6379"
	CacheMaxItems int           // env: CACHE_MAX_ITEMS, default: 10000

	// Scraper options (issue #16, Colly).
	ScraperMaxDepth      int  // env: SCRAPER_MAX_DEPTH, default: 1 (single page, jangan crawl)
	ScraperAsync         bool // env: SCRAPER_ASYNC, default: false (sync agar semantics Fetch tetap blocking)
	ScraperRespectRobots bool // env: SCRAPER_RESPECT_ROBOTS, default: false (reader read-once; lihat README)
}

// DefaultUserAgent adalah UA modern Chrome pada macOS/Linux untuk menghindari blocking standar oleh web bot detector.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

func LoadConfig() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		FetchTimeout:  getEnvDuration("FETCH_TIMEOUT", 8*time.Second),
		MaxBodyBytes:  getEnvInt64("MAX_BODY_BYTES", 5242880),
		CacheBackend:  getEnv("CACHE_BACKEND", "memory"),
		CacheTTL:      getEnvDuration("CACHE_TTL", 10*time.Minute),
		UserAgent:     getEnv("USER_AGENT", DefaultUserAgent),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		CacheMaxItems: int(getEnvInt64("CACHE_MAX_ITEMS", 10000)),

		ScraperMaxDepth:      int(getEnvInt64("SCRAPER_MAX_DEPTH", 1)),
		ScraperAsync:         getEnvBool("SCRAPER_ASYNC", false),
		ScraperRespectRobots: getEnvBool("SCRAPER_RESPECT_ROBOTS", false),
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func getEnvBool(key string, defaultVal bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

func getEnvInt64(key string, defaultVal int64) int64 {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return defaultVal
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultVal
	}
	return n
}
