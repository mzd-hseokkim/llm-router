package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// contextKey is a private type for context keys in this package.
type contextKey int

const virtualKeyCtxKey contextKey = 0

// base62Alphabet used for key encoding.
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// keyRandomLen is the number of random base62 characters after "sk-".
// Total key length = 3 + 43 = 46 chars.
const keyRandomLen = 43

// VirtualKey represents a gateway-issued API key with associated permissions.
type VirtualKey struct {
	ID            uuid.UUID
	KeyHash       string
	KeyPrefix     string // first 7 chars of the raw key (sk- + 4 random chars)
	Name          string
	UserID        *uuid.UUID
	TeamID        *uuid.UUID
	OrgID         *uuid.UUID
	ExpiresAt     *time.Time
	BudgetUSD     *float64
	RPMLimit      *int
	TPMLimit      *int
	AllowedModels []string
	BlockedModels []string
	Metadata      json.RawMessage
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastUsedAt    *time.Time
}

// Store defines the persistence interface for virtual keys.
type Store interface {
	Create(ctx context.Context, key *VirtualKey) error
	GetByHash(ctx context.Context, keyPrefix, keyHash string) (*VirtualKey, error)
	GetByID(ctx context.Context, id uuid.UUID) (*VirtualKey, error)
	List(ctx context.Context) ([]*VirtualKey, error)
	Update(ctx context.Context, key *VirtualKey) error
	Deactivate(ctx context.Context, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
	UpdateHash(ctx context.Context, id uuid.UUID, keyHash, keyPrefix string) error
}

// GenerateKey creates a new virtual key. Returns the raw key (shown once),
// its SHA-256 hex hash, and the key prefix used for DB lookup optimisation.
func GenerateKey() (rawKey, keyHash, keyPrefix string, err error) {
	b := make([]byte, keyRandomLen)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
	}

	chars := make([]byte, keyRandomLen)
	for i, v := range b {
		chars[i] = base62Alphabet[int(v)%62]
	}

	rawKey = "sk-" + string(chars)
	keyPrefix = rawKey[:7] // "sk-" + first 4 random chars
	keyHash = HashKey(rawKey)
	return rawKey, keyHash, keyPrefix, nil
}

// HashKey returns the SHA-256 hex hash of a raw key.
func HashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// VerifyHash compares a raw key against a stored hash using constant-time comparison.
func VerifyHash(rawKey, storedHash string) bool {
	computed := HashKey(rawKey)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// IsValid checks if the key is currently usable (active and not expired).
func (k *VirtualKey) IsValid() error {
	if !k.IsActive {
		return ErrKeyInactive
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return ErrKeyExpired
	}
	return nil
}

// CanAccessModel returns an error if the key is not permitted to use the given model.
func (k *VirtualKey) CanAccessModel(model string) error {
	if slices.Contains(k.BlockedModels, model) {
		return ErrModelBlocked
	}
	if len(k.AllowedModels) > 0 && !slices.Contains(k.AllowedModels, model) {
		return ErrModelNotAllowed
	}
	return nil
}

// SetVirtualKey stores a validated VirtualKey in the request context.
func SetVirtualKey(ctx context.Context, key *VirtualKey) context.Context {
	return context.WithValue(ctx, virtualKeyCtxKey, key)
}

// GetVirtualKey retrieves the VirtualKey from the request context.
// Returns nil if no key is present.
func GetVirtualKey(ctx context.Context) *VirtualKey {
	v, _ := ctx.Value(virtualKeyCtxKey).(*VirtualKey)
	return v
}
