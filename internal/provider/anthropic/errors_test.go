package anthropic

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
			name:           "auth error",
			status:         401,
			body:           []byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`),
			wantCode:       provider.ErrAuthFailed,
			wantHTTP:       401,
			wantMsgContain: "invalid x-api-key",
		},
		{
			name:           "overloaded 529",
			status:         529,
			body:           []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`),
			wantCode:       provider.ErrOverloaded,
			wantHTTP:       503,
			wantMsgContain: "Overloaded",
		},
		{
			name:           "rate limited",
			status:         429,
			body:           []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`),
			wantCode:       provider.ErrRateLimited,
			wantHTTP:       429,
			wantMsgContain: "Rate limit exceeded",
		},
		{
			name:           "invalid request",
			status:         400,
			body:           []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens too large"}}`),
			wantCode:       provider.ErrInvalidRequest,
			wantHTTP:       400,
			wantMsgContain: "max_tokens too large",
		},
		{
			name:           "server error",
			status:         500,
			body:           []byte(`{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`),
			wantCode:       provider.ErrProviderError,
			wantHTTP:       502,
			wantMsgContain: "Internal server error",
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
