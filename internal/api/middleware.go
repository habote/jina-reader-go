package api

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"time"
)

// statusResponseWriter membungkus http.ResponseWriter untuk merekam status code,
// serta meneruskan implementasi http.Flusher dan http.Hijacker jika ada.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

func (w *statusResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

// RecoveryMiddleware menangani unhandled panic agar server tidak crash.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v route=%s method=%s\n%s", rec, r.URL.Path, r.Method, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LoggerMiddleware mencatat method, path, remote addr, status code, dan latency eksekusi.
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		srw := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(srw, r)
		log.Printf("%s %s status=%d latency=%s remote=%s", r.Method, r.URL.Path, srw.statusCode, time.Since(start), r.RemoteAddr)
	})
}

// TimingMiddleware menghitung durasi request dan menambahkan header X-Response-Time.
func TimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		srw := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(srw, r)
		w.Header().Set("X-Response-Time", time.Since(start).String())
	})
}
