package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client.
type Client struct {
	rdb *redis.Client
}

// New parses a redis:// URL, connects, and verifies with a ping.
func New(ctx context.Context, redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}

	rdb := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// Raw exposes the underlying client for advanced operations.
func (c *Client) Raw() *redis.Client {
	return c.rdb
}

// Close releases the connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}
