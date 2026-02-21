package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Domain types ---

// Organization is a top-level tenant entity.
type Organization struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Team belongs to an organization.
type Team struct {
	ID        uuid.UUID
	OrgID     *uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// User belongs to an organization and optionally a team.
type User struct {
	ID        uuid.UUID
	OrgID     *uuid.UUID
	TeamID    *uuid.UUID
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Sentinel errors
var (
	ErrOrgNotFound  = errors.New("organization not found")
	ErrTeamNotFound = errors.New("team not found")
	ErrUserNotFound = errors.New("user not found")
)

// --- OrgStore ---

// OrgStore handles CRUD for organizations, teams, and users.
type OrgStore struct {
	pool *pgxpool.Pool
}

// NewOrgStore creates an OrgStore.
func NewOrgStore(pool *pgxpool.Pool) *OrgStore {
	return &OrgStore{pool: pool}
}

// CreateOrg inserts a new organization.
func (s *OrgStore) CreateOrg(ctx context.Context, name string) (*Organization, error) {
	const q = `INSERT INTO organizations (name) VALUES ($1) RETURNING id, created_at, updated_at`
	org := &Organization{Name: name}
	err := s.pool.QueryRow(ctx, q, name).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}
	return org, nil
}

// GetOrg returns an organization by ID.
func (s *OrgStore) GetOrg(ctx context.Context, id uuid.UUID) (*Organization, error) {
	const q = `SELECT id, name, created_at, updated_at FROM organizations WHERE id = $1`
	org := &Organization{}
	err := s.pool.QueryRow(ctx, q, id).Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrgNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get org: %w", err)
	}
	return org, nil
}

