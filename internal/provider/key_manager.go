package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/crypto"
)

// KeyProvider is the interface adapters use to obtain a provider API key
// at request time (supporting dynamic DB-backed key rotation).
type KeyProvider interface {
	SelectKey(ctx context.Context, providerName, group string) (string, error)
}

// ProviderKeyRecord is the raw DB record with the encrypted key.
type ProviderKeyRecord struct {
	ID                 uuid.UUID
	Provider           string
	KeyAlias           string
	EncryptedKey       []byte
	KeyPreview         string
	GroupName          string
	Tags               []string
	Weight             int
	IsActive           bool
	MonthlyBudgetUSD   *float64
	CurrentMonthSpend  float64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastUsedAt         *time.Time
	UseCount           int64
}

// ProviderKeyStore defines the persistence interface for provider keys.
type ProviderKeyStore interface {
	Create(ctx context.Context, rec *ProviderKeyRecord) error
	GetByID(ctx context.Context, id uuid.UUID) (*ProviderKeyRecord, error)
	List(ctx context.Context, providerFilter string) ([]*ProviderKeyRecord, error)
	Update(ctx context.Context, rec *ProviderKeyRecord) error
	Delete(ctx context.Context, id uuid.UUID) error
	RotateKey(ctx context.Context, id uuid.UUID, encryptedKey []byte, preview string) error
	ListActive(ctx context.Context, providerName, groupName string) ([]*ProviderKeyRecord, error)
}

// providerKey holds a decrypted key in memory (never logged).
type providerKey struct {
	id           uuid.UUID
	keyAlias     string
	decryptedKey string // never logged
	groupName    string
	weight       int
}

// KeyManager resolves provider API keys from the DB (encrypted) or falls back
// to config-file static keys. Keys are cached in memory for cacheTTL.
type KeyManager struct {
	store    ProviderKeyStore // nil → skip DB lookup
	cipher   *crypto.Cipher  // nil → skip DB lookup (can't decrypt)
	fallback map[string]string // provider name → static key from config

	mu        sync.RWMutex
	cache     map[string][]*providerKey // cache key → keys
	cacheTime map[string]time.Time
	cacheTTL  time.Duration
}

// NewKeyManager creates a KeyManager.
// store and cipher may be nil; in that case only fallback config keys are used.
// fallback maps provider name → static API key from config.
func NewKeyManager(store ProviderKeyStore, cipher *crypto.Cipher, fallback map[string]string) *KeyManager {
	return &KeyManager{
		store:     store,
		cipher:    cipher,
		fallback:  fallback,
		cache:     make(map[string][]*providerKey),
		cacheTime: make(map[string]time.Time),
		cacheTTL:  5 * time.Minute,
	}
}

// SelectKey returns the decrypted API key for the given provider and group.
// DB keys take priority over config-file fallback.
func (m *KeyManager) SelectKey(ctx context.Context, providerName, group string) (string, error) {
	if m.store != nil && m.cipher != nil {
		keys, err := m.getFromCache(ctx, providerName, group)
		if err != nil {
			return "", err
		}
		if len(keys) > 0 {
			selected, err := weightedRandom(keys)
			if err != nil {
				return "", err
			}
			return selected.decryptedKey, nil
		}
	}

	// Fall back to config-file key.
	if key, ok := m.fallback[providerName]; ok && key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no active API key for provider %q", providerName)
}

// InvalidateCache clears the cached keys for a provider so the next request
// reloads from DB. Call this after any DB mutation.
func (m *KeyManager) InvalidateCache(providerName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Clear all cache entries for this provider (any group).
	for k := range m.cache {
		// cache key format: "provider/group"
		if len(k) >= len(providerName) && k[:len(providerName)] == providerName {
			delete(m.cache, k)
			delete(m.cacheTime, k)
		}
	}
}

func (m *KeyManager) getFromCache(ctx context.Context, providerName, group string) ([]*providerKey, error) {
	cacheKey := providerName + "/" + group

	m.mu.RLock()
	keys, hit := m.cache[cacheKey]
	fresh := hit && time.Since(m.cacheTime[cacheKey]) < m.cacheTTL
	m.mu.RUnlock()

	if fresh {
		return keys, nil
	}

	return m.reloadCache(ctx, providerName, group, cacheKey)
}

func (m *KeyManager) reloadCache(ctx context.Context, providerName, group, cacheKey string) ([]*providerKey, error) {
	records, err := m.store.ListActive(ctx, providerName, group)
	if err != nil {
		return nil, fmt.Errorf("load active provider keys for %q: %w", providerName, err)
	}

	keys := make([]*providerKey, 0, len(records))
	for _, rec := range records {
		decrypted, err := m.cipher.Decrypt(rec.EncryptedKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt key %q: %w", rec.KeyAlias, err)
		}
		keys = append(keys, &providerKey{
			id:           rec.ID,
			keyAlias:     rec.KeyAlias,
			decryptedKey: string(decrypted),
			groupName:    rec.GroupName,
			weight:       rec.Weight,
		})
	}

	m.mu.Lock()
	m.cache[cacheKey] = keys
	m.cacheTime[cacheKey] = time.Now()
	m.mu.Unlock()

	return keys, nil
}
