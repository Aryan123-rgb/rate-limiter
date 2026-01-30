package memory

import (
	"context"
	"sync"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
)

type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string]ratelimiter.State
	// To implement distributed locking, we need to manage the locks per user
	// when acquirelock("user_1") is called we create a mutex for key "user_1" and lock
	// we will return a function that will unlock it, and during that time other goroutine cannot lock it
	locks      map[string]*sync.Mutex
	locksMapMu sync.Mutex
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		data:  make(map[string]ratelimiter.State),
		locks: make(map[string]*sync.Mutex),
	}
}

func (s *InMemoryStore) Get(ctx context.Context, key string) (ratelimiter.State, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.data[key]
	return val, ok, nil
}

func (s *InMemoryStore) Set(ctx context.Context, key string, state ratelimiter.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = state
	return nil
}

func (s *InMemoryStore) AcquireLock(ctx context.Context, key string) (func(), error) {
	s.locksMapMu.Lock()
	
	mu, exists := s.locks[key]
	if !exists {
		mu = &sync.Mutex{}
		s.locks[key] = mu
	}
	s.locksMapMu.Unlock()

	mu.Lock()

	return func() {
		mu.Unlock()
	}, nil
}
