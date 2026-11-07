package api

import (
	"net/http"
	"net/url"
	"strings"
)

// NewRouter menyusun rute dan membungkusnya dengan middleware chain.
// GET /r/* ditangani di luar http.ServeMux karena ServeMux melakukan
// redirect cleanPath untuk path mengandung "//" yang memecah target URL.
// Handler /r/* manual membaca RequestURI mentah.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", PingHandler)
	if h != nil {
		mux.HandleFunc("GET /healthz", h.Healthz)
		mux.HandleFunc("POST /read", h.HandlePostRead)
		return RecoveryMiddleware(LoggerMiddleware(TimingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Route /r/* manual (jangan lewat ServeMux cleanPath)
			if len(r.URL.Path) >= 3 && r.URL.Path[:3] == "/r/" {
				if r.Method != http.MethodGet {
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
					return
				}
				h.HandleWildcardRead(w, r)
				return
			}
			mux.ServeHTTP(w, r)
		}))))
	}
	return RecoveryMiddleware(LoggerMiddleware(TimingMiddleware(mux)))
}

// stripRPrefix mengambil target URL dari request path mentah /r/<url>.
// Mendukung URL encoded (%2F%2F) maupun raw (http://...).
func stripRPrefix(r *http.Request) string {
	uri := r.RequestURI
	if i := strings.Index(uri, "?"); i >= 0 {
		uri = uri[:i]
	}
	if !strings.HasPrefix(uri, "/r/") {
		return ""
	}
	rest := uri[3:]
	if decoded, err := url.PathUnescape(rest); err == nil {
		rest = decoded
	}
	return strings.TrimSpace(rest)
}
