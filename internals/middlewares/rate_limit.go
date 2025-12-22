package middlewares

import (
	"errors"
	"net/http"
	"time"

	"github.com/theabdullahishola/mzl-payment-app/internals/pkg"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

func (m *AuthMiddleware) RateLimitHandler(next http.Handler, redis *pkg.RedisQueue, limit int, window time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(UserIDKey)
		userID, ok := val.(string)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		limited, err := redis.IsRateLimited(r.Context(), userID, limit, window)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		if limited {
			utils.ErrorJSON(w, r, http.StatusTooManyRequests, errors.New("rate limit exceeded. please try again later"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
