package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

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
	keyHash string
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

		// 4. Async last_used_at update (non-blocking).
		select {
		case m.lastUsedCh <- lastUsedUpdate{keyHash: keyHash}:
		default: // drop if channel is full — acceptable
		}

		// 5. Record key ownership in the log context (if present).
		telemetry.SetVirtualKeyInfo(r.Context(), &key.ID, key.UserID, key.TeamID, key.OrgID)

		next.ServeHTTP(w, r.WithContext(SetVirtualKey(r.Context(), key)))
	})
}

// runLastUsedUpdater drains the channel and updates last_used_at in bulk.
// Runs as a background goroutine.
func (m *VirtualKeyMiddleware) runLastUsedUpdater() {
	for update := range m.lastUsedCh {
		_ = update // TODO phase 1-07: batch DB writes
	}
}

// AdminAuth returns middleware that requires the static master key.
func AdminAuth(masterKey string) func(http.Handler) http.Handler {
	masterKeyBytes := []byte(masterKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" || subtle.ConstantTimeCompare([]byte(token), masterKeyBytes) != 1 {
				writeAuthError(w, "invalid master key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
