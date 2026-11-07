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

	"github.com/gocolly/colly/v2"
)

// FetchResult adalah hasil fetch satu halaman. Body selalu dibatasi
// maxBodyBytes via io.LimitReader di sisi fetcher (lapis kedua di
// bawah MaxBodySize Colly) agar kontrak lama terjaga.
type FetchResult struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentType   string
	ContentLength int64
	FinalURL      string
}

// Fetcher adalah engine fetch berbasis colly.Collector per-request
// dengan *http.Transport singleton (SafeDialer + connection pooling).
type Fetcher struct {
	transport           *http.Transport
	maxBodyBytes        int64
	userAgent           string
	allowLocalhost      bool
	timeout             time.Duration
	maxDepth            int
	respectRobots       bool
	async               bool
	customCheckRedirect func(req *http.Request, via []*http.Request) error
}

type Option func(*Fetcher)

func WithAllowLocalhost(allow bool) Option {
	return func(f *Fetcher) {
		f.allowLocalhost = allow
	}
}

// WithHTTPClient dipertahankan untuk kompatibilitas test lama: bila
// client custom diberikan, transport dari client tersebut dipakai.
// Transport Colly tetap wajib memakai SafeDialer agar SSRF tidak hilang;
// opsi ini hanya untuk test yang menyuntikkan CheckRedirect khusus.
func WithHTTPClient(client *http.Client) Option {
	return func(f *Fetcher) {
		if client != nil {
			if tr, ok := client.Transport.(*http.Transport); ok && tr != nil {
				f.transport = tr
			}
			// CheckRedirect custom dipakai saat fetch (lihat Fetch).
			f.customCheckRedirect = client.CheckRedirect
		}
	}
}

func WithMaxDepth(depth int) Option {
	return func(f *Fetcher) {
		if depth > 0 {
			f.maxDepth = depth
		}
	}
}

func WithRespectRobots(respect bool) Option {
	return func(f *Fetcher) {
		f.respectRobots = respect
	}
}

func WithAsync(async bool) Option {
	return func(f *Fetcher) {
		f.async = async
	}
}

// limitReadCloser membungkus io.LimitReader + closer asli.
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

		if ip := net.ParseIP(hostname); ip != nil {
			if IsPrivateIP(ip) {
				return nil, ErrSSRFForbidden
			}
		} else {
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
	if timeout == 0 {
		timeout = 8 * time.Second
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 5242880
	}
	f := &Fetcher{
		timeout:      timeout,
		maxBodyBytes: maxBodyBytes,
		userAgent:    userAgent,
		maxDepth:     1,
	}

	for _, opt := range opts {
		opt(f)
	}

	if f.transport == nil {
		safeDialer := NewSafeDialer(DialerConfig{
			AllowLocalhost: f.allowLocalhost,
			Timeout:        timeout,
		})
		f.transport = &http.Transport{
			DialContext:           safeDialer.DialContext,
			MaxIdleConns:          2000,
			MaxIdleConnsPerHost:   200,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		}
	}

	return f
}

