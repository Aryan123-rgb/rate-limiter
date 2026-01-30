package fiberMiddleware

import (
	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/gofiber/fiber/v2"
)

// function for extracting unique ID from the fiber context
type KeyFunc func(c *fiber.Ctx) string

func RateLimitMiddleware(
	strategy ratelimiter.Strategy,
	store ratelimiter.Store,
	cfg ratelimiter.Config,
	keyFn KeyFunc,
) fiber.Handler {

	return func (c *fiber.Ctx) error {
		// 1. extract the key from the fiber context
		key := keyFn(c)

		// 2. run the ratelimiter
		allowed, err := strategy.Allow(c.Context(), key, store, cfg)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error during rate limiting",
			})
		}

		if !allowed {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Too many requests",
			})
		}

		return c.Next()
	}
}

