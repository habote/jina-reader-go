FROM golang:1.26-alpine AS builder
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X github.com/habote/jina-reader-go/internal/api.GitCommit=${GIT_COMMIT} -X github.com/habote/jina-reader-go/internal/api.BuildTime=${BUILD_TIME}" \
    -trimpath \
    -o /bin/jina-reader ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /bin/jina-reader /bin/jina-reader
ENV TZ=UTC
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/bin/jina-reader"]
