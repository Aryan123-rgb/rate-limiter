package slidingwindow

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/Aryan123-rgb/rate-limiter/memory"
	redisStore "github.com/Aryan123-rgb/rate-limiter/redis"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type FakeClock struct {
	currentTime time.Time
}

func (f *FakeClock) Now() time.Time {
	return f.currentTime
}

func (f *FakeClock) Add(d time.Duration) {
	f.currentTime = f.currentTime.Add(d)
}

func TestSlidingWindow_Backends(t *testing.T) {
	mr, err := miniredis.Run()
	assertNoError(t, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	tests := []struct {
		name  string
		store func() ratelimiter.Store
	}{
		{
			name: "InMemory",
			store: func() ratelimiter.Store {
				return memory.NewInMemoryStore()
			},
		},
		{
			name: "Redis",
			store: func() ratelimiter.Store {
				mr.FlushAll()
				return redisStore.NewRedisStore(redisClient)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("initial_window_fills_to_capacity", func(t *testing.T) {
				testInitialWindowFillsToCapacity(t, tt.store())
			})

			t.Run("requests_rejected_when_window_full", func(t *testing.T) {
				testRequestsRejectedWhenWindowFull(t, tt.store())
			})

			t.Run("window_fully_resets_after_double_window_duration", func(t *testing.T) {
				testWindowFullyResetsAfterDoubleWindowDuration(t, tt.store())
			})

			t.Run("edge_case_exact_window_boundary", func(t *testing.T) {
				testEdgeCaseExactWindowBoundary(t, tt.store())
			})

			t.Run("edge_case_just_before_window_slide", func(t *testing.T) {
				testEdgeCaseJustBeforeWindowSlide(t, tt.store())
			})

			t.Run("concurrent_requests_within_window", func(t *testing.T) {
				testConcurrentRequestsWithinWindow(t, tt.store())
			})

			t.Run("gradual_request_pattern_across_sliding_windows", func(t *testing.T) {
				testGradualRequestPatternAcrossSlidingWindows(t, tt.store())
			})

			t.Run("bursty_traffic_pattern", func(t *testing.T) {
				testBurstyTrafficPattern(t, tt.store())
			})

			t.Run("multiple_independent_users", func(t *testing.T) {
				testMultipleIndependentUsers(t, tt.store())
			})
		})
	}
}

// testInitialWindowFillsToCapacity verifies that a fresh window accepts exactly
// the configured number of requests per window
func testInitialWindowFillsToCapacity(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 30
		maxRequests   = 100
		key           = "test_initial_fill"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// t=0: Attempt to fill the window completely
	allowedCount := countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)
	assertRequestsCountAccepted(t, maxRequests, allowedCount)

	// t=0: Next request should be rejected
	isAllowed, err := sw.Allow(ctx, key, store, cfg)
	assertNoError(t, err)
	assertRequestRejected(t, isAllowed)
}

// testRequestsRejectedWhenWindowFull verifies that requests are properly rejected
// when the window is at capacity, even with small time progressions
func testRequestsRejectedWhenWindowFull(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 30
		maxRequests   = 50
		key           = "test_rejection"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// t=0: Fill window
	countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)

	// t=5: Small time progression within the window
	clock.Add(5 * time.Second)
	isAllowed, err := sw.Allow(ctx, key, store, cfg)
	assertNoError(t, err)
	assertRequestRejected(t, isAllowed)

	// t=29: Still within the same window
	clock.Add(24 * time.Second)
	isAllowed, err = sw.Allow(ctx, key, store, cfg)
	assertNoError(t, err)
	assertRequestRejected(t, isAllowed)
}

// testWindowFullyResetsAfterDoubleWindowDuration verifies that after 2x window duration
// with no requests, the state completely resets
func testWindowFullyResetsAfterDoubleWindowDuration(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 30
		maxRequests   = 100
		key       = "test_full_reset"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// t=0: Fill window
	countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)

	// t=70: More than 2 windows have passed (missedWindows >= 2)
	// State should completely reset
	clock.Add(70 * time.Second)
	allowedCount := countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)
	assertRequestsCountAccepted(t, maxRequests, allowedCount)
}

// testEdgeCaseExactWindowBoundary verifies behavior exactly at the window boundary
func testEdgeCaseExactWindowBoundary(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 30
		maxRequests   = 50
		key       = "test_exact_boundary"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// t=0: Fill window
	countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)

	// t=30: Exactly at window boundary, window should slide
	// completed window is treated as prev, the timeelapsed is 0 second
	// Available = 50 - 50 = 0 requests
	clock.Add(30 * time.Second)
	isAllowed, err := sw.Allow(ctx, key, store, cfg)
	assertNoError(t, err)
	assertRequestRejected(t, isAllowed)

	// t=30.001: Just past boundary
	// Elapsed = 0.001, Weight ≈ 0.99997
	// Still mostly weighted
	clock.Add(1 * time.Millisecond)
	isAllowed, err = sw.Allow(ctx, key, store, cfg)
	assertRequestAccepted(t, isAllowed)
}

