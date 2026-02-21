package provider

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// GatewayErrorCode is a categorized error code returned by the gateway.
type GatewayErrorCode string

const (
	// Non-retryable client errors (4xx)
	ErrInvalidRequest   GatewayErrorCode = "invalid_request_error"
	ErrAuthFailed       GatewayErrorCode = "authentication_error"
	ErrPermissionDenied GatewayErrorCode = "permission_error"
	ErrModelNotFound    GatewayErrorCode = "model_not_found"
	ErrContextTooLong   GatewayErrorCode = "context_length_exceeded"

	// Retryable errors
	ErrRateLimited   GatewayErrorCode = "rate_limit_error"
	ErrProviderError GatewayErrorCode = "provider_error"
	ErrTimeout       GatewayErrorCode = "timeout_error"
	ErrNetworkError  GatewayErrorCode = "network_error"
	ErrOverloaded    GatewayErrorCode = "overloaded_error"
)

// GatewayError is a normalized error returned by provider adapters.
// HTTPStatus is the HTTP status code the gateway should return to the client.
type GatewayError struct {
	Code       GatewayErrorCode
	Message    string
	HTTPStatus int
	RetryAfter time.Duration // non-zero if provider supplied Retry-After header
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// IsRetryable reports whether this error warrants a retry attempt.
func (e *GatewayError) IsRetryable() bool {
	switch e.Code {
	case ErrRateLimited, ErrProviderError, ErrTimeout, ErrNetworkError, ErrOverloaded:
		return true
	}
	return false
}

// NewNetworkError creates a GatewayError for connection/transport failures.
func NewNetworkError(msg string) *GatewayError {
	return &GatewayError{Code: ErrNetworkError, Message: msg, HTTPStatus: 503}
}

// ParseRetryAfterHeader parses a Retry-After header value (integer seconds).
// Returns 0 if the header is absent or unparseable.
func ParseRetryAfterHeader(header http.Header) time.Duration {
	v := header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second))
	}
	return 0
}

// NormalizeHTTPError maps a provider HTTP status code and message to a GatewayError.
// header is used to extract Retry-After for rate-limited and server-error responses.
func NormalizeHTTPError(status int, msg string, header http.Header) *GatewayError {
	retryAfter := ParseRetryAfterHeader(header)
	switch {
	case status == 400:
		return &GatewayError{Code: ErrInvalidRequest, Message: msg, HTTPStatus: 400}
	case status == 401:
		return &GatewayError{Code: ErrAuthFailed, Message: msg, HTTPStatus: 401}
	case status == 403:
		return &GatewayError{Code: ErrPermissionDenied, Message: msg, HTTPStatus: 403}
	case status == 404:
		return &GatewayError{Code: ErrModelNotFound, Message: msg, HTTPStatus: 404}
	case status == 413 || status == 422:
		return &GatewayError{Code: ErrInvalidRequest, Message: msg, HTTPStatus: status}
	case status == 429:
		return &GatewayError{Code: ErrRateLimited, Message: msg, HTTPStatus: 429, RetryAfter: retryAfter}
	case status == 503:
		return &GatewayError{Code: ErrOverloaded, Message: msg, HTTPStatus: 503, RetryAfter: retryAfter}
	case status == 504:
		return &GatewayError{Code: ErrTimeout, Message: msg, HTTPStatus: 504, RetryAfter: retryAfter}
	case status >= 500:
		return &GatewayError{Code: ErrProviderError, Message: msg, HTTPStatus: 502, RetryAfter: retryAfter}
	default:
		return &GatewayError{Code: ErrInvalidRequest, Message: msg, HTTPStatus: status}
	}
}
