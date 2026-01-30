package main

import (
	"fmt"
	"net/http"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/Aryan123-rgb/rate-limiter/algorithms/tokenbucket"
	echoMiddleware "github.com/Aryan123-rgb/rate-limiter/middleware/echo"
	redisStore "github.com/Aryan123-rgb/rate-limiter/storage/redis"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

func CreateEchoServer() {
	// set up redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer redisClient.Close()

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

	// middleware
	mw := echoMiddleware.RateLimitMiddleware(tb, redisDb, cfg, func(c echo.Context) string {
		return c.RealIP()
	})

	app := echo.New()

	// apply to just one route, pass it as the middle argument:
	// app.GET("/admin", adminHandler, mw)

	// apply to group specific routues
	// api := app.Group("/api")
	// api.Use(mw)

	// Apply middleware globally
	app.Use(mw)

	app.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Sliding Window says OK!")
	})

	fmt.Println("Echo (Sliding Window) running on :4000")
	app.Logger.Fatal(app.Start(":4000"))
}
