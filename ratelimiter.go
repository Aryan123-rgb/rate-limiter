package ratelimiter

import (
	"context"
	"time"
)

// created for testing purposes
type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (c RealClock) Now() time.Time {
	return time.Now()
}

type Config struct {
	// Token Bucket Fields
	Rate     float64 // how many tokens are added per second
	Capacity float64 // maximum burst allowed

	// Sliding Window Fields
	WindowSize       float64 // In seconds
	RequestPerWindow float64
}

type State struct {
	// Token Bucket Fields
	Tokens          float64
	LastRequestTime time.Time

	// Sliding Window Fields
	PreviousCount float64
	CurrentCount  float64
	WindowStart   time.Time
}

type Store interface {
	// Get retrieves the current state for the specific key
	Get(ctx context.Context, key string) (state State, found bool, err error)

	// Set updates the state for a specific key
	Set(ctx context.Context, key string, state State) error

	// AcquireLock locks the specific key.
	// It returns an unlock function that must be called to release the lock
	AcquireLock(ctx context.Context, key string) (unlock func(), err error)
}

type Strategy interface {
	Allow(ctx context.Context, key string, store Store, cfg Config) (bool, error)
}
