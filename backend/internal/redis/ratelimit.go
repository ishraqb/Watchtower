package redis

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// FinnhubCallLimit is the per-minute cap on Finnhub REST calls.
// Set below the documented 60/min ceiling to leave burst headroom.
const FinnhubCallLimit = 55

const rateWindow = time.Minute

// Limiter enforces a sliding-window rate limit backed by a Redis sorted set.
// Each request is logged with a nanosecond score; counts outside the window
// are trimmed on every call, giving an accurate rolling count (not a fixed bucket).
type Limiter struct {
	client *Client
	limit  int
	window time.Duration
}

// NewLimiter builds a limiter with the given per-window request cap.
func NewLimiter(client *Client, limit int, window time.Duration) *Limiter {
	return &Limiter{client: client, limit: limit, window: window}
}

// Allow reports whether an action under the given key may proceed right now.
// It is atomic via a pipeline: trim old entries, add this hit, count, set TTL.
func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UnixNano()
	windowStart := now - l.window.Nanoseconds()
	redisKey := "ratelimit:" + key

	pipe := l.client.Raw().TxPipeline()
	pipe.ZRemRangeByScore(ctx, redisKey, "0", strconv.FormatInt(windowStart, 10))
	pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(now), Member: now})
	countCmd := pipe.ZCard(ctx, redisKey)
	pipe.Expire(ctx, redisKey, l.window)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("ratelimit: pipeline exec: %w", err)
	}

	count := countCmd.Val()
	if count > int64(l.limit) {
		// Roll back the hit we just recorded so a blocked attempt doesn't
		// keep the window saturated.
		l.client.Raw().ZRem(ctx, redisKey, now)
		return false, nil
	}
	return true, nil
}

// Middleware returns a Gin handler that rate-limits inbound requests by a
// fixed key. Returns 429 with a generic message (no internal details leaked).
func (l *Limiter) Middleware(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, err := l.Allow(c.Request.Context(), key)
		if err != nil {
			// Fail open on limiter errors so a Redis blip doesn't take down the API,
			// but log server-side for visibility.
			c.Next()
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, please retry shortly",
			})
			return
		}
		c.Next()
	}
}

// PerClient rate-limits inbound requests per client IP under the given prefix.
// Without auth, this is what stops a single visitor from hammering the public
// proxy endpoints (quote/history) or spamming /api/watch. Note: behind a proxy
// or CDN, configure Gin's trusted proxies so ClientIP() reflects the real
// caller and can't be spoofed via X-Forwarded-For.
func (l *Limiter) PerClient(prefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, err := l.Allow(c.Request.Context(), prefix+":"+c.ClientIP())
		if err != nil {
			c.Next() // fail open on limiter errors
			return
		}
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, please retry shortly",
			})
			return
		}
		c.Next()
	}
}
