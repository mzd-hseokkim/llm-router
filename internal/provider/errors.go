package provider

import "fmt"

// GatewayError is a normalized error returned by provider adapters.
// HTTPStatus is the HTTP status code the gateway should return to the client.
type GatewayError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewRateLimitError wraps a provider 429 response.
func NewRateLimitError(msg string) *GatewayError {
	return &GatewayError{Code: "rate_limit_exceeded", Message: msg, HTTPStatus: 429}
}

// NewUnavailableError wraps a provider 5xx response.
func NewUnavailableError(msg string) *GatewayError {
	return &GatewayError{Code: "provider_unavailable", Message: msg, HTTPStatus: 502}
}

// NewAuthError wraps a provider 401/403 response.
func NewAuthError(msg string) *GatewayError {
	return &GatewayError{Code: "authentication_error", Message: msg, HTTPStatus: 401}
}

// NewInvalidRequestError wraps a provider 400 response.
func NewInvalidRequestError(msg string) *GatewayError {
	return &GatewayError{Code: "invalid_request_error", Message: msg, HTTPStatus: 400}
}

// NormalizeHTTPError maps a provider HTTP status to a GatewayError.
func NormalizeHTTPError(status int, body string) *GatewayError {
	switch {
	case status == 401 || status == 403:
		return NewAuthError(body)
	case status == 429:
		return NewRateLimitError(body)
	case status >= 500:
		return NewUnavailableError(body)
	default:
		return NewInvalidRequestError(body)
	}
}
