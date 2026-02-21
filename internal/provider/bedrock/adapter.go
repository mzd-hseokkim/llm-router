// Package bedrock implements a provider adapter for AWS Bedrock Converse API.
// Authentication uses AWS Signature Version 4 (implemented without the AWS SDK).
package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

const bedrockBaseURL = "https://bedrock-runtime.%s.amazonaws.com"

// AuthConfig holds AWS credentials for Bedrock.
type AuthConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string // optional; for temporary credentials
}

// Config holds Bedrock provider configuration.
type Config struct {
	Region string
	Auth   AuthConfig
}

// Adapter implements provider.Provider for AWS Bedrock.
type Adapter struct {
	region   string
	auth     AuthConfig
	baseURL  string
	client   *http.Client
	mu       sync.RWMutex
	dbModels []types.ModelInfo
}

// New returns a Bedrock Adapter with the given config.
func New(cfg Config) *Adapter {
	baseURL := fmt.Sprintf(bedrockBaseURL, cfg.Region)
	return &Adapter{
		region:  cfg.Region,
		auth:    cfg.Auth,
		baseURL: baseURL,
		client:  newHTTPClient(),
	}
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   100,
			ForceAttemptHTTP2:     true,
		},
	}
}

func (a *Adapter) Name() string { return "bedrock" }

// SetModels injects a DB-sourced model list, overriding the hardcoded default.
func (a *Adapter) SetModels(models []types.ModelInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dbModels = models
}

func (a *Adapter) Models() []types.ModelInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.dbModels != nil {
		return a.dbModels
	}
	return []types.ModelInfo{
		{ID: "bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0", Object: "model", OwnedBy: "bedrock"},
		{ID: "bedrock/anthropic.claude-3-5-haiku-20241022-v1:0", Object: "model", OwnedBy: "bedrock"},
		{ID: "bedrock/amazon.nova-pro-v1:0", Object: "model", OwnedBy: "bedrock"},
		{ID: "bedrock/amazon.nova-lite-v1:0", Object: "model", OwnedBy: "bedrock"},
		{ID: "bedrock/meta.llama3-70b-instruct-v1:0", Object: "model", OwnedBy: "bedrock"},
	}
}

// ChatCompletion calls the Bedrock Converse API.
// model is the Bedrock model ID (the part after "bedrock/").
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	body, err := BuildConverseRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build bedrock request: %w", err)
	}

	url := fmt.Sprintf("%s/model/%s/converse", a.baseURL, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create bedrock request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Host = httpReq.URL.Host

	signRequest(httpReq, a.region, "bedrock", a.auth.AccessKeyID, a.auth.SecretAccessKey, a.auth.SessionToken, time.Now(), body)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bedrock response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, parseBedrockError(resp.StatusCode, respBody, resp.Header)
	}

	return ParseConverseResponse("bedrock/"+model, respBody)
}

// ChatCompletionStream calls the Bedrock ConverseStream API.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	body, err := BuildConverseRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build bedrock stream request: %w", err)
	}

	url := fmt.Sprintf("%s/model/%s/converse-stream", a.baseURL, model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create bedrock stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Host = httpReq.URL.Host

	signRequest(httpReq, a.region, "bedrock", a.auth.AccessKeyID, a.auth.SecretAccessKey, a.auth.SessionToken, time.Now(), body)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, parseBedrockError(resp.StatusCode, body, resp.Header)
	}

	return streamBedrockEvents(ctx, resp.Body, "bedrock/"+model), nil
}

// streamBedrockEvents reads AWS event stream format from Bedrock ConverseStream.
// Bedrock uses a binary event stream protocol, but the JSON content inside is parseable.
func streamBedrockEvents(ctx context.Context, body io.ReadCloser, model string) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 16)
	go func() {
		defer close(ch)
		defer body.Close()

		buf := make([]byte, 8192)
		var leftover []byte

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := body.Read(buf)
			if n > 0 {
				leftover = append(leftover, buf[:n]...)
				leftover = parseBedrockStreamChunks(ctx, ch, leftover)
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

// parseBedrockStreamChunks extracts JSON event objects from the binary AWS event stream.
// The event stream format is: 4-byte total length, 4-byte header length, 4-byte CRC, headers, payload, 4-byte CRC.
// We extract JSON payloads by scanning for the `:event-type` header and the JSON payload.
func parseBedrockStreamChunks(ctx context.Context, ch chan<- provider.StreamChunk, buf []byte) []byte {
	for {
		// Look for JSON objects in the buffer (simplified approach).
		// AWS event stream binary frames contain JSON payloads.
		start := bytes.IndexByte(buf, '{')
		if start < 0 {
			return nil // no JSON yet
		}

		// Find matching end of JSON object.
		end := findJSONEnd(buf[start:])
		if end < 0 {
			return buf // incomplete JSON
		}

		jsonData := buf[start : start+end+1]
		buf = buf[start+end+1:]

		var event converseStreamEvent
		if err := json.Unmarshal(jsonData, &event); err != nil {
			continue
		}

		sc := provider.StreamChunk{}
		handled := false

		if event.ContentBlockDelta != nil {
			sc.Delta = event.ContentBlockDelta.Delta.Text
			handled = true
		}
		if event.MessageStop != nil {
			fr := mapStopReason(event.MessageStop.StopReason)
			sc.FinishReason = &fr
			handled = true
		}
		if event.Metadata != nil {
			sc.Usage = &types.Usage{
				PromptTokens:     event.Metadata.Usage.InputTokens,
				CompletionTokens: event.Metadata.Usage.OutputTokens,
				TotalTokens:      event.Metadata.Usage.TotalTokens,
			}
			handled = true
		}

		if !handled {
			continue
		}

		select {
		case <-ctx.Done():
			return buf
		case ch <- sc:
		}
	}
}

// findJSONEnd finds the index of the closing brace of the first JSON object in buf.
// Returns -1 if the object is incomplete.
func findJSONEnd(buf []byte) int {
	depth := 0
	inString := false
	escaped := false

	for i, b := range buf {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if b == '{' {
			depth++
		} else if b == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseBedrockError converts a Bedrock error response to a GatewayError.
func parseBedrockError(status int, body []byte, header http.Header) error {
	var errResp struct {
		Message string `json:"message"`
		Code    string `json:"__type"`
	}
	msg := string(body)
	if err := json.Unmarshal(body, &errResp); err == nil {
		if errResp.Message != "" {
			msg = errResp.Message
		}
		// Map AWS error types.
		if strings.Contains(errResp.Code, "ThrottlingException") {
			return provider.NormalizeHTTPError(429, msg, header)
		}
	}
	return provider.NormalizeHTTPError(status, msg, header)
}
