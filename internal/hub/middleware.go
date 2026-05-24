package hub

import (
	"context"
	"net/http"
	"strings"
)

type agentContextKey int

const (
	ctxKeyTokenID agentContextKey = iota
	ctxKeyAgentID
)

// TokenIDFromContext returns the agent token ID attached by AgentAuthMiddleware.
func TokenIDFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(ctxKeyTokenID).(int64)
	return v, ok
}

// AgentAuthMiddleware validates Bearer tokens for agent endpoints and attaches the token ID to the request context.
func AgentAuthMiddleware(store TokenStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		raw := auth[7:]
		if !strings.HasPrefix(raw, tokenPrefix) {
			http.Error(w, `{"error":"invalid token format"}`, http.StatusUnauthorized)
			return
		}

		t, err := ValidateToken(store, raw)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		_ = store.UpdateAgentTokenLastUsed(t.ID)

		ctx := context.WithValue(r.Context(), ctxKeyTokenID, t.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
