package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// slidingWindowScript implements a true Sliding Window Log using Redis Sorted Set (ZSet).
// KEYS[1]: rate limit key (e.g. ratelimit:auth:ip:127.0.0.1)
// ARGV[1]: current timestamp in milliseconds
// ARGV[2]: window duration in milliseconds
// ARGV[3]: max allowed requests in window
// ARGV[4]: unique request member identifier (e.g. now_ms:uuid)
var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now_ms = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local clear_before = now_ms - window_ms

-- 1. Prune all requests outside the sliding window
redis.call('ZREMRANGEBYSCORE', key, '-inf', clear_before)

-- 2. Count remaining requests in the active sliding window
local current_count = redis.call('ZCARD', key)

-- 3. Check limit
if current_count < limit then
    -- Record this request with score = now_ms
    redis.call('ZADD', key, now_ms, member)
    -- Refresh key expiration to window duration
    redis.call('PEXPIRE', key, window_ms)
    return 1
else
    return 0
end
`)

// KeyFunc extracts a rate-limiting key subject from the request context.
type KeyFunc func(c *gin.Context) (string, bool)

// RateLimiter returns a Gin middleware that enforces a true sliding window rate limit using Redis ZSet.
func RateLimiter(rdb *redis.Client, prefix string, limit int64, window time.Duration, keyFn KeyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil || keyFn == nil || limit <= 0 || window <= 0 {
			c.Next()
			return
		}

		subject, ok := keyFn(c)
		if !ok || subject == "" {
			c.Next()
			return
		}

		cleanPrefix := strings.TrimSpace(prefix)
		if cleanPrefix == "" {
			cleanPrefix = "global"
		}
		key := fmt.Sprintf("ratelimit:%s:%s", cleanPrefix, subject)
		nowMs := time.Now().UnixMilli()
		windowMs := window.Milliseconds()
		if windowMs <= 0 {
			windowMs = 1000
		}
		member := fmt.Sprintf("%d:%s", nowMs, uuid.NewString())

		ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
		defer cancel()

		res, err := slidingWindowScript.Run(ctx, rdb, []string{key}, nowMs, windowMs, limit, member).Int()
		if err != nil {
			// Fail-open strategy: log warning and let request pass to prevent Redis outage from cascading
			slog.Warn("ratelimiter redis error, failing open",
				slog.String("key", key),
				slog.String("error", err.Error()),
			)
			c.Next()
			return
		}

		if res == 0 {
			response.Fail(c, response.ErrRateLimited("Too many requests, please try again later"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// IPRateLimiter limits requests by client IP address.
func IPRateLimiter(rdb *redis.Client, prefix string, limit int64, window time.Duration) gin.HandlerFunc {
	return RateLimiter(rdb, prefix, limit, window, func(c *gin.Context) (string, bool) {
		ip := strings.TrimSpace(c.ClientIP())
		if ip == "" {
			return "", false
		}
		return ip, true
	})
}

// UserRateLimiter limits requests by authenticated UserID (falls back to IP if not authenticated).
func UserRateLimiter(rdb *redis.Client, prefix string, limit int64, window time.Duration) gin.HandlerFunc {
	return RateLimiter(rdb, prefix, limit, window, func(c *gin.Context) (string, bool) {
		if uid, ok := GetUserID(c); ok && uid > 0 {
			return "uid:" + strconv.FormatInt(uid, 10), true
		}
		ip := strings.TrimSpace(c.ClientIP())
		if ip != "" {
			return "ip:" + ip, true
		}
		return "", false
	})
}
