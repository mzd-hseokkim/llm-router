package gemini

import (
	"net/http"
	"testing"

	"github.com/llm-router/gateway/internal/provider"
)

func TestParseError(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		body           []byte
		wantCode       provider.GatewayErrorCode
		wantHTTP       int
		wantMsgContain string
	}{
		{
			name:     "resource exhausted → rate limited",
			status:   429,
			body:     []byte(`{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}`),
			wantCode: provider.ErrRateLimited,
			wantHTTP: 429,
			wantMsgContain: "Quota exceeded",
		},
		{
			name:     "unauthenticated",
			status:   401,
			body:     []byte(`{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`),
			wantCode: provider.ErrAuthFailed,
			wantHTTP: 401,
			wantMsgContain: "API key not valid",
		},
		{
			name:     "permission denied",
			status:   403,
			body:     []byte(`{"error":{"code":403,"message":"Permission denied","status":"PERMISSION_DENIED"}}`),
			wantCode: provider.ErrPermissionDenied,
			wantHTTP: 403,
			wantMsgContain: "Permission denied",
		},
		{
			name:     "invalid argument",
			status:   400,
			body:     []byte(`{"error":{"code":400,"message":"Invalid JSON payload","status":"INVALID_ARGUMENT"}}`),
			wantCode: provider.ErrInvalidRequest,
			wantHTTP: 400,
			wantMsgContain: "Invalid JSON payload",
		},
		{
			name:     "unavailable → overloaded",
			status:   503,
			body:     []byte(`{"error":{"code":503,"message":"Service unavailable","status":"UNAVAILABLE"}}`),
			wantCode: provider.ErrOverloaded,
			wantHTTP: 503,
			wantMsgContain: "Service unavailable",
		},
		{
			name:     "internal error",
			status:   500,
			body:     []byte(`{"error":{"code":500,"message":"Internal error","status":"INTERNAL"}}`),
			wantCode: provider.ErrProviderError,
			wantHTTP: 502,
			wantMsgContain: "Internal error",
		},
		{
			name:     "grpc status overrides http status",
			status:   200, // wrong HTTP status from proxy
			body:     []byte(`{"error":{"code":429,"message":"Rate limited","status":"RESOURCE_EXHAUSTED"}}`),
			wantCode: provider.ErrRateLimited,
			wantHTTP: 429,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ParseError(tc.status, tc.body, http.Header{})
			if err.Code != tc.wantCode {
				t.Errorf("Code = %v, want %v", err.Code, tc.wantCode)
			}
			if err.HTTPStatus != tc.wantHTTP {
				t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, tc.wantHTTP)
			}
			if tc.wantMsgContain != "" && !containsStr(err.Message, tc.wantMsgContain) {
				t.Errorf("Message %q does not contain %q", err.Message, tc.wantMsgContain)
			}
		})
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
