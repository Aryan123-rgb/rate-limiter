package httpMiddleware

import (
	"net/http"

	ratelimiter "github.com/Aryan123-rgb/rate-limiter"
)

type KeyFunc func(r *http.Request) string

func RateLimit(
	strategy ratelimiter.Strategy,
	store ratelimiter.Store,
	cfg ratelimiter.Config,
	keyFn KeyFunc,
) func(next http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1 : Extract the key (IP, userId, etc...)
			key := keyFn(r)

			// 2. Ask the strategy if this is allowed
			// We use r.Context() so if the client cancels the request,
			// the store/redis operation can also be cancelled.
			allowed, err := strategy.Allow(r.Context(), key, store, cfg)

			if err != nil {
				// Internal Server Error (e.g., Redis down)
				// You might choose to fail open (allow) or closed (block) here.
				// here we fail open
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				return
			}

			if !allowed {
				// 3. Block the request
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			// 4. Allow: Pass control to the next handler
			next.ServeHTTP(w, r)
		})
	}
}
