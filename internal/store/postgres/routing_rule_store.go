package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/gateway/types"
)

// RoutingRuleStore persists routing rules in PostgreSQL.
type RoutingRuleStore struct {
	pool *pgxpool.Pool
}

// NewRoutingRuleStore creates a RoutingRuleStore.
func NewRoutingRuleStore(pool *pgxpool.Pool) *RoutingRuleStore {
	return &RoutingRuleStore{pool: pool}
}

// List returns all routing rules ordered by priority descending.
func (s *RoutingRuleStore) List(ctx context.Context) ([]types.RouteRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, priority, enabled,
		       match_model, match_model_prefix, match_model_regex,
		       match_key_id, match_user_id, match_team_id, match_org_id,
		       match_metadata, match_min_context_tokens, match_max_context_tokens, match_has_tools,
		       strategy, targets, created_at, updated_at
		FROM routing_rules
		ORDER BY priority DESC, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list routing rules: %w", err)
	}
	defer rows.Close()

	var rules []types.RouteRule
	for rows.Next() {
		rule, err := scanRoutingRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// Get returns a single routing rule by ID.
func (s *RoutingRuleStore) Get(ctx context.Context, id uuid.UUID) (*types.RouteRule, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, priority, enabled,
		       match_model, match_model_prefix, match_model_regex,
		       match_key_id, match_user_id, match_team_id, match_org_id,
		       match_metadata, match_min_context_tokens, match_max_context_tokens, match_has_tools,
		       strategy, targets, created_at, updated_at
		FROM routing_rules WHERE id = $1
	`, id)
	rule, err := scanRoutingRule(row)
	if err != nil {
		return nil, fmt.Errorf("get routing rule: %w", err)
	}
	return &rule, nil
}

// Create inserts a new routing rule.
func (s *RoutingRuleStore) Create(ctx context.Context, rule *types.RouteRule) error {
	rule.ID = uuid.New()
	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	metaJSON, _ := json.Marshal(rule.Match.Metadata)
	targetsJSON, _ := json.Marshal(rule.Targets)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO routing_rules (
			id, name, priority, enabled,
			match_model, match_model_prefix, match_model_regex,
			match_key_id, match_user_id, match_team_id, match_org_id,
			match_metadata, match_min_context_tokens, match_max_context_tokens, match_has_tools,
			strategy, targets, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10, $11,
			$12, $13, $14, $15,
			$16, $17, $18, $19
		)`,
		rule.ID, rule.Name, rule.Priority, rule.Enabled,
		nullString(rule.Match.Model), nullString(rule.Match.ModelPrefix), nullString(rule.Match.ModelRegex),
		rule.Match.KeyID, rule.Match.UserID, rule.Match.TeamID, rule.Match.OrgID,
		metaJSON, nullInteger(rule.Match.MinContextTokens), nullInteger(rule.Match.MaxContextTokens), rule.Match.HasTools,
		string(rule.Strategy), targetsJSON, rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create routing rule: %w", err)
	}
	return nil
}

// Update replaces a routing rule.
func (s *RoutingRuleStore) Update(ctx context.Context, rule *types.RouteRule) error {
	rule.UpdatedAt = time.Now()
	metaJSON, _ := json.Marshal(rule.Match.Metadata)
	targetsJSON, _ := json.Marshal(rule.Targets)

	cmd, err := s.pool.Exec(ctx, `
		UPDATE routing_rules SET
			name=$2, priority=$3, enabled=$4,
			match_model=$5, match_model_prefix=$6, match_model_regex=$7,
			match_key_id=$8, match_user_id=$9, match_team_id=$10, match_org_id=$11,
			match_metadata=$12, match_min_context_tokens=$13, match_max_context_tokens=$14, match_has_tools=$15,
			strategy=$16, targets=$17, updated_at=$18
		WHERE id=$1`,
		rule.ID, rule.Name, rule.Priority, rule.Enabled,
		nullString(rule.Match.Model), nullString(rule.Match.ModelPrefix), nullString(rule.Match.ModelRegex),
		rule.Match.KeyID, rule.Match.UserID, rule.Match.TeamID, rule.Match.OrgID,
		metaJSON, nullInteger(rule.Match.MinContextTokens), nullInteger(rule.Match.MaxContextTokens), rule.Match.HasTools,
		string(rule.Strategy), targetsJSON, rule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update routing rule: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("routing rule not found: %s", rule.ID)
	}
	return nil
}

// Delete removes a routing rule by ID.
func (s *RoutingRuleStore) Delete(ctx context.Context, id uuid.UUID) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM routing_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete routing rule: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("routing rule not found: %s", id)
	}
	return nil
}

// scannable abstracts pgx.Row and pgx.Rows so the same scan function works for both.
type scannable interface {
	Scan(dest ...any) error
}

func scanRoutingRule(row scannable) (types.RouteRule, error) {
	var r types.RouteRule
	var metaRaw, targetsRaw []byte
	var matchModel, matchModelPrefix, matchModelRegex *string
	var matchMinCtx, matchMaxCtx *int
	var matchHasTools *bool
	var strategy string

	err := row.Scan(
		&r.ID, &r.Name, &r.Priority, &r.Enabled,
		&matchModel, &matchModelPrefix, &matchModelRegex,
		&r.Match.KeyID, &r.Match.UserID, &r.Match.TeamID, &r.Match.OrgID,
		&metaRaw, &matchMinCtx, &matchMaxCtx, &matchHasTools,
		&strategy, &targetsRaw, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return types.RouteRule{}, fmt.Errorf("scan routing rule: %w", err)
	}

	r.Strategy = types.Strategy(strategy)
	if matchModel != nil {
		r.Match.Model = *matchModel
	}
	if matchModelPrefix != nil {
		r.Match.ModelPrefix = *matchModelPrefix
	}
	if matchModelRegex != nil {
		r.Match.ModelRegex = *matchModelRegex
	}
	if matchMinCtx != nil {
		r.Match.MinContextTokens = *matchMinCtx
	}
	if matchMaxCtx != nil {
		r.Match.MaxContextTokens = *matchMaxCtx
	}
	if matchHasTools != nil {
		r.Match.HasTools = *matchHasTools
	}
	if len(metaRaw) > 0 && string(metaRaw) != "null" {
		_ = json.Unmarshal(metaRaw, &r.Match.Metadata)
	}
	if len(targetsRaw) > 0 {
		_ = json.Unmarshal(targetsRaw, &r.Targets)
	}

	return r, nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullInteger(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}
