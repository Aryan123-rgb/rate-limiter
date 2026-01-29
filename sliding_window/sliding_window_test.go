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

func TestSlidingWindow(t *testing.T) {
	mr, err := miniredis.Run()
	assertError(t, err)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

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
			t.Run("test_concurrent_burst_requests", func(t *testing.T) {
				testConcurrentBurstRequests(t, tt.store())
			})
		})
	}
}

func testConcurrentBurstRequests(t *testing.T, store ratelimiter.Store) {
	cfg := ratelimiter.Config{
		WindowSize:       1,
		RequestPerWindow: 30,
	}
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)
	ctx := context.Background()
	key := "test_concurrent_requests"

	var wg sync.WaitGroup
	var allowedCount int32
	totalRequests := 100

	wg.Add(totalRequests)

	for i := 0; i < totalRequests; i++ {
		go func() {
			defer wg.Done()
			isAllowed, err := sw.Allow(ctx, key, store, cfg)
			assertError(t, err)
			if isAllowed {
				atomic.AddInt32(&allowedCount, 1)
			}
		}()
	}

	wg.Wait()
	assertInvalidAmountRequestAccepeted(t, 30, int(allowedCount))

}

func testSlidingWindowTimer(t *testing.T, store ratelimiter.Store) {
	ctx := context.Background()
	clock := &FakeClock{currentTime: time.Now()}
	sw := NewSlidingWindow(clock)
	windowSize, reqPerWindow := 30, 100
	cfg := ratelimiter.Config{
		WindowSize:       float64(windowSize),
		RequestPerWindow: float64(reqPerWindow),
	}
	key := "test_window_timer"

	allowedCount := 0

	// fill the window
	for i := 0; i < reqPerWindow; i++ {
		isAllowed, err := sw.Allow(ctx, key, store, cfg)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}

	assertInvalidAmountRequestAccepeted(t, reqPerWindow, int(allowedCount))

	// move 10 seconds ahead
	clock.Add(time.Second * 10)

	// t = 10

	isAllowed, err := sw.Allow(ctx, key, store, cfg)
	assertError(t, err)
	if !isAllowed {
		t.Errorf("invalid request got accepeted")
	}

	// 30 second window resets
	// 35 second -> (5-35) 50 request - ACCEPTED
	// 40 second -> (10-40) 50 request - ACCEPTED
	// 65 second -> (35-65) - REQUEST REJECTED
	// 66 second -> (36-66) - 100 request -> 50 - ACCEPTED; 50 - REJECTED

	// t = 10, 35 - 10 = 25
	clock.Add(time.Second * 25)
	allowedCount = 0
	for i := 0; i < 50; i++ {
		isAllowed, err := sw.Allow(ctx, key, store, cfg)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}

	assertInvalidAmountRequestAccepeted(t, 50, allowedCount)

	// t = 35, 40 - 35 = 5
	clock.Add(time.Second * 5)
	allowedCount = 0
	for i := 0; i < 50; i++ {
		isAllowed, err := sw.Allow(ctx, key, store, cfg)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}

	assertInvalidAmountRequestAccepeted(t, 50, allowedCount)

	// t=40, 65-40 = 25
	clock.Add(time.Second * 25)
	isAllowed, err = sw.Allow(ctx, key, store, cfg)
	assertError(t, err)
	if !isAllowed {
		t.Errorf("invalid request got accepeted")
	}

	clock.Add(time.Second * 1)
	allowedCount = 0
	for i := 0; i < 100; i++ {
		isAllowed, err := sw.Allow(ctx, key, store, cfg)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}
	assertInvalidAmountRequestAccepeted(t, 50, allowedCount)
}

func assertError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertInvalidAmountRequestAccepeted(t testing.TB, want, got int) {
	t.Helper()
	if got != want {
		t.Errorf("expected %v req, but got %v req", want, got)
	}
}
