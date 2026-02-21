package fallback

import (
	"math/rand"
	"sort"
	"sync/atomic"

	"github.com/llm-router/gateway/internal/telemetry"
)

// LoadBalancer reorders a target list according to a strategy.
// The first element in the returned slice will be tried first.
type LoadBalancer interface {
	Sort(targets []Target) []Target
}

// WeightedRandom selects targets with probability proportional to weight.
// Targets with weight ≤ 0 are treated as weight 1.
// The chosen target is placed first; remaining order is preserved.
type WeightedRandom struct{}

func (WeightedRandom) Sort(targets []Target) []Target {
	if len(targets) <= 1 {
		return targets
	}

	total := 0
	for _, t := range targets {
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}

	pick := rand.Intn(total)
	chosen := 0
	cumulative := 0
	for i, t := range targets {
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if pick < cumulative {
			chosen = i
			break
		}
	}

	result := make([]Target, len(targets))
	result[0] = targets[chosen]
	j := 1
	for i, t := range targets {
		if i != chosen {
			result[j] = t
			j++
		}
	}
	return result
}

// RoundRobin rotates through targets in a fixed cyclic order.
// Each call advances the internal counter.
type RoundRobin struct {
	counter atomic.Uint64
}

func (rr *RoundRobin) Sort(targets []Target) []Target {
	if len(targets) <= 1 {
		return targets
	}
	n := uint64(len(targets))
	idx := rr.counter.Add(1) % n
	result := make([]Target, len(targets))
	for i := range targets {
		result[i] = targets[(idx+uint64(i))%n]
	}
	return result
}

// LeastLatency prefers the target with the lowest EWMA latency.
// Uses the LatencyTracker for per-provider measurements.
// Providers with no recorded latency are placed last.
type LeastLatency struct {
	Tracker *telemetry.LatencyTracker
}

func (ll LeastLatency) Sort(targets []Target) []Target {
	if len(targets) <= 1 || ll.Tracker == nil {
		return targets
	}
	result := make([]Target, len(targets))
	copy(result, targets)
	sort.SliceStable(result, func(i, j int) bool {
		li := ll.Tracker.Get(result[i].Provider)
		lj := ll.Tracker.Get(result[j].Provider)
		// Unrecorded providers (0) go last.
		if li == 0 && lj == 0 {
			return false
		}
		if li == 0 {
			return false
		}
		if lj == 0 {
			return true
		}
		return li < lj
	})
	return result
}

// LeastCost prefers the target with the lowest cost per token.
// CostPerToken maps provider names to their relative cost (lower is cheaper).
type LeastCost struct {
	CostPerToken map[string]float64
}

func (lc LeastCost) Sort(targets []Target) []Target {
	if len(targets) <= 1 {
		return targets
	}
	result := make([]Target, len(targets))
	copy(result, targets)
	sort.SliceStable(result, func(i, j int) bool {
		ci := lc.CostPerToken[result[i].Provider]
		cj := lc.CostPerToken[result[j].Provider]
		return ci < cj
	})
	return result
}
