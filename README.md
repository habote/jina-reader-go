# jina-reader-go

High-speed web reader and HTML-to-Markdown converter microservice in Go, inspired by Jina AI Reader (`r.jina.ai`). Designed for extreme speed, low latency, and high throughput to feed clean Markdown to AI agents and LLM pipelines.

## Features

- **Fast HTTP Pipeline**: Tuned TCP/TLS connection reuse, Keep-Alive connection pooling (`MaxIdleConns: 2000`, `MaxIdleConnsPerHost: 200`), and DNS caching.
- **Colly DOM Parser**: HTML-to-Markdown conversion via [`github.com/gocolly/colly/v2`](https://github.com/gocolly/colly) (`Collector` per-request + `*http.Transport` singleton) dan konversi DOM → Markdown berbasis selektor `goquery` (`internal/parser/document.go`). `sync.Pool` buffer reuse dipertahankan untuk writer Markdown.
- **DOM vs Streaming Tradeoff**: Implementasi lama streaming tokenizer (`golang.org/x/net/html.NewTokenizer`, token demi token, tanpa DOM tree) diganti DOM tree in-memory. Keunggulan zero-alloc/sub-millisecond dari issue #1 dikorbankan demi kesederhanaan & maintainability (satu pipeline deklaratif `OnRequest → OnResponse → OnHTML → OnScraped`). Lihat tabel benchmark di bawah. Body tetap dibatasi berlapis (`Colly.MaxBodySize` + `io.LimitReader`) agar halaman raksasa tidak OOM.
- **Fast Heuristic Pruning**: Strips `<script>`, `<style>`, navigation, ads, and footers via `Selection.Remove()` pada prune set yang sama seperti sebelumnya.
- **SSRF Protection**: Blocks private IPv4, loopback, link-local, CGNAT, DNS rebinding, and cloud metadata IPs at dial time and redirect validation.
- **Ephemeral Caching**: Redis / in-memory LRU cache layer with sub-25ms response time on cache hits (`X-Cache: HIT`).
- **SSE Streaming**: Real-time markdown streaming via `text/event-stream`.
- **Content Negotiation**: Raw Markdown (default), JSON, or SSE from the same endpoint.

## Architecture

```
Client -> Gateway (HTTP) -> Cache (mem/redis) -> Fetcher (Colly Collector + SSRF SafeDialer transport) -> Parser (goquery DOM -> Markdown) -> Stream Output
```

- `internal/config` — environment configuration.
- `internal/fetcher` — Colly `Collector` per-request, `*http.Transport` singleton + SafeDialer (SSRF), `MaxBodySize` + `io.LimitReader` (5MB).
- `internal/parser` — DOM (goquery) → Markdown (`ParseDocument`); `ParseHTML(io.Reader)` dipertahankan sebagai adaptor.
- `internal/cache` — memory LRU / Redis.
- `internal/api` — router, handlers, middleware.
- `cmd/server` — entrypoint, graceful shutdown.

## Run locally

Requires Go 1.26+.

```bash
go run ./cmd/server
# atau
go build -o bin/reader ./cmd/server && ./bin/reader
```

Server listens on `:8080` by default. See `/ping` and `/healthz` below.

## Docker

Build image:

```bash
docker build -t jina-reader-go:latest .
```

Run (in-memory cache):

```bash
docker run -d --name jina-reader -p 8080:8080 jina-reader-go:latest
```

Run with Redis:

```bash
docker network create reader-net
docker run -d --name redis --net reader-net redis:7-alpine
docker run -d --name jina-reader --net reader-net -p 8080:8080 \
  -e CACHE_BACKEND=redis \
  -e REDIS_URL=redis://redis:6379 \
  jina-reader-go:latest
```

Image is multi-stage (`golang:1.26-alpine` builder → `distroless/static:nonroot` runtime, non-root, ~5MB content / ~20MB on disk).

## API usage

Base URL: `http://localhost:8080`

### Healthcheck

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

### Ping / version

```bash
curl http://localhost:8080/ping
# {"status":"ok","version":"0.1.0","uptime":"running","git_commit":"...","build_time":"..."}
```

### Direct raw Markdown (`GET /r/*`)

```bash
curl http://localhost:8080/r/https://example.com
```

Note: quote the URL in the shell if it contains `&` or other special characters, e.g.:

```bash
curl 'http://localhost:8080/r/https://example.com/page?a=1&b=2'
```

### JSON output

```bash
curl -H "Accept: application/json" http://localhost:8080/r/https://example.com
# {"title":"Example Domain","url":"https://example.com","content":"..."}
```

### SSE streaming

```bash
curl -N http://localhost:8080/r/https://example.com?stream=true
# atau
curl -N -H "Accept: text/event-stream" http://localhost:8080/r/https://example.com
```

### POST endpoint (`/read`)

```bash
curl -X POST http://localhost:8080/read \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

Plain text body also works:

```bash
curl -X POST http://localhost:8080/read -H "Content-Type: text/plain" -d 'https://example.com'
```

Every read response includes `X-Cache: HIT|MISS` and `X-Response-Time` headers.

## Configuration

| Variable | Default | Type | Description |
|---|---|---|---|
| `PORT` | `8080` | string | HTTP listener port |
| `FETCH_TIMEOUT` | `8s` | duration | Timeout for fetching target content |
| `MAX_BODY_BYTES` | `5242880` | int64 | Max external response body size (5MB) |
| `CACHE_BACKEND` | `memory` | string | Cache driver: `memory`, `redis`, or `none` |
| `CACHE_TTL` | `10m` | duration | TTL for converted content |
| `REDIS_URL` | `redis://localhost:6379` | string | Redis connection URL (if `CACHE_BACKEND=redis`) |
| `CACHE_MAX_ITEMS` | `10000` | int | Max items for in-memory LRU cache |
| `USER_AGENT` | Chrome modern UA | string | Outbound fetch User-Agent header |
| `SCRAPER_MAX_DEPTH` | `1` | int | Colly `MaxDepth` — default single-page (jangan crawl link) |
| `SCRAPER_ASYNC` | `false` | bool | Colly async mode; default sync agar `Fetch` tetap blocking & sederhana |
| `SCRAPER_RESPECT_ROBOTS` | `false` | bool | Hormati `robots.txt`; default `false` (reader read-once seperti Jina tidak diblokir robots) |

## Testing

```bash
go test -v -race ./...
go test -bench=BenchmarkParseHTML -benchmem ./internal/parser/...
go vet ./...
docker build -t jina-reader-go:test .
```

## Architecture & Plan

See GitHub Issue #1 for the full engineering plan and architecture specifications.

<!-- ai-enhance -->

## Scraper design (issue #16: Colly)

| Keputusan desain | Pilihan | Alasan |
|---|---|---|
| Lifecycle Collector | **Collector per-request + `*http.Transport` singleton shared** | Handler global Colly tidak bocor antar-request; SafeDialer & connection pool tetap ter-reuse. Overhead pembuatan collector diukur di benchmark (di bawah). |
| Mode sync vs async | Default **sync** (`Async(false)`); `SCRAPER_ASYNC=true` opsional | Semantics `Fetch` tetap blocking & sederhana. Async hanya bila benchmark membuktikan manfaat thread-safety-nya. |
| Body limit berlapis | Colly `MaxBodySize` + `io.LimitReader` di `OnResponse` | Respons > `MAX_BODY_BYTES` (default 5MB) terpotong aman tanpa OOM (test `TestLimitReader`). |
| Robots.txt | Default **tidak dihormati** (`SCRAPER_RESPECT_ROBOTS=false` → `colly.IgnoreRobotsTxt()`) | Reader seperti Jina melakukan read-once. Set `true` untuk menghormati robots.txt. |
| Fetch API | **Kompatibel**: `Fetch(ctx, targetURL) (*FetchResult, error)` tetap sama | Status code, content-type, final URL setelah redirect, body terbatas `MaxBodyBytes`. Handler tidak perlu dimigrasi; mapping error Colly (`OnError`) ke 400/403/502/504/422 tetap via sentinel `ErrSSRFForbidden`/`ErrInvalidScheme`/`context.DeadlineExceeded`. |
| JS-rendered / SPA | **Di luar scope** | Colly bukan headless browser — halaman SPA tetap tidak ter-render, sama seperti sebelum refactor. |

### Benchmark Colly (DOM) vs streaming lama

Perintah:

```bash
go test -run=NONE -bench=. -benchmem ./internal/parser/
```

Dokumen benchmark 50 paragraf (heading, bold, link, script):

| Implementasi | ns/op | B/op | allocs/op |
|---|---|---|---|
| Streaming tokenizer (lama, sebelum refactor) | 73,664 | 30,177 | 1,329 |
| DOM goquery (`ParseDocument`, sesudah refactor) | ~179,000–186,000 | ~144,000 | 2,292 |

Angka diukur di AMD EPYC, Go 1.27 (`-benchmem`). DOM ~2.4× lebih lambat & ~4.8× lebih boros memori pada dokumen benchmark — tradeoff yang diterima demi kesederhanaan pipeline deklaratif dan maintainability (hapus ~428 baris state machine token-by-token). Mitigasi OOM: body limit berlapis (lihat di atas).

> Catatan: `golang.org/x/net/html` masih terpakai sebagai transitive dependency via `goquery` (parser DOM), bukan untuk parsing utama manual.
