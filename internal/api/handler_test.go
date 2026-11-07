package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/habote/jina-reader-go/internal/cache"
	"github.com/habote/jina-reader-go/internal/config"
	"github.com/habote/jina-reader-go/internal/fetcher"
)

func testHandler(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()
	// Server HTML dummy untuk fetch
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Test Page</title><style>x{}</style></head><body><h1>Judul</h1><p>Isi <b>tebal</b> <a href="https://example.com">link</a></p><script>alert(1)</script></body></html>`))
	}))
	t.Cleanup(page.Close)

	cfg := &config.Config{FetchTimeout: 8000000000, MaxBodyBytes: 5242880, CacheTTL: 600000000000, UserAgent: "test-agent"}
	// hindari SSRF block untuk httptest local
	f := fetcher.NewFetcher(cfg.FetchTimeout, cfg.MaxBodyBytes, cfg.UserAgent, fetcher.WithAllowLocalhost(true))
	c := cache.NewMemoryCache()
	t.Cleanup(func() { _ = c.Close() })
	h := NewHandler(cfg, c, f)
	return h, page
}

func TestHealthz(t *testing.T) {
	h, _ := testHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.Healthz(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rec.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("want ok got %q", resp.Status)
	}
}

func TestWildcardRead_Success(t *testing.T) {
	h, page := testHandler(t)
	router := NewRouter(h)
	target := strings.TrimPrefix(page.URL, "http://")
	req := httptest.NewRequest(http.MethodGet, "/r/http://"+target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Judul") || !strings.Contains(rec.Body.String(), "tebal") {
		t.Errorf("missing markdown: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "alert") {
		t.Errorf("script leaked: %s", rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("want text/plain got %q", ct)
	}
}

func TestWildcardRead_CacheHit(t *testing.T) {
	h, page := testHandler(t)
	router := NewRouter(h)
	target := strings.TrimPrefix(page.URL, "http://")
	// miss
	req1 := httptest.NewRequest(http.MethodGet, "/r/http://"+target, nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Header().Get("X-Cache") != "MISS" {
		t.Errorf("first want MISS got %q", rec1.Header().Get("X-Cache"))
	}
	// cache async: tunggu sebentar
	// Set berjalan async; pakai polling sederhana
	hit := false
	for i := 0; i < 20; i++ {
		req2 := httptest.NewRequest(http.MethodGet, "/r/http://"+target, nil)
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Header().Get("X-Cache") == "HIT" {
			hit = true
			break
		}
	}
	if !hit {
		t.Errorf("second request should be HIT")
	}
}

func TestPostRead_JSON(t *testing.T) {
	h, page := testHandler(t)
	router := NewRouter(h)
	target := "http://" + strings.TrimPrefix(page.URL, "http://")
	body, _ := json.Marshal(ReadRequest{URL: target})
	req := httptest.NewRequest(http.MethodPost, "/read", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Judul") {
		t.Errorf("missing Judul: %s", rec.Body.String())
	}
}

func TestPostRead_Text(t *testing.T) {
	h, page := testHandler(t)
	router := NewRouter(h)
	target := "http://" + strings.TrimPrefix(page.URL, "http://")
	req := httptest.NewRequest(http.MethodPost, "/read", strings.NewReader(target))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostRead_Invalid(t *testing.T) {
	h, _ := testHandler(t)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/read", strings.NewReader(""))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 got %d", rec.Code)
	}
}

func TestResponseNegotiation_JSON(t *testing.T) {
	h, page := testHandler(t)
	router := NewRouter(h)
	// pakai path yang sama dengan slash awal agar PathValue terisi
	target := strings.TrimPrefix(page.URL, "http://")
	req := httptest.NewRequest(http.MethodGet, "/r/http://"+target, nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rec.Code, rec.Body.String())
	}
	var resp ReadResponseJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("not json: %v body=%s", err, rec.Body.String())
	}
	if resp.Title == "" || resp.Content == "" {
		t.Errorf("empty title/content: %+v", resp)
	}
}

func TestResponseNegotiation_SSE(t *testing.T) {
	h, page := testHandler(t)
	router := NewRouter(h)
	target := strings.TrimPrefix(page.URL, "http://")
	req := httptest.NewRequest(http.MethodGet, "/r/http://"+target+"?stream=true", nil)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: meta") || !strings.Contains(body, "event: done") || !strings.Contains(body, "[DONE]") {
		t.Errorf("sse missing events: %s", body)
	}
	if !strings.Contains(body, "data:") {
		t.Errorf("sse missing data: %s", body)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("want sse content-type got %q", ct)
	}
}

func TestSSRFBlocking(t *testing.T) {
	// fetcher tanpa allow localhost harus menolak 127.0.0.1
	cfg := &config.Config{FetchTimeout: 5000000000, MaxBodyBytes: 5242880, CacheTTL: 600000000000, UserAgent: "test"}
	f := fetcher.NewFetcher(cfg.FetchTimeout, cfg.MaxBodyBytes, cfg.UserAgent)
	h := NewHandler(cfg, cache.NewMemoryCache(), f)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/r/http://127.0.0.1:18080/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadGateway {
		t.Errorf("want 403/502 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	// handler dummy yang panic
	mux := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500 got %d", rec.Code)
	}
}

func TestMiddlewareHeaders(t *testing.T) {
	h, _ := testHandler(t)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Header().Get("X-Response-Time") == "" {
		t.Errorf("missing X-Response-Time")
	}
}

func TestRouterHealthzViaRouter(t *testing.T) {
	h, _ := testHandler(t)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	if rec.Code != 200 || !strings.Contains(string(b), `"status":"ok"`) {
		t.Errorf("router healthz failed: %d %s", rec.Code, string(b))
	}
}

func TestWildcardMissingURL(t *testing.T) {
	h, _ := testHandler(t)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/r/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 got %d", rec.Code)
	}
}

func TestPostRead_WrongMethod(t *testing.T) {
	h, _ := testHandler(t)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/read", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// mux pattern POST /read -> GET tidak match -> 405 dari mux itu sendiri
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Errorf("want 405/404 got %d", rec.Code)
	}
}
