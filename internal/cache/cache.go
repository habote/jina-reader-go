package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Cache mendefinisikan interface penyimpanan ephemeral yang seragam
type Cache interface {
	// Get mengambil nilai string berdasarkan key.
	// Mengembalikan (value, true, nil) jika hit, ("", false, nil) jika miss/expired, atau error jika operasi gagal.
	Get(ctx context.Context, key string) (string, bool, error)

	// Set menyimpan key-value dengan durasi time-to-live (TTL).
	// Jika ttl <= 0, item dapat dianggap tidak kedaluwarsa atau menggunakan default TTL.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Close menutup koneksi atau menghentikan background cleaner goroutine.
	Close() error
}

// GenerateCacheKey menghasilkan cache key seragam dengan format "reader:cache:<sha256(normalizedURL)>".
func GenerateCacheKey(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		hash := sha256.Sum256([]byte(""))
		return fmt.Sprintf("reader:cache:%x", hash)
	}

	normalized := trimmed
	u, err := url.Parse(trimmed)
	if err == nil {
		// Normalisasi skema dan host ke lowercase
		u.Scheme = strings.ToLower(u.Scheme)
		u.Host = strings.ToLower(u.Host)

		// Hapus port default jika ada
		if (u.Scheme == "http" && u.Port() == "80") || (u.Scheme == "https" && u.Port() == "443") {
			u.Host = u.Hostname()
		}

		// Kosongkan fragment karena client-side anchor tidak membedakan isi dokumen
		u.Fragment = ""

		// Sort query parameters
		if u.RawQuery != "" {
			u.RawQuery = u.Query().Encode()
		}

		// Normalisasi trailing slash pada root path (misal: "http://example.com/" vs "http://example.com")
		if u.Path == "/" {
			u.Path = ""
		}

		normalized = u.String()
	} else {
		normalized = strings.ToLower(trimmed)
	}

	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("reader:cache:%x", hash)
}
