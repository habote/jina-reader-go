package fetcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"fe80::1", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"fc00::1", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:10.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700:4700::1111", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %s", tt.ip)
		}
		if got := IsPrivateIP(ip); got != tt.expected {
			t.Errorf("IsPrivateIP(%s) = %v, expected %v", tt.ip, got, tt.expected)
		}
	}
}

func TestSSRFRejection(t *testing.T) {
	fetcher := NewFetcher(5*time.Second, 1024*1024, "TestAgent")

	blockedURLs := []string{
		"http://127.0.0.1/",
		"http://127.0.0.1:8080/admin",
		"http://localhost/",
		"http://localhost:3000",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/",
		"file:///etc/passwd",
		"ftp://example.com/file",
	}

	ctx := context.Background()
	for _, targetURL := range blockedURLs {
		_, err := fetcher.Fetch(ctx, targetURL)
		if err == nil {
			t.Errorf("expected error for URL %s, but got nil", targetURL)
			continue
		}
		if !errors.Is(err, ErrSSRFForbidden) && !errors.Is(err, ErrInvalidScheme) {
			t.Logf("URL %s returned error: %v", targetURL, err)
		}
	}
}

func TestSSRFRedirectRejection(t *testing.T) {
	// A local server that redirects to a private IP (e.g. 127.0.0.1)
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/admin", http.StatusFound)
	}))
	defer redirectServer.Close()

	// Use WithAllowLocalhost(true) so initial request to test server succeeds,
	// but test CheckRedirect blocking
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if _, err := validateURL(req.URL.String(), false); err != nil {
				return err
			}
			return nil
		},
	}

	fetcher := NewFetcher(5*time.Second, 1024*1024, "TestAgent", WithHTTPClient(client), WithAllowLocalhost(true))

	_, err := fetcher.Fetch(context.Background(), redirectServer.URL)
	if err == nil {
		t.Fatal("expected SSRF error on redirect to 127.0.0.1, got nil")
	}

	if !errors.Is(err, ErrSSRFForbidden) && !bytes.Contains([]byte(err.Error()), []byte("forbidden")) {
		t.Errorf("expected SSRF error, got: %v", err)
	}
}

func TestLimitReader(t *testing.T) {
	// 10MB payload server
	payloadSize := 10 * 1024 * 1024 // 10 MB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		chunk := bytes.Repeat([]byte("A"), 64*1024)
		for written := 0; written < payloadSize; written += len(chunk) {
			_, _ = w.Write(chunk)
		}
	}))
	defer server.Close()

	maxBytes := int64(1024) // limit to 1KB
	fetcher := NewFetcher(5*time.Second, maxBytes, "TestAgent", WithAllowLocalhost(true))

	res, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("error reading body: %v", err)
	}

	if int64(len(data)) != maxBytes {
		t.Errorf("expected %d bytes, got %d", maxBytes, len(data))
	}
}

func TestFetchSuccessLocalhost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "CustomUA/1.0" {
			t.Errorf("unexpected User-Agent: %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<h1>Hello World</h1>"))
	}))
	defer server.Close()

	fetcher := NewFetcher(5*time.Second, 5*1024*1024, "CustomUA/1.0", WithAllowLocalhost(true))

	res, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("failed to fetch: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}
	if res.ContentType != "text/html; charset=utf-8" {
		t.Errorf("expected 'text/html; charset=utf-8', got %s", res.ContentType)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(body) != "<h1>Hello World</h1>" {
		t.Errorf("unexpected body content: %s", string(body))
	}
}

func TestFetchNonHTMLFallback(t *testing.T) {
	for _, ct := range []string{"text/plain", "application/json"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write([]byte("plain body"))
		}))
		f := NewFetcher(5*time.Second, 1024*1024, "TestAgent", WithAllowLocalhost(true))
		res, err := f.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("[%s] unexpected error: %v", ct, err)
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		server.Close()
		if string(data) != "plain body" {
			t.Errorf("[%s] body = %q", ct, string(data))
		}
		if res.StatusCode != 200 {
			t.Errorf("[%s] status = %d", ct, res.StatusCode)
		}
	}
}

func TestFetchFinalURLAndStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/go", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>F</title></head><body><p>hi</p></body></html>"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	f := NewFetcher(5*time.Second, 1024*1024, "TestAgent", WithAllowLocalhost(true))
	res, err := f.Fetch(context.Background(), server.URL+"/go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Errorf("status = %d", res.StatusCode)
	}
	if !strings.HasSuffix(res.FinalURL, "/final") {
		t.Errorf("FinalURL = %q, want suffix /final", res.FinalURL)
	}
	if !strings.Contains(res.ContentType, "text/html") {
		t.Errorf("ContentType = %q", res.ContentType)
	}
}

func TestFetchHTTPErrorStatus(t *testing.T) {
	for _, code := range []int{404, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(code)
			_, _ = w.Write([]byte("<html><head><title>Err</title></head><body><p>body</p></body></html>"))
		}))
		f := NewFetcher(5*time.Second, 1024*1024, "TestAgent", WithAllowLocalhost(true))
		res, err := f.Fetch(context.Background(), server.URL)
		server.Close()
		if err != nil {
			t.Errorf("[%d] unexpected error: %v", code, err)
		}
		if res == nil {
			t.Errorf("[%d] nil result", code)
			continue
		}
		if res.StatusCode != code {
			t.Errorf("[%d] status = %d", code, res.StatusCode)
		}
		res.Body.Close()
	}
}

func TestFetchTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		_, _ = w.Write([]byte("slow"))
	}))
	defer server.Close()
	f := NewFetcher(100*time.Millisecond, 1024*1024, "TestAgent", WithAllowLocalhost(true))
	_, err := f.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded sentinel, got: %v", err)
	}
}
