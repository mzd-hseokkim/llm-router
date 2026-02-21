package rbac

// Permission is a fine-grained capability string.
type Permission string

const (
	// Virtual key operations
	PermCreateKey Permission = "keys:create"
	PermReadKey   Permission = "keys:read"
	PermUpdateKey Permission = "keys:update"
	PermDeleteKey Permission = "keys:delete"

	// Team and user management
	PermManageTeam  Permission = "teams:manage"
	PermManageUsers Permission = "users:manage"

	// Usage and budget
	PermReadUsage Permission = "usage:read"
	PermSetBudget Permission = "budget:set"

	// Provider and model management (super_admin only)
	PermManageProviders Permission = "providers:manage"
	PermManageModels    Permission = "models:manage"

	// System-wide
	PermManageSystem Permission = "system:manage"

	// Wildcard — all permissions (super_admin)
	PermAll Permission = "*"
)

// All returns all defined permissions (excluding wildcard).
func All() []Permission {
	return []Permission{
		PermCreateKey, PermReadKey, PermUpdateKey, PermDeleteKey,
		PermManageTeam, PermManageUsers,
		PermReadUsage, PermSetBudget,
		PermManageProviders, PermManageModels,
		PermManageSystem,
	}
}
