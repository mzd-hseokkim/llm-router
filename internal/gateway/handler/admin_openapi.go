package handler

import (
	"net/http"
)

// AdminOpenAPIHandler serves the OpenAPI specification for the Admin API.
type AdminOpenAPIHandler struct{}

// NewAdminOpenAPIHandler creates the handler.
func NewAdminOpenAPIHandler() *AdminOpenAPIHandler { return &AdminOpenAPIHandler{} }

// Spec handles GET /admin/openapi.json.
func (h *AdminOpenAPIHandler) Spec(w http.ResponseWriter, r *http.Request) {
	spec := buildOpenAPISpec()
	writeJSON(w, http.StatusOK, spec)
}

// buildOpenAPISpec constructs an OpenAPI 3.0 document for the Admin API.
func buildOpenAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "LLM Router Admin API",
			"version": "1.0.0",
		},
		"security": []map[string]any{
			{"bearerAuth": []string{}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "master_key",
				},
			},
		},
		"paths": map[string]any{
			// Virtual Keys
			"/admin/keys": map[string]any{
				"get":  opSummary("List virtual keys"),
				"post": opSummary("Create a virtual key"),
			},
			"/admin/keys/{id}": map[string]any{
				"get":    opSummary("Get a virtual key"),
				"patch":  opSummary("Update a virtual key"),
				"delete": opSummary("Deactivate a virtual key"),
			},
			"/admin/keys/{id}/regenerate": map[string]any{
				"post": opSummary("Regenerate (rotate) a virtual key"),
			},
			// Provider Keys
			"/admin/provider-keys": map[string]any{
				"get":  opSummary("List provider keys"),
				"post": opSummary("Create a provider key"),
			},
			"/admin/provider-keys/{id}": map[string]any{
				"get":    opSummary("Get a provider key"),
				"put":    opSummary("Update a provider key"),
				"delete": opSummary("Delete a provider key"),
			},
			"/admin/provider-keys/{id}/rotate": map[string]any{
				"put": opSummary("Rotate a provider key"),
			},
			// Organizations
			"/admin/organizations": map[string]any{
				"get":  opSummary("List organizations"),
				"post": opSummary("Create an organization"),
			},
			"/admin/organizations/{id}": map[string]any{
				"get": opSummary("Get an organization"),
				"put": opSummary("Update an organization"),
			},
			// Teams
			"/admin/teams": map[string]any{
				"get":  opSummary("List teams"),
				"post": opSummary("Create a team"),
			},
			"/admin/teams/{id}": map[string]any{
				"get": opSummary("Get a team"),
				"put": opSummary("Update a team"),
			},
			// Users
			"/admin/users": map[string]any{
				"get":  opSummary("List users"),
				"post": opSummary("Create a user"),
			},
			"/admin/users/{id}": map[string]any{
				"get": opSummary("Get a user"),
				"put": opSummary("Update a user"),
			},
			// Usage
			"/admin/usage/summary": map[string]any{
				"get": opSummary("Get usage summary for an entity"),
			},
			"/admin/usage/top-spenders": map[string]any{
				"get": opSummary("List top-spending virtual keys"),
			},
			// Logs
			"/admin/logs": map[string]any{
				"get": opSummary("List request logs"),
			},
			"/admin/logs/{request_id}": map[string]any{
				"get": opSummary("Get a single request log"),
			},
			// Budgets
			"/admin/budgets": map[string]any{
				"post": opSummary("Create a budget"),
			},
			"/admin/budgets/{entity_type}/{entity_id}": map[string]any{
				"get": opSummary("List budgets for an entity"),
			},
			"/admin/budgets/{id}/reset": map[string]any{
				"post": opSummary("Reset a budget period"),
			},
			// Rate limits
			"/admin/rate-limits/{id}": map[string]any{
				"get": opSummary("Get rate limit status for a key"),
			},
			"/admin/rate-limits/{id}/reset": map[string]any{
				"post": opSummary("Reset rate limit counters for a key"),
			},
			// Circuit breakers
			"/admin/circuit-breakers": map[string]any{
				"get": opSummary("List circuit breaker states"),
			},
			"/admin/circuit-breakers/{provider}/reset": map[string]any{
				"post": opSummary("Reset a circuit breaker"),
			},
			// Routing
			"/admin/routing": map[string]any{
				"get": opSummary("Get current routing configuration"),
				"put": opSummary("Update routing configuration"),
			},
			"/admin/routing/reload": map[string]any{
				"post": opSummary("Trigger hot reload of routing config"),
			},
		},
	}
}

func opSummary(summary string) map[string]any {
	return map[string]any{"summary": summary}
}
