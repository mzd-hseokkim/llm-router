package audit

// Event type constants — {resource_type}.{action} convention.
const (
	// Auth
	EventAuthLoginSuccess = "auth.login_success"
	EventAuthLoginFailed  = "auth.login_failed"
	EventAuthLogout       = "auth.logout"
	EventAuthKeyDenied    = "auth.key_denied"
	EventAuthPermDenied   = "auth.permission_denied"

	// Virtual Key management
	EventKeyCreated     = "virtual_key.created"
	EventKeyUpdated     = "virtual_key.updated"
	EventKeyDeleted     = "virtual_key.deleted"
	EventKeyDeactivated = "virtual_key.deactivated"
	EventKeyRotated     = "virtual_key.rotated"

	// Provider Key management
	EventProviderKeyCreated = "provider_key.created"
	EventProviderKeyUpdated = "provider_key.updated"
	EventProviderKeyDeleted = "provider_key.deleted"
	EventProviderKeyRotated = "provider_key.rotated"

	// Routing
	EventRoutingRuleCreated = "routing_rule.created"
	EventRoutingRuleUpdated = "routing_rule.updated"
	EventRoutingRuleDeleted = "routing_rule.deleted"
	EventRoutingReloaded    = "routing.reloaded"

	// User / Team / Org
	EventUserCreated      = "user.created"
	EventUserUpdated      = "user.updated"
	EventUserDeleted      = "user.deleted"
	EventTeamMemberAdded  = "team.member_added"
	EventTeamMemberRemoved = "team.member_removed"
	EventRoleAssigned     = "user.role_assigned"
	EventRoleRevoked      = "user.role_revoked"

	// Budget
	EventBudgetCreated  = "budget.created"
	EventBudgetReset    = "budget.reset"
	EventBudgetExceeded = "budget.exceeded"

	// System
	EventCircuitBreakerReset = "circuit_breaker.reset"
	EventCacheDeleted        = "cache.deleted"
	EventConfigReloaded      = "config.reloaded"

	// Security / Guardrails
	EventGuardrailPII        = "guardrail.pii_detected"
	EventGuardrailInjection  = "guardrail.injection_detected"
	EventGuardrailBlocked    = "guardrail.request_blocked"
)

// Actor types.
const (
	ActorUser   = "user"
	ActorAPIKey = "api_key"
	ActorSystem = "system"
)
