package main

import (
	"fmt"
	"net/http"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/Aryan123-rgb/rate-limiter/memory"
	"github.com/Aryan123-rgb/rate-limiter/middleware/http"
	"github.com/Aryan123-rgb/rate-limiter/tokenbucket"
)

func main() {
	// 1. Setup Dependancies
	// storage: in memory store
	memStore := memory.NewInMemoryStore()

	// algorithm: token bucket
	algorithm := tokenbucket.NewTokenBucket(ratelimiter.RealClock{})

	// config: 5 request/second, burst of 10
	config := ratelimiter.Config{
		Rate: 5,
		Capacity: 10,
	}

	// 2. Define how to identify user
	// using ip address
	keyExtractor := func(r *http.Request) string {
		return r.RemoteAddr
	}

	// 3. create the rate limiter middleware
	limiterMiddleWare := middleware.RateLimit(algorithm, memStore, config, keyExtractor)

	// implement a simple server with a simple handler
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Success!"))
	})

	mux.Handle("/", limiterMiddleWare(handler))

	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", mux)
}