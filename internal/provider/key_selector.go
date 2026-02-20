package provider

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// weightedRandom selects a providerKey using weighted random selection.
// Keys with higher weight values are chosen proportionally more often.
func weightedRandom(keys []*providerKey) (*providerKey, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("no keys to select from")
	}
	if len(keys) == 1 {
		return keys[0], nil
	}

	totalWeight := 0
	for _, k := range keys {
		w := k.weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	pick, err := cryptoRandN(totalWeight)
	if err != nil {
		return nil, fmt.Errorf("random selection: %w", err)
	}

	cumulative := 0
	for _, k := range keys {
		w := k.weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if pick < cumulative {
			return k, nil
		}
	}
	return keys[len(keys)-1], nil
}

// cryptoRandN returns a cryptographically secure random integer in [0, n).
func cryptoRandN(n int) (int, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint64(b[:]) % uint64(n)), nil
}
