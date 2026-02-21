package types

import (
	"time"

	"github.com/google/uuid"
)

// Strategy controls target selection for a routing rule.
type Strategy string

const (
	StrategyDirect    Strategy = "direct"       // single target
	StrategyWeighted  Strategy = "weighted"      // weighted random
	StrategyLeastCost Strategy = "least_cost"    // cheapest target
	StrategyFailover  Strategy = "failover"      // ordered fallback
	StrategyQuality   Strategy = "quality_first" // highest quality first
)

// RuleTarget is one entry in a routing rule's target list.
type RuleTarget struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Weight   int    `json:"weight,omitempty"`
}

// RouteMatch defines the conditions under which a rule applies.
// All non-zero fields must match; zero / nil fields are ignored.
type RouteMatch struct {
	Model       string `json:"model,omitempty"`
	ModelPrefix string `json:"model_prefix,omitempty"`
	ModelRegex  string `json:"model_regex,omitempty"`

	KeyID  *uuid.UUID `json:"key_id,omitempty"`
	UserID *uuid.UUID `json:"user_id,omitempty"`
	TeamID *uuid.UUID `json:"team_id,omitempty"`
	OrgID  *uuid.UUID `json:"org_id,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`

	MinContextTokens int  `json:"min_context_tokens,omitempty"`
	MaxContextTokens int  `json:"max_context_tokens,omitempty"`
	HasTools         bool `json:"has_tools,omitempty"`
}

// RouteRule is a single routing rule with match conditions and target strategy.
type RouteRule struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Priority int       `json:"priority"`
	Enabled  bool      `json:"enabled"`

	Match    RouteMatch  `json:"match"`
	Strategy Strategy    `json:"strategy"`
	Targets  []RuleTarget `json:"targets"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