// ListOrgs returns all organizations.
func (s *OrgStore) ListOrgs(ctx context.Context) ([]*Organization, error) {
	const q = `SELECT id, name, created_at, updated_at FROM organizations ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list orgs: %w", err)
	}
	defer rows.Close()

	var orgs []*Organization
	for rows.Next() {
		org := &Organization{}
		if err := rows.Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

// UpdateOrg changes the name of an organization.
func (s *OrgStore) UpdateOrg(ctx context.Context, id uuid.UUID, name string) (*Organization, error) {
	const q = `UPDATE organizations SET name = $1, updated_at = NOW() WHERE id = $2 RETURNING id, name, created_at, updated_at`
	org := &Organization{}
	err := s.pool.QueryRow(ctx, q, name, id).Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrgNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update org: %w", err)
	}
	return org, nil
}

// --- Teams ---

// CreateTeam inserts a new team.
func (s *OrgStore) CreateTeam(ctx context.Context, orgID *uuid.UUID, name string) (*Team, error) {
	const q = `INSERT INTO teams (org_id, name) VALUES ($1, $2) RETURNING id, created_at, updated_at`
	team := &Team{OrgID: orgID, Name: name}
	err := s.pool.QueryRow(ctx, q, uuidPtrToParam(orgID), name).Scan(&team.ID, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return team, nil
}

// GetTeam returns a team by ID.
func (s *OrgStore) GetTeam(ctx context.Context, id uuid.UUID) (*Team, error) {
	const q = `SELECT id, org_id, name, created_at, updated_at FROM teams WHERE id = $1`
	team := &Team{}
	var orgID pgtype.UUID
	err := s.pool.QueryRow(ctx, q, id).Scan(&team.ID, &orgID, &team.Name, &team.CreatedAt, &team.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTeamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	if orgID.Valid {
		uid := uuid.UUID(orgID.Bytes)
		team.OrgID = &uid
	}
	return team, nil
}

// ListTeams returns teams, optionally filtered by org_id.
func (s *OrgStore) ListTeams(ctx context.Context, orgID *uuid.UUID) ([]*Team, error) {
	var q string
	var args []any
	if orgID != nil {
		q = `SELECT id, org_id, name, created_at, updated_at FROM teams WHERE org_id = $1 ORDER BY created_at DESC`
		args = []any{uuidPtrToParam(orgID)}
	} else {
		q = `SELECT id, org_id, name, created_at, updated_at FROM teams ORDER BY created_at DESC`
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		team := &Team{}
		var oid pgtype.UUID
		if err := rows.Scan(&team.ID, &oid, &team.Name, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		if oid.Valid {
			uid := uuid.UUID(oid.Bytes)
			team.OrgID = &uid
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

// UpdateTeam changes the name of a team.
func (s *OrgStore) UpdateTeam(ctx context.Context, id uuid.UUID, name string) (*Team, error) {
	const q = `UPDATE teams SET name = $1, updated_at = NOW() WHERE id = $2 RETURNING id, org_id, name, created_at, updated_at`
	team := &Team{}
	var orgID pgtype.UUID
	err := s.pool.QueryRow(ctx, q, name, id).Scan(&team.ID, &orgID, &team.Name, &team.CreatedAt, &team.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTeamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update team: %w", err)
	}
	if orgID.Valid {
		uid := uuid.UUID(orgID.Bytes)
		team.OrgID = &uid
	}
	return team, nil
}

// --- Users ---

// CreateUser inserts a new user.
func (s *OrgStore) CreateUser(ctx context.Context, orgID *uuid.UUID, teamID *uuid.UUID, email string) (*User, error) {
	const q = `INSERT INTO users (org_id, team_id, email) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at`
	user := &User{OrgID: orgID, TeamID: teamID, Email: email}
	err := s.pool.QueryRow(ctx, q, uuidPtrToParam(orgID), uuidPtrToParam(teamID), email).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// GetUser returns a user by ID.
func (s *OrgStore) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	const q = `SELECT id, org_id, team_id, email, created_at, updated_at FROM users WHERE id = $1`
	user := &User{}
	var orgID, teamID pgtype.UUID
	err := s.pool.QueryRow(ctx, q, id).Scan(&user.ID, &orgID, &teamID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if orgID.Valid {
		uid := uuid.UUID(orgID.Bytes)
		user.OrgID = &uid
	}
	if teamID.Valid {
		tid := uuid.UUID(teamID.Bytes)
		user.TeamID = &tid
	}
	return user, nil
}

// ListUsers returns users, optionally filtered by org_id.
func (s *OrgStore) ListUsers(ctx context.Context, orgID *uuid.UUID) ([]*User, error) {
	var q string
	var args []any
	if orgID != nil {
		q = `SELECT id, org_id, team_id, email, created_at, updated_at FROM users WHERE org_id = $1 ORDER BY created_at DESC`
		args = []any{uuidPtrToParam(orgID)}
	} else {
		q = `SELECT id, org_id, team_id, email, created_at, updated_at FROM users ORDER BY created_at DESC`
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		var oid, tid pgtype.UUID
		if err := rows.Scan(&user.ID, &oid, &tid, &user.Email, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		if oid.Valid {
			uid := uuid.UUID(oid.Bytes)
			user.OrgID = &uid
		}
		if tid.Valid {
			t := uuid.UUID(tid.Bytes)
			user.TeamID = &t
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// --- Team Members (many-to-many) ---

// AddTeamMember adds a user to a team with the given role.
func (s *OrgStore) AddTeamMember(ctx context.Context, teamID, userID uuid.UUID, role string) error {
	const q = `
		INSERT INTO team_members (team_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role`
	_, err := s.pool.Exec(ctx, q, teamID, userID, role)
	if err != nil {
		return fmt.Errorf("add team member: %w", err)
	}
	return nil
}

// RemoveTeamMember removes a user from a team.
func (s *OrgStore) RemoveTeamMember(ctx context.Context, teamID, userID uuid.UUID) error {
	const q = `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`
	_, err := s.pool.Exec(ctx, q, teamID, userID)
	return err
}

// GetUserPrimaryOrg returns the first org_id associated with a user.
func (s *OrgStore) GetUserPrimaryOrg(ctx context.Context, userID uuid.UUID) (*uuid.UUID, *uuid.UUID, error) {
	const q = `SELECT org_id, team_id FROM users WHERE id = $1`
	var orgID, teamID pgtype.UUID
	err := s.pool.QueryRow(ctx, q, userID).Scan(&orgID, &teamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get user primary org: %w", err)
	}
	var orgUUID, teamUUID *uuid.UUID
	if orgID.Valid {
		uid := uuid.UUID(orgID.Bytes)
		orgUUID = &uid
	}
	if teamID.Valid {
		tid := uuid.UUID(teamID.Bytes)
		teamUUID = &tid
	}
	return orgUUID, teamUUID, nil
}

// GetUserByEmail retrieves a user by their email address.
func (s *OrgStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	const q = `SELECT id, org_id, team_id, email, created_at, updated_at FROM users WHERE email = $1`
	user := &User{}
	var orgID, teamID pgtype.UUID
	err := s.pool.QueryRow(ctx, q, email).Scan(&user.ID, &orgID, &teamID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	if orgID.Valid {
		uid := uuid.UUID(orgID.Bytes)
		user.OrgID = &uid
	}
	if teamID.Valid {
		tid := uuid.UUID(teamID.Bytes)
		user.TeamID = &tid
	}
	return user, nil
}

// UpdateUser updates email (and optionally team) for a user.
func (s *OrgStore) UpdateUser(ctx context.Context, id uuid.UUID, email string, teamID *uuid.UUID) (*User, error) {
	const q = `UPDATE users SET email = $1, team_id = $2, updated_at = NOW() WHERE id = $3
		RETURNING id, org_id, team_id, email, created_at, updated_at`
	user := &User{}
	var orgID, tid pgtype.UUID
	err := s.pool.QueryRow(ctx, q, email, uuidPtrToParam(teamID), id).Scan(
		&user.ID, &orgID, &tid, &user.Email, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	if orgID.Valid {
		uid := uuid.UUID(orgID.Bytes)
		user.OrgID = &uid
	}
	if tid.Valid {
		t := uuid.UUID(tid.Bytes)
		user.TeamID = &t
	}
	return user, nil
}
