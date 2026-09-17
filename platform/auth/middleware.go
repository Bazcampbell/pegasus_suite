// auth/middleware.go

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ctxKey int

const claimsKey ctxKey = 0

// ClaimsFromContext returns nil when the request did not pass RequireAuth. The
// Claims methods are nil-safe, so callers can test a role without a nil check
// first.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// RequireAuth verifies a SESSION-issued bearer token and puts the claims on the
// request context. A nil verifier answers 503 rather than allowing the request
// through: an unconfigured JWK_URL must not read as "auth disabled".
func RequireAuth(v *Verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v == nil {
			writeError(w, http.StatusServiceUnavailable, "auth not configured")
			return
		}
		ah := r.Header.Get("Authorization")
		if !strings.HasPrefix(ah, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := v.Parse(strings.TrimPrefix(ah, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
	})
}

// RequireRole wraps RequireAuth, so a role-gated route does not need both.
func RequireRole(v *Verifier, role string, next http.Handler) http.Handler {
	return RequireAuth(v, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ClaimsFromContext(r.Context()).HasRole(role) {
			writeError(w, http.StatusForbidden, "role required: "+role)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
