package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/llm-router/gateway/internal/auth/rbac"
)

// RoleRecord is the DB representation of a role.
type RoleRecord struct {
	ID          uuid.UUID
	OrgID       *uuid.UUID
	Name        string
	Description string
	IsSystem    bool
	Permissions []rbac.Permission
	CreatedAt   time.Time
}

var ErrRoleNotFound = errors.New("role not found")

// RoleStore manages roles and user_roles in PostgreSQL.
type RoleStore struct {
	pool *pgxpool.Pool
}

func NewRoleStore(pool *pgxpool.Pool) *RoleStore {
	return &RoleStore{pool: pool}
}

// GetUserRoles implements rbac.Store — returns all roles assigned to a user.
func (s *RoleStore) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]rbac.UserRole, error) {
	const q = `
		SELECT r.name, r.permissions, ur.org_id, ur.team_id
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1`

	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}
	defer rows.Close()

	var result []rbac.UserRole
	for rows.Next() {
		var (
			roleName string
			permJSON []byte
			orgID    pgtype.UUID
			teamID   pgtype.UUID
		)
		if err := rows.Scan(&roleName, &permJSON, &orgID, &teamID); err != nil {
			return nil, err
		}

		var perms []rbac.Permission
		if err := json.Unmarshal(permJSON, &perms); err != nil {
			return nil, fmt.Errorf("unmarshal permissions: %w", err)
		}

		ur := rbac.UserRole{
			Role:        rbac.Role(roleName),
			Permissions: perms,
		}
		if orgID.Valid {
			ur.OrgID = uuid.UUID(orgID.Bytes).String()
		}
		if teamID.Valid {
			ur.TeamID = uuid.UUID(teamID.Bytes).String()
		}
		result = append(result, ur)
	}
	return result, rows.Err()
}

// AssignRole assigns a role to a user (scoped to org/team).
func (s *RoleStore) AssignRole(ctx context.Context, userID, roleID uuid.UUID, orgID, teamID *uuid.UUID) error {
	const q = `
		INSERT INTO user_roles (user_id, role_id, org_id, team_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`
	_, err := s.pool.Exec(ctx, q, userID, roleID, uuidPtrToParam(orgID), uuidPtrToParam(teamID))
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

// RevokeRole removes a role assignment from a user.
func (s *RoleStore) RevokeRole(ctx context.Context, userID, roleID uuid.UUID, orgID *uuid.UUID) error {
	const q = `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2 AND org_id = $3`
	_, err := s.pool.Exec(ctx, q, userID, roleID, uuidPtrToParam(orgID))
	return err
}

// GetRoleByName fetches a role by name (optionally scoped to an org).
func (s *RoleStore) GetRoleByName(ctx context.Context, name string, orgID *uuid.UUID) (*RoleRecord, error) {
	const q = `
		SELECT id, org_id, name, description, is_system, permissions, created_at
		FROM roles
		WHERE name = $1 AND (org_id = $2 OR org_id IS NULL)
		ORDER BY org_id NULLS LAST
		LIMIT 1`

	rec := &RoleRecord{}
	var (
		oid      pgtype.UUID
		permJSON []byte
	)
	err := s.pool.QueryRow(ctx, q, name, uuidPtrToParam(orgID)).Scan(
		&rec.ID, &oid, &rec.Name, &rec.Description, &rec.IsSystem, &permJSON, &rec.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get role by name: %w", err)
	}
	if oid.Valid {
		uid := uuid.UUID(oid.Bytes)
		rec.OrgID = &uid
	}
	if err := json.Unmarshal(permJSON, &rec.Permissions); err != nil {
		return nil, fmt.Errorf("unmarshal permissions: %w", err)
	}
	return rec, nil
}

// ListRoles returns all roles visible to an org (system roles + org-specific roles).
func (s *RoleStore) ListRoles(ctx context.Context, orgID *uuid.UUID) ([]*RoleRecord, error) {
	const q = `
		SELECT id, org_id, name, description, is_system, permissions, created_at
		FROM roles
		WHERE org_id IS NULL OR org_id = $1
		ORDER BY is_system DESC, name`

	rows, err := s.pool.Query(ctx, q, uuidPtrToParam(orgID))
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var result []*RoleRecord
	for rows.Next() {
		rec := &RoleRecord{}
		var (
			oid      pgtype.UUID
			permJSON []byte
		)
		if err := rows.Scan(&rec.ID, &oid, &rec.Name, &rec.Description, &rec.IsSystem, &permJSON, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if oid.Valid {
			uid := uuid.UUID(oid.Bytes)
			rec.OrgID = &uid
		}
		if err := json.Unmarshal(permJSON, &rec.Permissions); err != nil {
			return nil, fmt.Errorf("unmarshal permissions: %w", err)
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

// CreateCustomRole creates a new org-scoped role.
func (s *RoleStore) CreateCustomRole(ctx context.Context, orgID uuid.UUID, name, description string, perms []rbac.Permission) (*RoleRecord, error) {
	permJSON, err := json.Marshal(perms)
	if err != nil {
		return nil, err
	}
	const q = `
		INSERT INTO roles (org_id, name, description, is_system, permissions)
		VALUES ($1, $2, $3, false, $4)
		RETURNING id, created_at`
	rec := &RoleRecord{OrgID: &orgID, Name: name, Description: description, Permissions: perms}
	err = s.pool.QueryRow(ctx, q, orgID, name, description, permJSON).Scan(&rec.ID, &rec.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}
	return rec, nil
}
