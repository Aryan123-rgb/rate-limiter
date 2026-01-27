package tokenbucket

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/Aryan123-rgb/rate-limiter/memory"
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

func TestInitialBurstRequestsLocally(t *testing.T) {
	capacity, rate := 200, 10
	cfg := ratelimiter.Config{
		Capacity: float64(capacity),
		Rate:     float64(rate),
	}
	ctx := context.Background()

	t.Run("sequential_requests_should_not_exceed_capacity", func(t *testing.T) {
		clock := FakeClock{currentTime: time.Now()}
		store := memory.NewInMemoryStore()
		ntb := NewTokenBucket(&clock)

		key := "test_seq"
		allowedCount := 0
		// exhaust all the tokens initially
		for i := 1; i <= capacity; i++ {
			isAllowed, err := ntb.Allow(ctx, key, store, cfg)
			assertErrAccessingLocalMemory(t, err)

			if isAllowed {
				allowedCount++
			}
		}
		assertInvalidAmountRequestAccepeted(t, capacity, allowedCount)
		
		// all tokens are drained
		isAllowed, err := ntb.Allow(ctx, key, store, cfg)

		assertErrAccessingLocalMemory(t, err)
		assertInvalidRequestAccepted(t, isAllowed)
		assertInvalidTokenValue(t, store, key, ctx, capacity)
	})

	t.Run("concurrent_requests_should_not_exceed_capacity", func(t *testing.T) {
		store := memory.NewInMemoryStore()
		clock := FakeClock{currentTime: time.Now()}
		ntb := NewTokenBucket(&clock)
		key := "test_parallel"
		totalRequests := capacity * 10
		var allowedCount int32 // atomic counter

		var wg sync.WaitGroup
		wg.Add(totalRequests)

		for i := 1; i <= totalRequests; i++ {
			go func() {
				defer wg.Done()
				allowed, err := ntb.Allow(context.Background(), key, store, cfg)
				assertErrAccessingLocalMemory(t, err)
				if allowed {
					atomic.AddInt32(&allowedCount, 1)
				}
			}()
		}

		go func() {
			wg.Wait()
		}()

		assertInvalidAmountRequestAccepeted(t, capacity, int(allowedCount))
	})
}

func TestRefillLogic(t *testing.T) {
	capacity, rate := 20, 5
	cfg := ratelimiter.Config{
		Capacity: float64(capacity),
		Rate:     float64(rate),
	}

	t.Run("tokenbucket_should_refill_at_rate_per_second", func(t *testing.T) {
		clock := FakeClock{currentTime: time.Now()}
		ntb := NewTokenBucket(&clock)
		ctx := context.Background()
		store := memory.NewInMemoryStore()
		key := "test_1s_delay"
		allowedCount := 0
		// drain all the current tokens
		for i := 1; i <= capacity; i++ {
			isAllowed, err := ntb.Allow(ctx, key, store, cfg)
			assertErrAccessingLocalMemory(t, err)
			if isAllowed {
				allowedCount++
			}
		}
		assertInvalidAmountRequestAccepeted(t, capacity, allowedCount)

		// simulate a 1s delay
		clock.Add(time.Second)

		// rate amount of requests should get accepted
		allowedCount = 0
		for i := 1; i <= rate; i++ {
			isAllowed, err := ntb.Allow(ctx, key, store, cfg)
			assertErrAccessingLocalMemory(t, err)
			if isAllowed {
				allowedCount++
			}
		}
		assertInvalidAmountRequestAccepeted(t, rate, allowedCount)
	})

	t.Run("tokenbucket_should_be_refilled_completely_after_certain_time", func(t *testing.T) {
		clock := FakeClock{currentTime: time.Now()}
		ntb := NewTokenBucket(&clock)
		ctx := context.Background()
		store := memory.NewInMemoryStore()
		key := "test_complete_refill"

		// drain all the tokens
		for i := 1; i <= capacity; i++ {
			_, err := ntb.Allow(ctx, key, store, cfg)
			assertErrAccessingLocalMemory(t, err)
		}

		// wait for the bucket to refilled completely
		timeToRefill := time.Duration(capacity / rate)
		timeToRefill = timeToRefill * 10
		clock.Add(timeToRefill * time.Second)

		// should be able to process capacity requests only
		allowedCount := 0
		for i := 1; i <= capacity+10; i++ {
			isAllowed, err := ntb.Allow(ctx, key, store, cfg)
			assertErrAccessingLocalMemory(t, err)
			if isAllowed {
				allowedCount++
			}
		}

		assertInvalidAmountRequestAccepeted(t, capacity, allowedCount)
	})
}

func assertInvalidTokenValue(t testing.TB, store ratelimiter.Store, key string, ctx context.Context, capacity int) {
	t.Helper()
	state, _, _ := store.Get(ctx, key)
	if state.Tokens < 0 || state.Tokens > float64(capacity) {
		t.Errorf("invalid token value: %v", state.Tokens)
	}
}

func assertInvalidAmountRequestAccepeted(t testing.TB, want, got int) {
	t.Helper()
	if want != got {
		t.Errorf("%v requests should have been accepted, but %v request got accepted", want, got)
	}
}

func assertErrAccessingLocalMemory(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error occured: %v", err)
	}
}

func assertInvalidRequestAccepted(t testing.TB, isAllowed bool) {
	t.Helper()
	if isAllowed {
		t.Errorf("request should have been rejected but got accepted")
	}
}