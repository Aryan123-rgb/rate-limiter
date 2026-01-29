package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/Aryan123-rgb/rate-limiter/memory"
	redisStore "github.com/Aryan123-rgb/rate-limiter/redis"
	"github.com/Aryan123-rgb/rate-limiter/tokenbucket"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestHTTPMiddleware(t *testing.T) {
	mr, err := miniredis.Run()
	assertError(t, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	
	tests := []struct{
		name string
		store ratelimiter.Store
	}{
		{
			name: "InMemory",
			store: memory.NewInMemoryStore(),
		},
		{
			name: "Redis",
			store: redisStore.NewRedisStore(redisClient),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

		})
	}
}

func TestTokenBucket(t *testing.T) {
	// 1. Setup Dependencies
	store := memory.NewInMemoryStore()
	strategy := tokenbucket.NewTokenBucket(ratelimiter.RealClock{})

	// Config: Capacity 5, Refill 1 per second
	// This means we can burst 5 requests immediately, then wait.
	cfg := ratelimiter.Config{
		Rate:     1.0,
		Capacity: 5.0,
	}

	// Mock Key Extractor (Always returns "user_1" for this test)
	keyFn := func(r *http.Request) string {
		return "user_1"
	}

	// 2. Create the Middleware
	mw := RateLimit(strategy, store, cfg, keyFn)

	// 3. Create a dummy handler (simulating your API)
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap it
	handler := mw(nextHandler)

	// 4. THE TEST LOGIC
	// We will send 10 requests in a loop.
	// Expected: First 5 return 200 OK. Next 5 return 429 Too Many Requests.

	for i := 1; i <= 10; i++ {
		// Create a fake request
		req := httptest.NewRequest("GET", "http://example.com/api", nil)
		// Create a recorder to capture the response
		w := httptest.NewRecorder()

		// Execute the request
		handler.ServeHTTP(w, req)

		// Check the result
		if i <= 5 {
			if w.Code != http.StatusOK {
				t.Errorf("Request %d: Expected 200 OK, got %d", i, w.Code)
			}
		} else {
			if w.Code != http.StatusTooManyRequests {
				t.Errorf("Request %d: Expected 429 TooManyRequests, got %d", i, w.Code)
			}
		}
	}
}

func TestTokenBucketConcurrency(t *testing.T) {
	store := memory.NewInMemoryStore()
	strategy := tokenbucket.NewTokenBucket(ratelimiter.RealClock{})
	cfg := ratelimiter.Config{
		Rate:     1,
		Capacity: 100,
	}
	keyFn := func(r *http.Request) string {
		return "concurrent_users"
	}

	mw := RateLimit(strategy, store, cfg, keyFn)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(nextHandler)

	successCount := 20

	results := make(chan int, 20)

	for i := 0; i < 20; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	for i := 20; i < 20; i++ {
		code := <-results
		if code == http.StatusOK {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("Race Condition Detected! Expected 10 allowed, but %d got through.", successCount)
		t.Fail() // Uncomment this to make the test actually fail
	}
}

func assertError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error occured: %v", err)
	}
}
