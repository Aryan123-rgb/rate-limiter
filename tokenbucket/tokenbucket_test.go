package tokenbucket

import (
	"context"
	"fmt"
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

func TestTokenBucket_Backends(t *testing.T) {
	mr, err := miniredis.Run()
	assertError(t, err)
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
			t.Run("seq_burst_logic", func(t *testing.T) {
				testSequentialBurstRequests(t, tt.store())
			})

			t.Run("concurrent_burst_logic", func(t *testing.T) {
				testConcurrentBurstRequests(t, tt.store())
			})

			t.Run("refill_logic", func(t *testing.T) {
				testRefillLogic(t, tt.store())
			})
		})
	}
}

func testSequentialBurstRequests(t *testing.T, store ratelimiter.Store) {
	clock := &FakeClock{currentTime: time.Now()}
	ntb := NewTokenBucket(clock)
	capacity, rate := 200, 10
	cfg := ratelimiter.Config{
		Capacity: float64(capacity),
		Rate:     float64(rate),
	}
	ctx := context.Background()
	key := "test_seq"

	allowedCount := 0

	// drain all tokens
	// printStore(ctx, store, key)
	for i := 1; i <= capacity; i++ {
		isAllowed, err := ntb.Allow(ctx, key, store, cfg)
		// printStore(ctx, store, key)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}
	isAllowed, err := ntb.Allow(ctx, key, store, cfg)
	assertInvalidAmountRequestAccepeted(t, capacity, allowedCount)
	assertError(t, err)
	assertInvalidRequestAccepted(t, isAllowed)
	assertInvalidTokenValue(t, store, key, ctx, capacity)
}

func testConcurrentBurstRequests(t *testing.T, store ratelimiter.Store) {
	clock := &FakeClock{currentTime: time.Now()}
	capacity, rate := 100, 10
	cfg := ratelimiter.Config{
		Capacity: float64(capacity),
		Rate:     float64(rate),
	}
	ntb := NewTokenBucket(clock)
	ctx := context.Background()
	key := "test_concurrent"

	totalRequests := capacity * 10
	var allowedCount int32
	var wg sync.WaitGroup

	wg.Add(totalRequests)

	for i := 1; i <= totalRequests; i++ {
		go func() {
			defer wg.Done()
			isAllowed, err := ntb.Allow(ctx, key, store, cfg)
			assertError(t, err)
			if isAllowed {
				atomic.AddInt32(&allowedCount, 1)
			}
		}()
	}

	wg.Wait()
	assertInvalidTokenValue(t, store, key, ctx, capacity)
	assertInvalidAmountRequestAccepeted(t, capacity, int(allowedCount))
}

func testRefillLogic(t *testing.T, store ratelimiter.Store) {
	capacity, rate := 20, 5
	clock := &FakeClock{currentTime: time.Now()}
	cfg := ratelimiter.Config{
		Capacity: float64(capacity),
		Rate:     float64(rate),
	}
	ctx := context.Background()
	ntb := NewTokenBucket(clock)
	key := "test_refill"

	allowedCount := 0
	// drain all the tokens
	for i := 1; i <= capacity; i++ {
		isAllowed, err := ntb.Allow(ctx, key, store, cfg)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}
	assertInvalidTokenValue(t, store, key, ctx, capacity)
	assertInvalidAmountRequestAccepeted(t, capacity, allowedCount)

	// simulate 1 second time delay
	// bucket should now have rate tokens
	clock.Add(1 * time.Second)
	assertInvalidTokenValue(t, store, key, ctx, rate)
	allowedCount = 0
	for i := 1; i <= rate; i++ {
		isAllowed, err := ntb.Allow(ctx, key, store, cfg)
		assertError(t, err)
		if isAllowed {
			allowedCount++
		}
	}
	assertInvalidAmountRequestAccepeted(t, rate, allowedCount)

	// wait for a long time to check if the bucket gets filled
	clock.Add(time.Second * 100)
	assertInvalidTokenValue(t, store, key, ctx, capacity)
}

func assertInvalidTokenValue(t testing.TB, store ratelimiter.Store, key string, ctx context.Context, maxTokenValue int) {
	t.Helper()
	state, _, _ := store.Get(ctx, key)
	if state.Tokens < 0 || state.Tokens > float64(maxTokenValue) {
		t.Errorf("invalid token value: %v", state.Tokens)
	}
}

func assertInvalidAmountRequestAccepeted(t testing.TB, want, got int) {
	t.Helper()
	if want != got {
		t.Errorf("%v requests should have been accepted, but %v request got accepted", want, got)
	}
}

func assertError(t testing.TB, err error) {
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

func printStore(ctx context.Context, store ratelimiter.Store, key string) {
	state, found, _ := store.Get(ctx, key)
	if !found {
		fmt.Println("the value for the ", key, " is not present")
		return
	}
	fmt.Println("the number of tokens: ", state.Tokens)
	fmt.Println("the last refill time: ", state.LastRequestTime.Format("2006-01-02 15:04:05"))
}
