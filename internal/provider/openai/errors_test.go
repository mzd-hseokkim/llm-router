package openai

import (
	"net/http"
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/provider"
)

func TestParseError(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		body           []byte
		header         http.Header
		wantCode       provider.GatewayErrorCode
		wantHTTP       int
		wantMsgContain string
		wantRetryAfter time.Duration
	}{
		{
			name:           "auth error",
			status:         401,
			body:           []byte(`{"error":{"message":"Invalid API key","type":"invalid_api_key","code":"invalid_api_key"}}`),
			wantCode:       provider.ErrAuthFailed,
			wantHTTP:       401,
			wantMsgContain: "Invalid API key",
		},
		{
			name:           "rate limited with retry-after",
			status:         429,
			body:           []byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`),
			header:         http.Header{"Retry-After": []string{"10"}},
			wantCode:       provider.ErrRateLimited,
			wantHTTP:       429,
			wantMsgContain: "Rate limit exceeded",
			wantRetryAfter: 10 * time.Second,
		},
		{
			name:           "context too long",
			status:         400,
			body:           []byte(`{"error":{"message":"This model's maximum context length is 4096 tokens","type":"invalid_request_error","code":"context_length_exceeded"}}`),
			wantCode:       provider.ErrInvalidRequest,
			wantHTTP:       400,
			wantMsgContain: "context_length_exceeded",
		},
		{
			name:           "server error 500",
			status:         500,
			body:           []byte(`{"error":{"message":"Internal server error","type":"server_error"}}`),
			wantCode:       provider.ErrProviderError,
			wantHTTP:       502,
			wantMsgContain: "Internal server error",
		},
		{
			name:           "overloaded 503",
			status:         503,
			body:           []byte(`{"error":{"message":"Service unavailable","type":"server_error"}}`),
			wantCode:       provider.ErrOverloaded,
			wantHTTP:       503,
			wantMsgContain: "Service unavailable",
		},
		{
			name:           "malformed body falls back to raw",
			status:         500,
			body:           []byte(`not json`),
			wantCode:       provider.ErrProviderError,
			wantHTTP:       502,
			wantMsgContain: "not json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.header == nil {
				tc.header = http.Header{}
			}
			err := ParseError(tc.status, tc.body, tc.header)
			if err.Code != tc.wantCode {
				t.Errorf("Code = %v, want %v", err.Code, tc.wantCode)
			}
			if err.HTTPStatus != tc.wantHTTP {
				t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, tc.wantHTTP)
			}
			if tc.wantMsgContain != "" && !contains(err.Message, tc.wantMsgContain) {
				t.Errorf("Message %q does not contain %q", err.Message, tc.wantMsgContain)
			}
			if tc.wantRetryAfter != 0 && err.RetryAfter != tc.wantRetryAfter {
				t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, tc.wantRetryAfter)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
