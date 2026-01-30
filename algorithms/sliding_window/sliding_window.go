package slidingwindow

import (
	"context"
	"time"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
)

type SlidingWindow struct {
	clock ratelimiter.Clock
}

func NewSlidingWindow(clock ratelimiter.Clock) *SlidingWindow {
	return &SlidingWindow{
		clock: clock,
	}
}

func (sw *SlidingWindow) Allow(ctx context.Context, key string, store ratelimiter.Store, cfg ratelimiter.Config) (bool, error) {
	// acquire the lock
	unlock, err := store.AcquireLock(ctx, key)
	if err != nil {
		// too many go-routines are trying to access the same lock
		// result: request rejected (rate limited). No error
		return false, nil
	}
	defer unlock()

	now := sw.clock.Now()

	state, found, err := store.Get(ctx, key)
	if err != nil {
		return false, err
	}

	if !found {
		// first request from the user
		state = ratelimiter.State{
			WindowStart:   now,
			PreviousCount: 0,
			CurrentCount:  0,
		}
	}

	timeElapsed := now.Sub(state.WindowStart).Seconds()

	if timeElapsed >= cfg.WindowSize {
		missedWindows := (timeElapsed) / cfg.WindowSize
		if missedWindows >= 2 {
			// too much time passed, reset the states
			state = ratelimiter.State{
				PreviousCount: 0,
				CurrentCount:  0,
				WindowStart:   now,
			}
		} else {
			// slide the window
			state.PreviousCount = state.CurrentCount
			state.CurrentCount = 0
			state.WindowStart = state.WindowStart.Add(time.Second * time.Duration(cfg.WindowSize))
		}
		timeElapsed = now.Sub(state.WindowStart).Seconds()
	}

	// percentage of previous window required
	weight := (cfg.WindowSize - timeElapsed) / cfg.WindowSize

	// add the tokens from the previous count with the current count
	weightedCount := state.CurrentCount + (state.PreviousCount * weight)

	allowed := false
	if weightedCount < cfg.RequestPerWindow {
		state.CurrentCount++ 
		allowed = true 
	}

	err = store.Set(ctx, key, state)
	return allowed, err 
}
