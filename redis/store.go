package redisStore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// Get retrieves the state from redis
func (r *RedisStore) Get(ctx context.Context, key string) (ratelimiter.State, bool, error) {
	// key -> user_123; redisKey -> limiter:user_123
	redisKey := fmt.Sprintf("limiter:%s", key)

	val, err := r.client.Get(ctx, redisKey).Result()
	if errors.Is(err, redis.Nil) {
		return ratelimiter.State{}, false, nil // not found
	}

	if err != nil {
		return ratelimiter.State{}, false, err
	}

	// fmt.Printf("RAW REDIS VALUE (%q): %q\n", redisKey, val)

	var state ratelimiter.State
	err = json.Unmarshal([]byte(val), &state)
	if err != nil {
		return ratelimiter.State{}, false, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	return state, true, nil
}

// Set saves the state to redis
func (r *RedisStore) Set(ctx context.Context, key string, state ratelimiter.State) error {
	redisKey := fmt.Sprintf("limiter:%s", key)

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	err = r.client.Set(ctx, redisKey, data, time.Hour).Err()
	return err
}

// AcquireLock implements a distrbuted spin lock
func (r *RedisStore) AcquireLock(ctx context.Context, key string) (func(), error) {
	lockKey := fmt.Sprintf("lock:%s", key)
	// Unique ID for this specific lock instance (to safely unlock later)
	lockValue := uuid.New().String()

	// How long to wait before giving up (Timeout)
	timeout := time.After(30 * time.Second)
	// How long the lock lasts in Redis if we crash (Safety)
	ttl := 10 * time.Second

	// SPIN LOOP
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("the goroutine timeout before acquiring the lock %v", timeout)
		case <-ticker.C:
			// Try to set the lock: SET key value NX PX ttl
			success, err := r.client.SetNX(ctx, lockKey, lockValue, ttl).Result()
			if err != nil {
				return nil, err
			}
			if success {
				// lock acquired return the cleanup function
				return func() {
					// LUA SCRIPT: Only delete the lock if the value matches OUR value.
					// This prevents us from deleting someone else's lock if ours expired.
					const unlockScript = `
						if redis.call("get", KEYS[1]) == ARGV[1] then
							return redis.call("del", KEYS[1])
						else
							return 0
						end
					`
					r.client.Eval(context.Background(), unlockScript, []string{lockKey}, lockValue)
				}, nil
			}
		}
	}
}
