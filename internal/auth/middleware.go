package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/telemetry"
)

// VirtualKeyMiddleware validates incoming Bearer tokens against virtual keys.
type VirtualKeyMiddleware struct {
	store  Store
	cache  Cache
	logger *slog.Logger
	// lastUsedCh is a fire-and-forget channel for async last_used_at updates.
	lastUsedCh chan lastUsedUpdate
}

type lastUsedUpdate struct {
	id uuid.UUID
}

// NewVirtualKeyMiddleware creates the middleware and starts the async updater.
func NewVirtualKeyMiddleware(store Store, cache Cache, logger *slog.Logger) *VirtualKeyMiddleware {
	m := &VirtualKeyMiddleware{
		store:      store,
		cache:      cache,
		logger:     logger,
		lastUsedCh: make(chan lastUsedUpdate, 512),
	}
	go m.runLastUsedUpdater()
	return m
}

// Middleware returns an http.Handler middleware that enforces virtual key auth.
func (m *VirtualKeyMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawKey := bearerToken(r)
		if rawKey == "" {
			writeAuthError(w, "missing or malformed Authorization header")
			return
		}

		keyHash := HashKey(rawKey)

		// 1. Try Redis cache first.
		key, err := m.cache.Get(r.Context(), keyHash)
		if err != nil {
			m.logger.Warn("auth cache get failed", "error", err)
		}

		// 2. Cache miss — look up in DB.
		if key == nil {
			prefix := rawKey
			if len(prefix) >= 7 {
				prefix = prefix[:7]
			}
			key, err = m.store.GetByHash(r.Context(), prefix, keyHash)
			if err != nil {
				if errors.Is(err, ErrKeyNotFound) {
					writeAuthError(w, "invalid API key")
					return
				}
				m.logger.Error("virtual key DB lookup failed", "error", err)
				writeAuthError(w, "authentication error")
				return
			}

			// Populate cache for subsequent requests.
			if cacheErr := m.cache.Set(r.Context(), keyHash, key); cacheErr != nil {
				m.logger.Warn("auth cache set failed", "error", cacheErr)
			}
		}

		// 3. Validate key state.
		if err := key.IsValid(); err != nil {
			switch {
			case errors.Is(err, ErrKeyExpired):
				writeAuthError(w, "API key has expired")
			default:
				writeAuthError(w, "API key is inactive")
			}
			return
		}

		// 4. Async last_used_at update (non-blocking, deduplicated in updater).
		select {
		case m.lastUsedCh <- lastUsedUpdate{id: key.ID}:
		default: // drop if channel is full — acceptable
		}

		// 5. Record key ownership in the log context (if present).
		telemetry.SetVirtualKeyInfo(r.Context(), &key.ID, key.UserID, key.TeamID, key.OrgID)

		next.ServeHTTP(w, r.WithContext(SetVirtualKey(r.Context(), key)))
	})
}

// runLastUsedUpdater drains the channel and updates last_used_at in the DB.
// Deduplicates per key: each key is updated at most once per minute.
func (m *VirtualKeyMiddleware) runLastUsedUpdater() {
	// seen tracks the last time each key was written to the DB.
	seen := make(map[uuid.UUID]time.Time)

	for update := range m.lastUsedCh {
		// Skip if this key was already updated within the last minute.
		if last, ok := seen[update.id]; ok && time.Since(last) < time.Minute {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := m.store.UpdateLastUsed(ctx, update.id); err != nil {
			m.logger.Warn("last_used_at update failed", "error", err, "key_id", update.id)
		}
		cancel()

		seen[update.id] = time.Now()

		// Evict stale entries to prevent unbounded map growth.
		if len(seen) > 5000 {
			cutoff := time.Now().Add(-2 * time.Minute)
			for k, v := range seen {
				if v.Before(cutoff) {
					delete(seen, k)
				}
			}
		}
	}
}

// adminClaimsKey is the context key for AdminClaims.
type adminClaimsKey struct{}

// AdminAuth returns middleware that validates admin JWTs.
func AdminAuth(jwtSvc *JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			claims, err := jwtSvc.ValidateToken(token)
			if err != nil {
				writeAuthError(w, "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), adminClaimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAdminClaims retrieves admin JWT claims from the request context.
func GetAdminClaims(ctx context.Context) *AdminClaims {
	v, _ := ctx.Value(adminClaimsKey{}).(*AdminClaims)
	return v
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

func writeAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorDetail{
			Message: msg,
			Type:    "invalid_request_error",
			Code:    "invalid_api_key",
		},
	})
}
