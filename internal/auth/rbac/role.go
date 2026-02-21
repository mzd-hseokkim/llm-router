package rbac

// Role is a named set of permissions.
type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleOrgAdmin   Role = "org_admin"
	RoleTeamAdmin  Role = "team_admin"
	RoleDeveloper  Role = "developer"
	RoleViewer     Role = "viewer"
)

// DefaultRolePermissions defines the built-in permission sets for system roles.
var DefaultRolePermissions = map[Role][]Permission{
	RoleSuperAdmin: {PermAll},
	RoleOrgAdmin: {
		PermCreateKey, PermReadKey, PermUpdateKey, PermDeleteKey,
		PermManageTeam, PermManageUsers,
		PermReadUsage, PermSetBudget,
	},
	RoleTeamAdmin: {
		PermCreateKey, PermReadKey, PermUpdateKey, PermDeleteKey,
		PermManageTeam,
		PermReadUsage,
	},
	RoleDeveloper: {PermReadKey, PermReadUsage},
	RoleViewer:    {PermReadUsage},
}

// UserRole associates a role with optional org/team scope.
type UserRole struct {
	Role   Role
	OrgID  string // empty = global
	TeamID string // empty = org-wide
	// Permissions are either loaded from DefaultRolePermissions or DB custom roles
	Permissions []Permission
}

// HasPermission checks if this role assignment grants the given permission.
func (ur UserRole) HasPermission(p Permission) bool {
	for _, perm := range ur.Permissions {
		if perm == PermAll || perm == p {
			return true
		}
	}
	return false
}
