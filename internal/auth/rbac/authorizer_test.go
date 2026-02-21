package rbac_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth/rbac"
)

func TestUserInfo_HasPermission(t *testing.T) {
	orgID := uuid.New().String()

	tests := []struct {
		name     string
		ui       rbac.UserInfo
		perm     rbac.Permission
		orgID    string
		teamID   string
		expected bool
	}{
		{
			name: "super_admin has all permissions via wildcard",
			ui: rbac.UserInfo{
				Roles: []rbac.UserRole{{Role: rbac.RoleSuperAdmin, Permissions: []rbac.Permission{rbac.PermAll}}},
			},
			perm:     rbac.PermManageSystem,
			expected: true,
		},
		{
			name: "developer cannot manage providers",
			ui: rbac.UserInfo{
				Roles: []rbac.UserRole{{Role: rbac.RoleDeveloper, Permissions: []rbac.Permission{rbac.PermReadKey, rbac.PermReadUsage}}},
			},
			perm:     rbac.PermManageProviders,
			expected: false,
		},
		{
			name: "org_admin in correct org can create keys",
			ui: rbac.UserInfo{
				Roles: []rbac.UserRole{
					{Role: rbac.RoleOrgAdmin, OrgID: orgID, Permissions: rbac.DefaultRolePermissions[rbac.RoleOrgAdmin]},
				},
			},
			perm:     rbac.PermCreateKey,
			orgID:    orgID,
			expected: true,
		},
		{
			name: "org_admin in wrong org is denied",
			ui: rbac.UserInfo{
				Roles: []rbac.UserRole{
					{Role: rbac.RoleOrgAdmin, OrgID: orgID, Permissions: rbac.DefaultRolePermissions[rbac.RoleOrgAdmin]},
				},
			},
			perm:     rbac.PermCreateKey,
			orgID:    uuid.New().String(), // different org
			expected: false,
		},
		{
			name: "viewer can only read usage",
			ui: rbac.UserInfo{
				Roles: []rbac.UserRole{{Role: rbac.RoleViewer, Permissions: []rbac.Permission{rbac.PermReadUsage}}},
			},
			perm:     rbac.PermReadUsage,
			expected: true,
		},
		{
			name: "viewer cannot set budget",
			ui: rbac.UserInfo{
				Roles: []rbac.UserRole{{Role: rbac.RoleViewer, Permissions: []rbac.Permission{rbac.PermReadUsage}}},
			},
			perm:     rbac.PermSetBudget,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ui.HasPermission(tc.perm, tc.orgID, tc.teamID)
			if got != tc.expected {
				t.Errorf("HasPermission(%q, %q, %q) = %v; want %v", tc.perm, tc.orgID, tc.teamID, got, tc.expected)
			}
		})
	}
}

func TestDefaultRolePermissions(t *testing.T) {
	// team_admin should NOT have budget:set
	perms := rbac.DefaultRolePermissions[rbac.RoleTeamAdmin]
	for _, p := range perms {
		if p == rbac.PermSetBudget {
			t.Error("team_admin should not have budget:set permission")
		}
	}

	// developer should not have keys:create
	perms = rbac.DefaultRolePermissions[rbac.RoleDeveloper]
	for _, p := range perms {
		if p == rbac.PermCreateKey {
			t.Error("developer should not have keys:create permission")
		}
	}
}
