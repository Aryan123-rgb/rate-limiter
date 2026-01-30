package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	slidingwindow "github.com/Aryan123-rgb/rate-limiter/algorithms/sliding_window"
	"github.com/Aryan123-rgb/rate-limiter/algorithms/tokenbucket"
	httpMiddleware "github.com/Aryan123-rgb/rate-limiter/middleware/http"
	"github.com/Aryan123-rgb/rate-limiter/storage/memory"
	"github.com/Aryan123-rgb/rate-limiter/storage/redis"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// RealClock wrapper for integration tests
type RealClock struct{}
func (c RealClock) Now() time.Time { return time.Now() }

func TestRateLimiter_Integration(t *testing.T) {
	// 1. Setup MiniRedis (So we don't need a real server running)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// 2. Define the Test Matrix
	tests := []struct {
		name     string
		strategy ratelimiter.Strategy
		store    ratelimiter.Store
		cfg      ratelimiter.Config
	}{
		{
			name:     "TokenBucket_Memory",
			strategy: tokenbucket.NewTokenBucket(RealClock{}),
			store:    memory.NewInMemoryStore(),
			cfg:      ratelimiter.Config{Rate: 10, Capacity: 5},
		},
		{
			name:     "TokenBucket_Redis",
			strategy: tokenbucket.NewTokenBucket(RealClock{}),
			store:    redisStore.NewRedisStore(redisClient),
			cfg:      ratelimiter.Config{Rate: 10, Capacity: 5},
		},
		{
			name:     "SlidingWindow_Memory",
			strategy: slidingwindow.NewSlidingWindow(RealClock{}),
			store:    memory.NewInMemoryStore(),
			cfg:      ratelimiter.Config{RequestPerWindow: 5, WindowSize: 1}, // 5 reqs per window of 1 seconds 
		},
		{
			name:     "SlidingWindow_Redis",
			strategy: slidingwindow.NewSlidingWindow(RealClock{}),
			store:    redisStore.NewRedisStore(redisClient),
			cfg:      ratelimiter.Config{RequestPerWindow: 5, WindowSize: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear Redis before each run to ensure isolation
			mr.FlushAll()

			// 3. Create the Middleware
			// We simulate a unique user "user_integration_test"
			mw := httpMiddleware.RateLimit(tt.strategy, tt.store, tt.cfg, func(r *http.Request) string {
				return "user_integration_test"
			})

			// 4. Create a dummy handler
			finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Wrap it
			handler := mw(finalHandler)

			// 5. EXECUTE TRAFFIC
			// We expect Capacity (5) requests to succeed
			for i := 1; i <= 5; i++ {
				req := httptest.NewRequest("GET", "http://example.com", nil)
				w := httptest.NewRecorder()
				
				handler.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					t.Errorf("Request %d failed. Expected 200, got %d", i, w.Code)
				}
			}

			// 6. VERIFY BLOCKING
			// The 6th request MUST fail
			req := httptest.NewRequest("GET", "http://example.com", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusTooManyRequests {
				t.Errorf("Rate limit check failed. Expected 429 Too Many Requests, got %d", w.Code)
			}
		})
	}
}