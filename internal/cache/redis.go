package cache

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache mengimplementasikan interface Cache dengan go-redis
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache membuat instance RedisCache baru. Mendukung URL ("redis://user:pass@host:port/db") maupun host:port biasa.
func NewRedisCache(redisURL string) (*RedisCache, error) {
	var opts *redis.Options
	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") || strings.HasPrefix(redisURL, "unix://") {
		parsedOpts, err := redis.ParseURL(redisURL)
		if err != nil {
			return nil, err
		}
		opts = parsedOpts
	} else {
		opts = &redis.Options{
			Addr: redisURL,
		}
	}

	client := redis.NewClient(opts)
	return &RedisCache{client: client}, nil
}

// NewRedisCacheFromClient membuat instance RedisCache dari client go-redis yang sudah ada (mempermudah testing)
func NewRedisCacheFromClient(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

// Get mengambil nilai string berdasarkan key dari Redis.
func (r *RedisCache) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	return val, true, nil
}

// Set menyimpan key-value ke Redis dengan TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Close menutup koneksi client Redis.
func (r *RedisCache) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
