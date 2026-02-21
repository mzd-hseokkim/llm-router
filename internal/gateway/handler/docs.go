package handler

import (
	"net/http"
)

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>LLM Router API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: "/docs/openapi.json",
  dom_id: '#swagger-ui',
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
  layout: "BaseLayout",
  deepLinking: true,
  persistAuthorization: true,
})
</script>
</body>
</html>`

// DocsHandler serves the Swagger UI and public OpenAPI spec.
type DocsHandler struct {
	openapi *AdminOpenAPIHandler
}

// NewDocsHandler creates a DocsHandler.
func NewDocsHandler() *DocsHandler {
	return &DocsHandler{openapi: NewAdminOpenAPIHandler()}
}

// UI handles GET /docs — serves the Swagger UI HTML.
func (h *DocsHandler) UI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

// JSON handles GET /docs/openapi.json — public OpenAPI JSON spec.
func (h *DocsHandler) JSON(w http.ResponseWriter, r *http.Request) {
	h.openapi.SpecJSON(w, r)
}

// YAML handles GET /docs/openapi.yaml — public OpenAPI YAML spec.
func (h *DocsHandler) YAML(w http.ResponseWriter, r *http.Request) {
	h.openapi.SpecYAML(w, r)
}
