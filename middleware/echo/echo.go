package echoMiddleware

import (
	"net/http"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
	"github.com/labstack/echo/v4"
)

type KeyFunc func(c echo.Context) string

func RateLimitMiddleware(
	strategy ratelimiter.Strategy,
	store ratelimiter.Store,
	cfg ratelimiter.Config,
	keyFn KeyFunc,
) echo.MiddlewareFunc {

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1. extract the key
			key := keyFn(c)

			// 2. run the middleware
			allowed, err := strategy.Allow(c.Request().Context(), key, store, cfg)

			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error during rate limiting")
			}

			if !allowed {
				return echo.NewHTTPError(http.StatusTooManyRequests, "Too many requests")
			}

			// 3. proceed
			return next(c)
		}
	}
}