// testEdgeCaseJustBeforeWindowSlide verifies behavior just before window slides
func testEdgeCaseJustBeforeWindowSlide(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 30
		maxRequests   = 100
		key       = "test_before_slide"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// t=0: Fill window
	countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)

	// t=29.999: still within the same window
	clock.Add(29*time.Second + 999*time.Millisecond)
	isAllowed, err := sw.Allow(ctx, key, store, cfg)
	assertNoError(t, err)
	assertRequestRejected(t, isAllowed)
}

// testConcurrentRequestsWithinWindow verifies thread-safety of the sliding window
func testConcurrentRequestsWithinWindow(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec  = 30
		maxRequests    = 100
		concurrentReqs = 1000
		key        = "test_concurrent"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	var allowedCount int32
	var wg sync.WaitGroup
	wg.Add(concurrentReqs)

	for i := 0; i < concurrentReqs; i++ {
		go func() {
			defer wg.Done()
			isAllowed, err := sw.Allow(ctx, key, store, cfg)
			assertNoError(t, err)
			if isAllowed {
				atomic.AddInt32(&allowedCount, 1)
			}
		}()
	}

	wg.Wait()
	assertRequestsCountAccepted(t, maxRequests, int(allowedCount))
}

// testGradualRequestPatternAcrossSlidingWindows simulates a realistic traffic pattern
// with requests spread out over time
func testGradualRequestPatternAcrossSlidingWindows(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 60
		maxRequests   = 100
		key       = "test_gradual"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// t=0: Send 50 requests
	countAllowedRequests(t, ctx, sw, store, cfg, key, 50)

	// t=20: Send 20 requests
	clock.Add(20 * time.Second)
	allowedCount := countAllowedRequests(t, ctx, sw, store, cfg, key, 20)
	assertRequestsCountAccepted(t, 20, allowedCount)

	// t=40: Send requests
	clock.Add(20 * time.Second)
	allowedCount = countAllowedRequests(t, ctx, sw, store, cfg, key, 30)
	assertRequestsCountAccepted(t, 30, allowedCount)
}

// testBurstyTrafficPattern verifies behavior with sudden bursts of traffic
func testBurstyTrafficPattern(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 50
		maxRequests   = 50
		key       = "test_bursty"
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// Pattern: Burst -> Quiet -> Burst -> Quiet

	// t=0: First burst - fill window
	countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)

	// t=40: window slides
	// 50 requests in 50 second, 10-50 -> 40 requests
	// Available = 50 - 40 = 10
	clock.Add(60 * time.Second)
	allowedCount := countAllowedRequests(t, ctx, sw, store, cfg, key, 10)
	assertRequestsCountAccepted(t, 10, allowedCount)

	// Another long quiet period (full reset)
	clock.Add(100 * time.Second)
	allowedCount = countAllowedRequests(t, ctx, sw, store, cfg, key, maxRequests)
	assertRequestsCountAccepted(t, maxRequests, allowedCount)
}

// testMultipleIndependentUsers verifies that different keys maintain separate state
func testMultipleIndependentUsers(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)

	const (
		windowSizeSec = 30
		maxRequests   = 50
		numUsers      = 26
	)

	cfg := ratelimiter.Config{
		WindowSize:       windowSizeSec,
		RequestPerWindow: maxRequests,
	}

	// Each user should be able to make requests independently
	var userIds []string
	for i := 0; i < numUsers; i++ {
		key := "user_" + string(rune('A'+i))
		userIds = append(userIds, key)
	}

	var wg sync.WaitGroup
	var allowedCount int32

	wg.Add(numUsers)

	for _, ids := range userIds {
		go func(){
			defer wg.Done()
			isAllowed, err := sw.Allow(ctx, ids, store, cfg)
			assertNoError(t, err)
			if isAllowed {
				atomic.AddInt32(&allowedCount, 1)
			}
		}()
	}

	wg.Wait()

	assertRequestsCountAccepted(t, numUsers, int(allowedCount))
}

// Helper Functions

func countAllowedRequests(t testing.TB, ctx context.Context, sw *SlidingWindow,
	store ratelimiter.Store, cfg ratelimiter.Config, key string, attempts int) int {
	t.Helper()
	allowedCount := 0
	for i := 0; i < attempts; i++ {
		isAllowed, err := sw.Allow(ctx, key, store, cfg)
		assertNoError(t, err)
		if isAllowed {
			allowedCount++
		}
	}
	return allowedCount
}

func assertRequestsCountAccepted(t testing.TB, expected, actual int) {
	t.Helper()
	// Allow ±1 tolerance for floating point rounding in weight calculations
	if actual < expected-1 || actual > expected+1 {
		t.Errorf("expected %d requests, got %d", expected, actual)
	}
}

func assertRequestRejected(t testing.TB, isAllowed bool) {
	t.Helper()
	if isAllowed {
		t.Errorf("request should have been rejected")
	}
}

func assertRequestAccepted(t testing.TB, isAllowed bool) {
	t.Helper()
	if !isAllowed {
		t.Errorf("request should have been rejected")
	}
}

func assertNoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
