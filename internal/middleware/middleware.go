package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eventpark/api/internal/auth"
	"github.com/google/uuid"
)

type contextKey string

const UserKey contextKey = "user"

type AuthUser struct {
	ID    uuid.UUID
	Phone string
	Role  string
}

func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				writeErr(w, http.StatusUnauthorized, "missing or invalid authorization header")
				return
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := auth.ParseAccessToken(tokenStr, jwtSecret)
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserKey, &AuthUser{
				ID:    claims.UserID,
				Phone: claims.Phone,
				Role:  claims.Role,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUser(r *http.Request) *AuthUser {
	u, _ := r.Context().Value(UserKey).(*AuthUser)
	return u
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
