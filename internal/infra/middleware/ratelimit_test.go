package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func getTestRedis(t *testing.T) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6380",
		DB:   2, // use DB 2 for ratelimit tests
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping ratelimiter test: redis not available on 127.0.0.1:6380 (%v)", err)
		return nil
	}
	return rdb
}

func TestRateLimiter_SlidingWindowEnforceAndSmoothSlide(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	ctx := context.Background()
	testPrefix := "test_slide_" + time.Now().Format("150405_000")
	testIP := "192.168.1.100"

	// Cleanup test key
	defer func() {
		_ = rdb.Del(ctx, "ratelimit:"+testPrefix+":"+testIP).Err()
	}()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Limit: 3 requests per 1000ms
	r.Use(IPRateLimiter(rdb, testPrefix, 3, 1000*time.Millisecond))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"msg": "ok"})
	})

	// 1. Send 2 requests at t=0ms -> should both pass
	for i := 1; i <= 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = testIP + ":12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request #%d should pass, got %d", i, w.Code)
		}
	}

	// Wait 500ms
	time.Sleep(500 * time.Millisecond)

	// 2. Send 3rd request at t=500ms -> should pass (total 3 in 1000ms)
	req3, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req3.RemoteAddr = testIP + ":12345"
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("request #3 should pass, got %d", w3.Code)
	}

	// 3. Send 4th request immediately at t=510ms -> should be blocked!
	req4, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req4.RemoteAddr = testIP + ":12345"
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", w4.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(w4.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response json: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != response.CodeRateLimited {
		t.Fatalf("expected error code %q, got %+v", response.CodeRateLimited, resp.Error)
	}

	// 4. Wait 600ms (total time elapsed ~1110ms):
	// The first 2 requests (from t=0) have slid out of the 1000ms window!
	// Only request #3 (from t=500ms) is still in the active window.
	time.Sleep(600 * time.Millisecond)

	// 5. This request should pass smoothly because of true sliding window!
	req5, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	req5.RemoteAddr = testIP + ":12345"
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, req5)
	if w5.Code != http.StatusOK {
		t.Fatalf("request after partial sliding window expiry should pass, got code %d: %s", w5.Code, w5.Body.String())
	}
}

func TestRateLimiter_UserRateLimiter(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	ctx := context.Background()
	testPrefix := "test_uid_" + time.Now().Format("150405_000")
	uid := int64(9999)

	defer func() {
		_ = rdb.Del(ctx, "ratelimit:"+testPrefix+":uid:9999").Err()
	}()

	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Mock middleware to inject user_id
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUserID, uid)
		c.Next()
	})
	r.Use(UserRateLimiter(rdb, testPrefix, 2, 2*time.Second))
	r.GET("/action", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"msg": "success"})
	})

	// 1st request -> OK
	req1, _ := http.NewRequest(http.MethodGet, "/action", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("req1 expected 200, got %d", w1.Code)
	}

	// 2nd request -> OK
	req2, _ := http.NewRequest(http.MethodGet, "/action", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("req2 expected 200, got %d", w2.Code)
	}

	// 3rd request -> 429
	req3, _ := http.NewRequest(http.MethodGet, "/action", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("req3 expected 429, got %d", w3.Code)
	}
}

func TestRateLimiter_FailOpenOnNilRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(IPRateLimiter(nil, "any", 1, 1*time.Minute)) // rdb is nil
	r.GET("/failopen", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"msg": "allowed"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/failopen", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when redis is nil (fail open), got %d", w.Code)
	}
}
