package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/llm-router/gateway/internal/audit"
)

// AuditStore persists audit events to PostgreSQL.
type AuditStore struct {
	pool *pgxpool.Pool
}

// NewAuditStore creates an AuditStore backed by pool.
func NewAuditStore(pool *pgxpool.Pool) *AuditStore {
	return &AuditStore{pool: pool}
}

// Insert writes a single audit event. The table is insert-only (no UPDATE/DELETE).
func (s *AuditStore) Insert(ctx context.Context, e *audit.Event) error {
	changesJSON, err := json.Marshal(e.Changes)
	if err != nil {
		return fmt.Errorf("audit: marshal changes: %w", err)
	}
	metaJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}

	var ipVal any // INET — pass as string or nil
	if e.IPAddress != "" {
		stripped := stripPort(e.IPAddress)
		if net.ParseIP(stripped) != nil {
			ipVal = stripped
		}
	}

	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_logs
			(event_type, action, actor_type, actor_id, actor_email,
			 ip_address, user_agent, resource_type, resource_id, resource_name,
			 changes, metadata, request_id, org_id, team_id, timestamp)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		e.EventType, e.Action, e.ActorType, e.ActorID, nilIfEmpty(e.ActorEmail),
		ipVal, nilIfEmpty(e.UserAgent), nilIfEmpty(e.ResourceType), e.ResourceID, nilIfEmpty(e.ResourceName),
		changesJSON, metaJSON, nilIfEmpty(e.RequestID), e.OrgID, e.TeamID, ts,
	)
	return err
}

// AuditFilter holds query parameters for listing audit logs.
type AuditFilter struct {
	From         time.Time
	To           time.Time
	ActorID      string
	EventType    string
	ResourceID   string
	SecurityOnly bool
	Limit        int
	Page         int
}

// List returns audit log entries matching the filter.
func (s *AuditStore) List(ctx context.Context, f AuditFilter) ([]*audit.Event, int, error) {
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 100
	}
	if f.Page < 1 {
		f.Page = 1
	}
	offset := (f.Page - 1) * f.Limit

	var conditions []string
	var args []any
	argIdx := 1

	add := func(cond string, val any) {
		conditions = append(conditions, fmt.Sprintf(cond, argIdx))
		args = append(args, val)
		argIdx++
	}

	if !f.From.IsZero() {
		add("timestamp >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("timestamp <= $%d", f.To)
	}
	if f.ActorID != "" {
		add("actor_id = $%d", f.ActorID)
	}
	if f.EventType != "" {
		add("event_type = $%d", f.EventType)
	}
	if f.ResourceID != "" {
		add("resource_id = $%d", f.ResourceID)
	}
	if f.SecurityOnly {
		conditions = append(conditions, "(event_type LIKE 'auth.%' OR event_type LIKE 'guardrail.%')")
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM audit_logs " + where
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit: count: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, event_type, action, actor_type, actor_id, actor_email,
		       host(ip_address), user_agent, resource_type, resource_id, resource_name,
		       changes, metadata, request_id, org_id, team_id, timestamp
		FROM audit_logs
		%s
		ORDER BY timestamp DESC
		LIMIT %d OFFSET %d`, where, f.Limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	var events []*audit.Event
	for rows.Next() {
		e := &audit.Event{}
		var (
			id, actorID, resourceID, orgID, teamID pgtype.UUID
			actorEmail, ipAddr, userAgent          pgtype.Text
			resourceType, resourceName, requestID  pgtype.Text
			changesRaw, metaRaw                    []byte
		)
		if err := rows.Scan(
			&id, &e.EventType, &e.Action, &e.ActorType, &actorID, &actorEmail,
			&ipAddr, &userAgent, &resourceType, &resourceID, &resourceName,
			&changesRaw, &metaRaw, &requestID, &orgID, &teamID, &e.Timestamp,
		); err != nil {
			return nil, 0, fmt.Errorf("audit: scan: %w", err)
		}
		if id.Valid {
			u := uuid.UUID(id.Bytes)
			e.ID = &u
		}
		if actorID.Valid {
			u := uuid.UUID(actorID.Bytes)
			e.ActorID = &u
		}
		if resourceID.Valid {
			u := uuid.UUID(resourceID.Bytes)
			e.ResourceID = &u
		}
		if orgID.Valid {
			u := uuid.UUID(orgID.Bytes)
			e.OrgID = &u
		}
		if teamID.Valid {
			u := uuid.UUID(teamID.Bytes)
			e.TeamID = &u
		}
		e.ActorEmail = actorEmail.String
		e.IPAddress = ipAddr.String
		e.UserAgent = userAgent.String
		e.ResourceType = resourceType.String
		e.ResourceName = resourceName.String
		e.RequestID = requestID.String
		_ = json.Unmarshal(changesRaw, &e.Changes)
		_ = json.Unmarshal(metaRaw, &e.Metadata)
		events = append(events, e)
	}
	return events, total, rows.Err()
}

func stripPort(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
