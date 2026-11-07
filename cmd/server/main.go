package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/habote/jina-reader-go/internal/api"
	"github.com/habote/jina-reader-go/internal/cache"
	"github.com/habote/jina-reader-go/internal/config"
	"github.com/habote/jina-reader-go/internal/fetcher"
)

func main() {
	cfg := config.LoadConfig()

	var c cache.Cache
	switch strings.ToLower(cfg.CacheBackend) {
	case "redis":
		rc, err := cache.NewRedisCache(cfg.RedisURL)
		if err != nil {
			log.Printf("redis cache init failed, falling back to memory: %v", err)
			c = cache.NewMemoryCache()
		} else {
			c = rc
		}
	case "none":
		c = nil
	default:
		c = cache.NewMemoryCache()
	}
	if c != nil {
		defer c.Close()
	}

	f := fetcher.NewFetcher(cfg.FetchTimeout, cfg.MaxBodyBytes, cfg.UserAgent)
	h := api.NewHandler(cfg, c, f)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.NewRouter(h),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("jina-reader-go listening on %s (port=%s, cache=%s)", addr, cfg.Port, cfg.CacheBackend)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}