// Fetch melakukan GET ke targetURL lewat satu Visit Colly.
// Signature & semantics tetap kompatibel dengan pemanggil lama:
// status code, content-type, final URL setelah redirect, dan body
// yang dibatasi maxBodyBytes.
func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
	if _, err := validateURL(targetURL, f.allowLocalhost); err != nil {
		return nil, err
	}

	res := &FetchResult{}
	var visitErr error

	// Keputusan desain #1 (issue #16): collector per-request +
	// transport singleton shared. Handler global Colly tidak bocor
	// antar-request; SafeDialer & pool tetap ter-reuse.
	c := colly.NewCollector(
		colly.MaxDepth(f.maxDepth),
		colly.Async(f.async),
		colly.AllowURLRevisit(),
		// ParseHTTPErrorResponse: status non-2xx TIDAK dianggap error
		// Colly (handleOnError) — OnResponse menerima status & body apa
		// adanya, sesuai kontrak lama di mana handler mengembalikan
		// FetchResult untuk status apa pun.
		colly.ParseHTTPErrorResponse(),
		colly.MaxBodySize(int(f.maxBodyBytes)),
		colly.UserAgent(f.userAgent),
		colly.StdlibContext(ctx),
	)
	if !f.respectRobots {
		colly.IgnoreRobotsTxt()(c)
	}
	c.SetRequestTimeout(f.timeout)

	// http.Client untuk Colly: CheckRedirect SSRF-aware (maks 10 hop).
	// Custom CheckRedirect dari test (WithHTTPClient) dihormati bila ada.
	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if _, err := validateURL(req.URL.String(), f.allowLocalhost); err != nil {
			return err
		}
		return nil
	}
	if f.customCheckRedirect != nil {
		custom := f.customCheckRedirect
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return custom(req, via)
		}
	}
	c.SetClient(&http.Client{
		Transport:     f.transport,
		Timeout:       f.timeout,
		CheckRedirect: checkRedirect,
	})

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", f.userAgent)
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9,id;q=0.8")
		// Validasi ulang URL request (SSRF) sebelum request dikirim.
		if _, err := validateURL(r.URL.String(), f.allowLocalhost); err != nil {
			r.Abort()
			visitErr = err
		}
	})

	c.OnResponse(func(r *colly.Response) {
		res.StatusCode = r.StatusCode
		if r.Headers != nil {
			res.ContentType = r.Headers.Get("Content-Type")
		}
		res.ContentLength = int64(len(r.Body))
		if r.Request != nil && r.Request.URL != nil {
			res.FinalURL = r.Request.URL.String()
		}
		// Body sudah dibatasi Colly MaxBodySize; bungkus LimitReader
		// lagi sebagai lapis pertama yang menjamin kontrak maxBodyBytes.
		res.Body = io.NopCloser(io.LimitReader(strings.NewReader(string(r.Body)), f.maxBodyBytes))
	})

	c.OnError(func(r *colly.Response, err error) {
		// Colly memanggil OnError untuk non-2xx & error jaringan.
		// Status code tetap diteruskan via OnResponse. Error di sini
		// hanya dicatat sebagai visitErr bila belum ada hasil
		// (res.Body nil) — mis. network error / timeout / SSRF.
		// HTTP error status (404/500) TIDAK dianggap fetch gagal
		// bila body sudah ada: kontrak lama mengembalikan body mentah
		// dan status code apa adanya (handler tidak mengubahnya).
		if err != nil && res.Body == nil && visitErr == nil {
			visitErr = mapCollyError(err, ctx)
		}
	})

	c.OnScraped(func(r *colly.Response) {
		_ = r
	})

	err := c.Visit(targetURL)
	// Colly mengembalikan error status HTTP non-2xx dari Visit walau
	// OnResponse sudah menangkap body. Itu bukan kegagalan fetch
	// (kontrak lama: status & body diteruskan apa adanya).
	// Hanya anggap gagal bila belum ada body yang diterima.
	if err != nil && res.Body == nil && visitErr == nil {
		visitErr = mapCollyError(err, ctx)
	}

	if res.Body == nil {
		// Tidak ada response (error sebelum OnResponse): kembalikan error.
		if visitErr != nil {
			return nil, visitErr
		}
		if err != nil {
			return nil, mapCollyError(err, ctx)
		}
		res.Body = io.NopCloser(strings.NewReader(""))
	}

	_ = visitErr // body tersedia (mis. non-HTML fallback / HTTP error): kontrak lama tetap jalan
	return res, nil
}

// mapCollyError memetakan error Colly/transport ke sentinel existing.
// Wrap agar errors.Is bekerja untuk ErrSSRFForbidden / ErrInvalidScheme.
func mapCollyError(err error, ctx context.Context) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, ErrSSRFForbidden.Error()):
		return fmt.Errorf("%w: %v", ErrSSRFForbidden, err)
	case strings.Contains(msg, ErrInvalidScheme.Error()):
		return fmt.Errorf("%w: %v", ErrInvalidScheme, err)
	case ctx.Err() == context.DeadlineExceeded,
		strings.Contains(msg, "deadline"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "Client.Timeout"),
		strings.Contains(msg, "context deadline"):
		return fmt.Errorf("%w: %v", context.DeadlineExceeded, err)
	default:
		return fmt.Errorf("request failed: %w", err)
	}
}
