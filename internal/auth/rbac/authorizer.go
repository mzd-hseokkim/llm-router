package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// UserInfo holds the authenticated user's identity and assigned roles.
type UserInfo struct {
	ID    uuid.UUID
	Email string
	Roles []UserRole
}

// HasPermission returns true if any of the user's roles grants the permission.
// Scope check: if orgID / teamID are provided, the role must match that scope
// (or be a global/org-wide role covering it).
func (u *UserInfo) HasPermission(p Permission, orgID, teamID string) bool {
	for _, r := range u.Roles {
		// Role scope check
		if r.OrgID != "" && r.OrgID != orgID {
			continue
		}
		if r.TeamID != "" && r.TeamID != teamID {
			continue
		}
		if r.HasPermission(p) {
			return true
		}
	}
	return false
}

// Store is the minimal interface Authorizer needs from the database.
type Store interface {
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]UserRole, error)
}

// Authorizer resolves and caches user permissions.
type Authorizer struct {
	store Store
	redis *redis.Client
	ttl   time.Duration
}

// NewAuthorizer creates an Authorizer. redis may be nil to disable caching.
func NewAuthorizer(store Store, rdb *redis.Client) *Authorizer {
	return &Authorizer{
		store: store,
		redis: rdb,
		ttl:   5 * time.Minute,
	}
}

// GetUserInfo loads (or caches) the UserInfo for the given user ID.
func (a *Authorizer) GetUserInfo(ctx context.Context, userID uuid.UUID) (*UserInfo, error) {
	if a.redis != nil {
		if ui, err := a.fromCache(ctx, userID); err == nil {
			return ui, nil
		}
	}

	roles, err := a.store.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	ui := &UserInfo{ID: userID, Roles: roles}
	if a.redis != nil {
		_ = a.toCache(ctx, ui)
	}
	return ui, nil
}

// InvalidateCache removes cached roles for a user (call after role changes).
func (a *Authorizer) InvalidateCache(ctx context.Context, userID uuid.UUID) {
	if a.redis != nil {
		a.redis.Del(ctx, cacheKey(userID))
	}
}

func cacheKey(userID uuid.UUID) string {
	return "rbac:user:" + userID.String()
}

func (a *Authorizer) fromCache(ctx context.Context, userID uuid.UUID) (*UserInfo, error) {
	data, err := a.redis.Get(ctx, cacheKey(userID)).Bytes()
	if err != nil {
		return nil, err
	}
	var ui UserInfo
	if err := json.Unmarshal(data, &ui); err != nil {
		return nil, err
	}
	return &ui, nil
}

func (a *Authorizer) toCache(ctx context.Context, ui *UserInfo) error {
	data, err := json.Marshal(ui)
	if err != nil {
		return err
	}
	return a.redis.Set(ctx, cacheKey(ui.ID), data, a.ttl).Err()
}
