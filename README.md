# jina-reader-go

High-speed web reader and HTML-to-Markdown converter microservice in Go, inspired by Jina AI Reader (`r.jina.ai`). Designed for extreme speed, low latency, and high throughput to feed clean Markdown to AI agents and LLM pipelines.

## Features

- **Fast HTTP Pipeline**: Tuned TCP/TLS connection reuse, Keep-Alive connection pooling (`MaxIdleConns: 2000`, `MaxIdleConnsPerHost: 200`), and DNS caching.
- **Streaming Tokenizer**: HTML-to-Markdown conversion using sequential streaming tokenizer (`golang.org/x/net/html`) with zero DOM-tree overhead and `sync.Pool` buffer reuse.
- **Fast Heuristic Pruning**: Strips `<script>`, `<style>`, navigation, ads, and footers in sub-millisecond CPU time.
- **SSRF Protection**: Blocks private IPv4, loopback, link-local, CGNAT, DNS rebinding, and cloud metadata IPs at dial time and redirect validation.
- **Ephemeral Caching**: Redis / in-memory LRU cache layer with sub-25ms response time on cache hits (`X-Cache: HIT`).
- **SSE Streaming**: Real-time markdown streaming via `text/event-stream`.
- **Content Negotiation**: Raw Markdown (default), JSON, or SSE from the same endpoint.

## Architecture

```
Client -> Gateway (HTTP) -> Cache (mem/redis) -> Fetcher (SSRF dial) -> Parser (streaming) -> Stream Output
```

- `internal/config` — environment configuration.
- `internal/fetcher` — HTTP client singleton, safe dialer, `io.LimitReader` (5MB).
- `internal/parser` — streaming HTML to Markdown.
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
