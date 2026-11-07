package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/habote/jina-reader-go/internal/cache"
	"github.com/habote/jina-reader-go/internal/config"
	"github.com/habote/jina-reader-go/internal/fetcher"
	"github.com/habote/jina-reader-go/internal/parser"
)

// Handler mendefinisikan controller HTTP dan dependensi layanannya.
type Handler struct {
	cfg     *config.Config
	cache   cache.Cache
	fetcher *fetcher.Fetcher
}

// NewHandler membuat Handler baru.
func NewHandler(cfg *config.Config, c cache.Cache, f *fetcher.Fetcher) *Handler {
	return &Handler{cfg: cfg, cache: c, fetcher: f}
}

// Healthz menangani GET /healthz.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}

// HandleWildcardRead menangani GET /r/{url...}.
func (h *Handler) HandleWildcardRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	rawURL := normalizeWildcardURL(r)
	if rawURL == "" {
		http.Error(w, "missing target url: use /r/<url>", http.StatusBadRequest)
		return
	}
	h.processURL(w, r, rawURL)
}

// HandlePostRead menangani POST /read.
func (h *Handler) HandlePostRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	rawURL, err := readURLFromBody(r)
	if err != nil || strings.TrimSpace(rawURL) == "" {
		http.Error(w, "invalid request: expected {\"url\": \"https://...\"} or plain text URL", http.StatusBadRequest)
		return
	}
	h.processURL(w, r, normalizeURL(rawURL))
}

func normalizeWildcardURL(r *http.Request) string {
	// Prioritas: PathValue (jika pattern {url...} dipakai), fallback strip prefix /r/
	if v := r.PathValue("url"); v != "" {
		return normalizeURL(v)
	}
	if rest := stripRPrefix(r); rest != "" {
		return normalizeURL(rest)
	}
	// Fallback terakhir: r.URL.Path
	p := strings.TrimPrefix(r.URL.Path, "/r/")
	if p == r.URL.Path {
		return ""
	}
	return normalizeURL(p)
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	// Tanpa skema: default https
	return "https://" + s
}

func readURLFromBody(r *http.Request) (string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return "", err
	}
	contentType := r.Header.Get("Content-Type")
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", fmt.Errorf("empty body")
	}
	if strings.Contains(contentType, "application/json") || strings.HasPrefix(text, "{") {
		var req ReadRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return "", err
		}
		return req.URL, nil
	}
	return text, nil
}

func wantsStream(r *http.Request) bool {
	if r.URL.Query().Get("stream") == "true" {
		return true
	}
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		if strings.TrimSpace(part) == "text/event-stream" {
			return true
		}
	}
	return false
}

func wantsJSON(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		if strings.TrimSpace(part) == "application/json" {
			return true
		}
	}
	return false
}

// processURL adalah core flow: cache -> fetch -> parse -> cache async -> respond.
func (h *Handler) processURL(w http.ResponseWriter, r *http.Request, targetURL string) {
	ctx := r.Context()

	if h.cache != nil {
		key := cache.GenerateCacheKey(targetURL)
		if cached, found, err := h.cache.Get(ctx, key); err == nil && found {
			title, markdown := decodeCached(cached)
			w.Header().Set("X-Cache", "HIT")
			h.writeResponse(w, r, targetURL, title, markdown)
			return
		}
		w.Header().Set("X-Cache", "MISS")
	} else {
		w.Header().Set("X-Cache", "NONE")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, h.cfg.FetchTimeout)
	defer cancel()

	fetchRes, err := h.fetcher.Fetch(fetchCtx, targetURL)
	if err != nil {
		switch {
		case errors.Is(err, fetcher.ErrSSRFForbidden):
			http.Error(w, "Forbidden: SSRF blocked", http.StatusForbidden)
		case errors.Is(err, fetcher.ErrInvalidScheme):
			http.Error(w, "Bad Request: only http/https allowed", http.StatusBadRequest)
		case errors.Is(err, context.DeadlineExceeded),
			errors.Is(fetchCtx.Err(), context.DeadlineExceeded),
			strings.Contains(err.Error(), "timeout"),
			strings.Contains(err.Error(), "deadline"):
			http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		default:
			http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		}
		return
	}
	defer fetchRes.Body.Close()

	// Parse tetap lewat adaptor ParseHTML(io.Reader) agar kontrak
	// handler tidak berubah; di dalam, adaptor memakai DOM goquery
	// (ParseDocument) sesuai refactor Colly issue #16.
	parsed, err := parser.ParseHTML(fetchRes.Body)
	if err != nil {
		http.Error(w, "Unprocessable Entity: parse error", http.StatusUnprocessableEntity)
		return
	}

	// Simpan cache async (non-blocking)
	if h.cache != nil {
		key := cache.GenerateCacheKey(targetURL)
		serialized := encodeCached(parsed.Title, parsed.Markdown)
		go func(k, v string) {
			_ = h.cache.Set(context.Background(), k, v, h.cfg.CacheTTL)
		}(key, serialized)
	}

	h.writeResponse(w, r, targetURL, parsed.Title, parsed.Markdown)
}

func encodeCached(title, markdown string) string {
	b, _ := json.Marshal(map[string]string{"title": title, "markdown": markdown})
	return string(b)
}

func decodeCached(s string) (string, string) {
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err == nil {
		return m["title"], m["markdown"]
	}
	return "", s
}

func (h *Handler) writeResponse(w http.ResponseWriter, r *http.Request, targetURL, title, markdown string) {
	switch {
	case wantsStream(r):
		h.writeSSE(w, r, targetURL, title, markdown)
	case wantsJSON(r):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(ReadResponseJSON{Title: title, URL: targetURL, Content: markdown})
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(markdown))
	}
}

func (h *Handler) writeSSE(w http.ResponseWriter, r *http.Request, targetURL, title, markdown string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(markdown))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	meta, _ := json.Marshal(map[string]string{"title": title, "url": targetURL})
	fmt.Fprintf(w, "event: meta\ndata: %s\n\n", string(meta))
	flusher.Flush()

	for _, line := range strings.Split(markdown, "\n") {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}
	fmt.Fprintf(w, "event: done\ndata: [DONE]\n\n")
	flusher.Flush()
}
