package fetcher

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Option func(*Fetcher)

func WithAllowLocalhost(allow bool) Option {
	return func(f *Fetcher) {
		f.allowLocalhost = allow
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(f *Fetcher) {
		f.client = client
	}
}

type FetchResult struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentType   string
	ContentLength int64
	FinalURL      string
}

type Fetcher struct {
	client         *http.Client
	maxBodyBytes   int64
	userAgent      string
	allowLocalhost bool
}

// limitReadCloser wraps an io.Reader (io.LimitReader) and underlying io.Closer
type limitReadCloser struct {
	r io.Reader
	c io.Closer
}

func (lrc *limitReadCloser) Read(p []byte) (n int, err error) {
	return lrc.r.Read(p)
}

func (lrc *limitReadCloser) Close() error {
	return lrc.c.Close()
}

func validateURL(targetURL string, allowLocalhost bool) (*url.URL, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrInvalidScheme
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, fmt.Errorf("missing host in URL")
	}

	if !allowLocalhost {
		if strings.EqualFold(hostname, "localhost") {
			return nil, ErrSSRFForbidden
		}

		// If hostname is directly an IP literal
		if ip := net.ParseIP(hostname); ip != nil {
			if IsPrivateIP(ip) {
				return nil, ErrSSRFForbidden
			}
		} else {
			// DNS pre-check for hostname resolution
			ips, err := net.LookupIP(hostname)
			if err == nil {
				for _, ip := range ips {
					if IsPrivateIP(ip) {
						return nil, ErrSSRFForbidden
					}
				}
			}
		}
	}

	return u, nil
}

func NewFetcher(timeout time.Duration, maxBodyBytes int64, userAgent string, opts ...Option) *Fetcher {
	f := &Fetcher{
		maxBodyBytes: maxBodyBytes,
		userAgent:    userAgent,
	}

	for _, opt := range opts {
		opt(f)
	}

	if f.client == nil {
		safeDialer := NewSafeDialer(DialerConfig{
			AllowLocalhost: f.allowLocalhost,
			Timeout:        timeout,
		})

		transport := &http.Transport{
			DialContext:           safeDialer.DialContext,
			MaxIdleConns:          2000,
			MaxIdleConnsPerHost:   200,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		}

		f.client = &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if _, err := validateURL(req.URL.String(), f.allowLocalhost); err != nil {
					return err
				}
				return nil
			},
		}
	}

	return f
}

// Fetch performs HTTP GET to targetURL with SSRF validation, modern UA, and response body limited to maxBodyBytes.
func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
	if _, err := validateURL(targetURL, f.allowLocalhost); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,id;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	limitedBody := &limitReadCloser{
		r: io.LimitReader(resp.Body, f.maxBodyBytes),
		c: resp.Body,
	}

	return &FetchResult{
		Body:          limitedBody,
		StatusCode:    resp.StatusCode,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		FinalURL:      resp.Request.URL.String(),
	}, nil
}
