package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/crypto"
	"github.com/llm-router/gateway/internal/provider"
)

// --- fake store ---

type fakeProviderKeyStore struct {
	records []*provider.ProviderKeyRecord
}

func (f *fakeProviderKeyStore) Create(_ context.Context, rec *provider.ProviderKeyRecord) error {
	rec.ID = uuid.New()
	rec.CreatedAt = time.Now()
	rec.UpdatedAt = time.Now()
	f.records = append(f.records, rec)
	return nil
}

func (f *fakeProviderKeyStore) GetByID(_ context.Context, id uuid.UUID) (*provider.ProviderKeyRecord, error) {
	for _, r := range f.records {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeProviderKeyStore) List(_ context.Context, _ string) ([]*provider.ProviderKeyRecord, error) {
	return f.records, nil
}

func (f *fakeProviderKeyStore) Update(_ context.Context, _ *provider.ProviderKeyRecord) error {
	return nil
}

func (f *fakeProviderKeyStore) Delete(_ context.Context, id uuid.UUID) error {
	for i, r := range f.records {
		if r.ID == id {
			f.records = append(f.records[:i], f.records[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (f *fakeProviderKeyStore) RotateKey(_ context.Context, _ uuid.UUID, _ []byte, _ string) error {
	return nil
}

func (f *fakeProviderKeyStore) ListActive(_ context.Context, providerName, _ string) ([]*provider.ProviderKeyRecord, error) {
	var out []*provider.ProviderKeyRecord
	for _, r := range f.records {
		if r.Provider == providerName && r.IsActive {
			out = append(out, r)
		}
	}
	return out, nil
}

// --- tests ---

func newTestCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	// 32 bytes of fixed test key
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestKeyManager_FallbackWhenNoDBKeys(t *testing.T) {
	store := &fakeProviderKeyStore{}
	cipher := newTestCipher(t)
	fallback := map[string]string{"openai": "sk-config-key"}

	km := provider.NewKeyManager(store, cipher, fallback)

	key, err := km.SelectKey(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("SelectKey: %v", err)
	}
	if key != "sk-config-key" {
		t.Errorf("expected fallback key, got %q", key)
	}
}

func TestKeyManager_DBKeyTakesPriority(t *testing.T) {
	store := &fakeProviderKeyStore{}
	cipher := newTestCipher(t)

	// Encrypt a test key and store it.
	enc, err := cipher.Encrypt([]byte("sk-db-key"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	rec := &provider.ProviderKeyRecord{
		Provider:     "openai",
		KeyAlias:     "primary",
		EncryptedKey: enc,
		Weight:       100,
		IsActive:     true,
	}
	_ = store.Create(context.Background(), rec)

	fallback := map[string]string{"openai": "sk-config-key"}
	km := provider.NewKeyManager(store, cipher, fallback)

	key, err := km.SelectKey(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("SelectKey: %v", err)
	}
	if key != "sk-db-key" {
		t.Errorf("expected DB key to take priority, got %q", key)
	}
}

func TestKeyManager_NoKeyReturnsError(t *testing.T) {
	store := &fakeProviderKeyStore{}
	cipher := newTestCipher(t)
	km := provider.NewKeyManager(store, cipher, map[string]string{})

	_, err := km.SelectKey(context.Background(), "openai", "")
	if err == nil {
		t.Fatal("expected error when no key is configured")
	}
}

func TestKeyManager_NilStoreUsesFallback(t *testing.T) {
	fallback := map[string]string{"anthropic": "sk-anthropic"}
	km := provider.NewKeyManager(nil, nil, fallback)

	key, err := km.SelectKey(context.Background(), "anthropic", "")
	if err != nil {
		t.Fatalf("SelectKey: %v", err)
	}
	if key != "sk-anthropic" {
		t.Errorf("expected fallback key, got %q", key)
	}
}

func TestKeyManager_InvalidateCacheRefreshesKeys(t *testing.T) {
	store := &fakeProviderKeyStore{}
	cipher := newTestCipher(t)

	enc1, _ := cipher.Encrypt([]byte("sk-first"))
	rec := &provider.ProviderKeyRecord{
		Provider:     "openai",
		KeyAlias:     "key1",
		EncryptedKey: enc1,
		Weight:       100,
		IsActive:     true,
	}
	_ = store.Create(context.Background(), rec)

	km := provider.NewKeyManager(store, cipher, map[string]string{})

	key1, err := km.SelectKey(context.Background(), "openai", "")
	if err != nil || key1 != "sk-first" {
		t.Fatalf("initial select: err=%v key=%q", err, key1)
	}

	// Update the stored key.
	enc2, _ := cipher.Encrypt([]byte("sk-second"))
	store.records[0].EncryptedKey = enc2

	// Without invalidation, cache returns old key.
	key2, _ := km.SelectKey(context.Background(), "openai", "")
	if key2 != "sk-first" {
		t.Errorf("expected cached key 'sk-first', got %q", key2)
	}

	// After invalidation, new key is returned.
	km.InvalidateCache("openai")
	key3, err := km.SelectKey(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("SelectKey after invalidation: %v", err)
	}
	if key3 != "sk-second" {
		t.Errorf("expected refreshed key 'sk-second', got %q", key3)
	}
}

func TestCryptoRoundtrip(t *testing.T) {
	cipher := newTestCipher(t)
	plaintext := []byte("sk-super-secret-api-key-1234567890")

	enc, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	dec, err := cipher.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(dec) != string(plaintext) {
		t.Errorf("roundtrip mismatch: got %q, want %q", dec, plaintext)
	}
}
