package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/auth"
)

func TestGenerateKey(t *testing.T) {
	rawKey, keyHash, keyPrefix, err := auth.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	// Key format: sk-{43 chars} = 46 total
	if !strings.HasPrefix(rawKey, "sk-") {
		t.Errorf("rawKey %q does not start with 'sk-'", rawKey)
	}
	if len(rawKey) != 46 {
		t.Errorf("rawKey length = %d, want 46", len(rawKey))
	}

	// Prefix = first 7 chars
	if keyPrefix != rawKey[:7] {
		t.Errorf("keyPrefix = %q, want %q", keyPrefix, rawKey[:7])
	}

	// Hash must match
	if keyHash != auth.HashKey(rawKey) {
		t.Error("keyHash does not match HashKey(rawKey)")
	}

	// Two generated keys must differ
	rawKey2, keyHash2, _, _ := auth.GenerateKey()
	if rawKey == rawKey2 {
		t.Error("two generated keys are equal — very unlikely unless broken")
	}
	if keyHash == keyHash2 {
		t.Error("two key hashes are equal — implies collision")
	}
}

func TestVerifyHash(t *testing.T) {
	rawKey, keyHash, _, err := auth.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	if !auth.VerifyHash(rawKey, keyHash) {
		t.Error("VerifyHash returned false for valid key/hash pair")
	}
	if auth.VerifyHash("tampered", keyHash) {
		t.Error("VerifyHash returned true for tampered key")
	}
}

func TestVirtualKey_IsValid(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name    string
		key     auth.VirtualKey
		wantErr error
	}{
		{
			name:    "active no expiry",
			key:     auth.VirtualKey{IsActive: true},
			wantErr: nil,
		},
		{
			name:    "inactive",
			key:     auth.VirtualKey{IsActive: false},
			wantErr: auth.ErrKeyInactive,
		},
		{
			name:    "expired",
			key:     auth.VirtualKey{IsActive: true, ExpiresAt: &past},
			wantErr: auth.ErrKeyExpired,
		},
		{
			name:    "not yet expired",
			key:     auth.VirtualKey{IsActive: true, ExpiresAt: &future},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.key.IsValid()
			if err != tt.wantErr {
				t.Errorf("IsValid() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestVirtualKey_CanAccessModel(t *testing.T) {
	tests := []struct {
		name          string
		allowedModels []string
		blockedModels []string
		model         string
		wantErr       error
	}{
		{
			name:    "no restrictions",
			model:   "openai/gpt-4o",
			wantErr: nil,
		},
		{
			name:          "blocked model",
			blockedModels: []string{"openai/gpt-4o"},
			model:         "openai/gpt-4o",
			wantErr:       auth.ErrModelBlocked,
		},
		{
			name:          "allowed list — in list",
			allowedModels: []string{"openai/gpt-4o", "anthropic/claude-sonnet-4-20250514"},
			model:         "openai/gpt-4o",
			wantErr:       nil,
		},
		{
			name:          "allowed list — not in list",
			allowedModels: []string{"openai/gpt-4o"},
			model:         "anthropic/claude-sonnet-4-20250514",
			wantErr:       auth.ErrModelNotAllowed,
		},
		{
			name:          "blocked takes priority over allowed",
			allowedModels: []string{"openai/gpt-4o"},
			blockedModels: []string{"openai/gpt-4o"},
			model:         "openai/gpt-4o",
			wantErr:       auth.ErrModelBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := auth.VirtualKey{
				AllowedModels: tt.allowedModels,
				BlockedModels: tt.blockedModels,
			}
			err := k.CanAccessModel(tt.model)
			if err != tt.wantErr {
				t.Errorf("CanAccessModel() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateKey_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		rawKey, _, _, err := auth.GenerateKey()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if seen[rawKey] {
			t.Fatalf("duplicate key generated at iteration %d", i)
		}
		seen[rawKey] = true
	}
}
