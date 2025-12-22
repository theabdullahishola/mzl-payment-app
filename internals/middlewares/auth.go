package middlewares

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/theabdullahishola/mzl-payment-app/internals/config"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

type AuthMiddleware struct {
	Config *config.Config
}

func NewAuthMiddleware(cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{Config: cfg}
}
type ContextKey string
const UserIDKey ContextKey = "user_id"
func (m *AuthMiddleware) MiddlewareAuthHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("missing authorization header"))
			return
		}

		// Header format: "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("invalid header format"))
			return
		}
		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(m.Config.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("invalid or expired token"))
			return
		}

		//Lets get the userID
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("invalid token claims"))
			return
		}

		userID, ok := claims["sub"].(string)
		if !ok {
			utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("invalid user ID in token"))
			return
		}


		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
