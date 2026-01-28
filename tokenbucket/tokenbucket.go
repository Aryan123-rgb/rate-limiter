package tokenbucket

import (
	"context"
	"fmt"
	"math"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
)

type TokenBucket struct{
	clock ratelimiter.Clock
}

func NewTokenBucket(clock ratelimiter.Clock) *TokenBucket {
	return &TokenBucket{
		clock: clock,
	}
}

func (tb *TokenBucket) Allow(ctx context.Context, key string, store ratelimiter.Store, cfg ratelimiter.Config) (bool, error) {
	// acquire the lock 
	unlock, err := store.AcquireLock(ctx, key)
	if err != nil {
		return false, fmt.Errorf("error acquiring the lock : %v", err) 
	}
	defer unlock()

	now := tb.clock.Now()

	// Step 1: retrieve the current token count from the store
	state, found, err := store.Get(ctx, key)
	if err != nil {
		return false, fmt.Errorf("err retrieveing the state, state: %v\n, error: %v", state, err)
	}

	// if the user is new, initialise the bucket
	if !found {
		state = ratelimiter.State{
			Tokens:         cfg.Capacity - 1, // initialise with maximum capacity to allow burst request
			LastRequestTime: now,
		}
		store.Set(ctx, key, state)
		return true, nil
	}

	// Step 2: refill the bucket based on the time elapsed
	elapsed := now.Sub(state.LastRequestTime).Seconds()
	tokensToAdd := elapsed * cfg.Rate
	state.Tokens = math.Min(state.Tokens + tokensToAdd, cfg.Capacity)
	state.LastRequestTime = now

	// Step 3: check if the tokens are enough
	allowed := false
	if state.Tokens >= 1.0 {
		allowed = true
		state.Tokens -= 1.0
	}

	err = store.Set(ctx, key, state)
	if err != nil {
		// if storage is down, we fail closed
		return false, fmt.Errorf("error occured while trying to update the state: %v", err)
	}
	return allowed, nil 
}
