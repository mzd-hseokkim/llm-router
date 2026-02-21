package abtest

import "hash/fnv"

// Assign returns the variant name that entityID maps to for exp.
// Returns "" when:
//   - the experiment is not active
//   - the entity is outside the configured sample rate
func Assign(exp *Experiment, entityID string) string {
	if !exp.IsActive() {
		return ""
	}

	// Sample-rate check — stable per entity per experiment.
	if exp.Target.SampleRate < 1.0 {
		h := fnv.New32a()
		h.Write([]byte("sample\x00" + exp.ID + "\x00" + entityID))
		// Map [0, 2^32) into [0.0, 1.0)
		ratio := float64(h.Sum32()) / float64(1<<32)
		if ratio >= exp.Target.SampleRate {
			return ""
		}
	}

	// Consistent variant assignment via bucket in [0, 100).
	h := fnv.New32a()
	h.Write([]byte(exp.ID + "\x00" + entityID))
	bucket := h.Sum32() % 100

	cumulative := uint32(0)
	for _, split := range exp.TrafficSplit {
		cumulative += uint32(split.Weight)
		if bucket < cumulative {
			return split.Variant
		}
	}

	// Fallback: first variant (handles rounding if weights don't sum to 100).
	if len(exp.TrafficSplit) > 0 {
		return exp.TrafficSplit[0].Variant
	}
	return ""
}
