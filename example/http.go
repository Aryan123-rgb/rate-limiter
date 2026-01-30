package main

import (
	"fmt"
	"net/http"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/Aryan123-rgb/rate-limiter/algorithms/tokenbucket"
	httpMiddleware "github.com/Aryan123-rgb/rate-limiter/middleware/http"
	redisStore "github.com/Aryan123-rgb/rate-limiter/storage/redis"
	"github.com/redis/go-redis/v9"
)

func CreateHTTPServer() {
	// set up redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// storage use either redisStore for using redis db
	// or memory for using in-memory storage
	redisDb := redisStore.NewRedisStore(redisClient)
	// inMemoryDb := memory.NewInMemoryStore()

	// define configurations
	cfg := ratelimiter.Config{
		Rate:             10,  // 10 req/second
		Capacity:         100, // Burst upto 100
		WindowSize:       30,  // 30 second window size
		RequestPerWindow: 100, // request allowed per window
	}

	// define the algorithms
	tb := tokenbucket.NewTokenBucket(ratelimiter.RealClock{})
	// sw := slidingwindow.NewSlidingWindow(ratelimiter.RealClock{})

	middleware := httpMiddleware.RateLimit(tb, redisDb, cfg, func(r *http.Request) string {
		return r.RemoteAddr
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("success"))
	})

	app := http.NewServeMux()

	// route specific middleware
	app.Handle("/api", middleware(handler))

	// To protect the ENTIRE server, wrap the mux itself:
	// http.ListenAndServe(":8080", middleware(app))


	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", app)
}
