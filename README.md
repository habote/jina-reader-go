# jina-reader-go

High-speed web reader and HTML-to-Markdown converter microservice in Go, inspired by Jina AI Reader (`r.jina.ai`). Designed for extreme speed, low latency, and high throughput to feed clean Markdown to AI agents and LLM pipelines.

## Highlights
- **Fast HTTP Pipeline**: Tuned TCP/TLS connection reuse, Keep-Alive connection pooling, and DNS caching.
- **Streaming Tokenizer**: HTML-to-Markdown conversion using sequential streaming tokenizer (`net/html`) with zero DOM-tree overhead.
- **Fast Heuristic Pruning**: Strips `<script>`, `<style>`, navigation, ads, and footers in sub-millisecond CPU time.
- **Ephemeral Caching**: Redis / in-memory cache layer with sub-25ms response time on cache hits.
- **SSE Streaming**: Real-time markdown streaming via `text/event-stream`.

## Architecture & Plan
See GitHub Issue #1 for the full engineering plan and architecture specifications.
