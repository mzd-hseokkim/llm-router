package tenant

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Settings holds configurable limits and rules for an org or team.
type Settings struct {
	RPMLimit      int      `json:"rpm_limit,omitempty"`
	TPMLimit      int      `json:"tpm_limit,omitempty"`
	BudgetUSD     float64  `json:"budget_usd,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	BlockedModels []string `json:"blocked_models,omitempty"`
}

// OrgEntity holds org data with parsed settings.
type OrgEntity struct {
	ID       uuid.UUID
	Name     string
	Slug     string
	Settings Settings
	IsActive bool
}

// TeamEntity holds team data with parsed settings.
type TeamEntity struct {
	ID       uuid.UUID
	OrgID    uuid.UUID
	Name     string
	Slug     string
	Settings Settings
}

// MergeSettings computes the effective settings for a team by merging org and team settings.
// The more restrictive value always wins (child cannot relax parent limits).
func MergeSettings(org, team Settings) Settings {
	merged := Settings{
		RPMLimit:  minNonZero(org.RPMLimit, team.RPMLimit),
		TPMLimit:  minNonZero(org.TPMLimit, team.TPMLimit),
		BudgetUSD: minNonZeroFloat(org.BudgetUSD, team.BudgetUSD),
	}

	// Model allow/block lists: intersection of org and team
	if len(org.AllowedModels) == 0 {
		merged.AllowedModels = team.AllowedModels
	} else if len(team.AllowedModels) == 0 {
		merged.AllowedModels = org.AllowedModels
	} else {
		merged.AllowedModels = intersection(org.AllowedModels, team.AllowedModels)
	}

	// Union of block lists (block if either blocks it)
	merged.BlockedModels = union(org.BlockedModels, team.BlockedModels)

	return merged
}

// ParseSettings unmarshals a JSONB settings blob.
func ParseSettings(raw json.RawMessage) Settings {
	if len(raw) == 0 {
		return Settings{}
	}
	var s Settings
	_ = json.Unmarshal(raw, &s)
	return s
}

func minNonZero(a, b int) int {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func minNonZeroFloat(a, b float64) float64 {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func intersection(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, v := range b {
		set[v] = true
	}
	var result []string
	for _, v := range a {
		if set[v] {
			result = append(result, v)
		}
	}
	return result
}

func union(a, b []string) []string {
	set := make(map[string]bool)
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		set[v] = true
	}
	result := make([]string, 0, len(set))
	for v := range set {
		result = append(result, v)
	}
	return result
}
