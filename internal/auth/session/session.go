// Package session manages user web sessions backed by Redis.
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultTTL    = 24 * time.Hour
	RenewThreshold = time.Hour // renew if less than this remains
)

// Session holds authenticated session data.
type Session struct {
	ID        string    `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Provider  string    `json:"provider"`  // which OAuth provider
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired returns true if the session has passed its expiry time.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// NeedsRenewal returns true if the session should be extended on this request.
func (s *Session) NeedsRenewal() bool {
	return time.Until(s.ExpiresAt) < RenewThreshold
}

// Store manages sessions in Redis.
type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewStore creates a session store with the given TTL.
func NewStore(rdb *redis.Client, ttl time.Duration) *Store {
	if ttl == 0 {
		ttl = DefaultTTL
	}
	return &Store{rdb: rdb, ttl: ttl}
}

// Create generates a new session, stores it in Redis, and returns it.
func (s *Store) Create(ctx context.Context, userID uuid.UUID, provider string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	now := time.Now().UTC()
	sess := &Session{
		ID:        id,
		UserID:    userID,
		Provider:  provider,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("marshal session: %w", err)
	}

	if err := s.rdb.Set(ctx, redisKey(id), data, s.ttl).Err(); err != nil {
		return nil, fmt.Errorf("store session: %w", err)
	}

	return sess, nil
}

// Get retrieves a session by ID. Returns nil, nil if not found.
func (s *Store) Get(ctx context.Context, id string) (*Session, error) {
	data, err := s.rdb.Get(ctx, redisKey(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &sess, nil
}

// Renew extends the session TTL if it is close to expiry.
// Returns the updated session so callers can refresh the browser cookie.
func (s *Store) Renew(ctx context.Context, id string) (*Session, error) {
	sess, err := s.Get(ctx, id)
	if err != nil || sess == nil {
		return nil, err
	}
	if !sess.NeedsRenewal() {
		return sess, nil
	}
	sess.ExpiresAt = time.Now().UTC().Add(s.ttl)
	data, _ := json.Marshal(sess)
	if err := s.rdb.Set(ctx, redisKey(id), data, s.ttl).Err(); err != nil {
		return nil, err
	}
	return sess, nil
}

// Delete removes a session (logout).
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, redisKey(id)).Err()
}

// StoreState saves an OAuth state token in Redis (10-minute TTL).
func (s *Store) StoreState(ctx context.Context, state, provider string) error {
	return s.rdb.Set(ctx, stateKey(state), provider, 10*time.Minute).Err()
}

// VerifyState validates and removes an OAuth state token.
// Returns the provider name if valid.
func (s *Store) VerifyState(ctx context.Context, state string) (string, error) {
	provider, err := s.rdb.GetDel(ctx, stateKey(state)).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired oauth state")
	}
	if err != nil {
		return "", fmt.Errorf("verify state: %w", err)
	}
	return provider, nil
}

func redisKey(id string) string   { return "session:" + id }
func stateKey(state string) string { return "oauth:state:" + state }

func generateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
