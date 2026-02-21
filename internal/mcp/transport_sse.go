package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type sseServer struct {
	cfg    ServerConfig
	logger *slog.Logger
	client *http.Client

	mu        sync.Mutex
	postURL   string
	sseCancel context.CancelFunc
	connected bool

	nextID  atomic.Int64
	pending sync.Map // int64 → chan Response
}

func newSSEServer(cfg ServerConfig) *sseServer {
	return &sseServer{
		cfg:    cfg,
		logger: slog.Default(),
		client: &http.Client{Timeout: 0},
	}
}

func (s *sseServer) Name() string { return s.cfg.Name }

func (s *sseServer) Connect(ctx context.Context) error {
	sseURL := strings.TrimRight(s.cfg.URL, "/") + "/sse"
	sseCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.sseCancel = cancel
	s.mu.Unlock()

	endpointCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go s.sseLoop(sseCtx, sseURL, endpointCh, errCh)

	select {
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	case err := <-errCh:
		cancel()
		return fmt.Errorf("sse: connect %s: %w", sseURL, err)
	case ep := <-endpointCh:
		s.mu.Lock()
		s.postURL = ep
		s.connected = true
		s.mu.Unlock()
	case <-time.After(15 * time.Second):
		cancel()
		return fmt.Errorf("sse: timeout waiting for endpoint from %s", sseURL)
	}

	initResult, err := s.call(ctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "llm-router", "version": "1.0.0"},
	})
	if err != nil {
		return fmt.Errorf("sse: initialize: %w", err)
	}
	s.logger.Info("mcp sse connected", "server", s.cfg.Name, "result", string(initResult))
	_ = s.postMessage(0, "notifications/initialized", nil)
	return nil
}

func (s *sseServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	if s.sseCancel != nil {
		s.sseCancel()
		s.sseCancel = nil
	}
	return nil
}

func (s *sseServer) Health(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected {
		return fmt.Errorf("not connected")
	}
	return nil
}

func (s *sseServer) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := s.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	return result.Tools, json.Unmarshal(raw, &result)
}

func (s *sseServer) CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	return s.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
}

func (s *sseServer) ListResources(ctx context.Context) ([]Resource, error) {
	raw, err := s.call(ctx, "resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Resources []Resource `json:"resources"`
	}
	return result.Resources, json.Unmarshal(raw, &result)
}

func (s *sseServer) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	return s.call(ctx, "resources/read", map[string]any{"uri": uri})
}

func (s *sseServer) ListPrompts(ctx context.Context) ([]Prompt, error) {
	raw, err := s.call(ctx, "prompts/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Prompts []Prompt `json:"prompts"`
	}
	return result.Prompts, json.Unmarshal(raw, &result)
}

func (s *sseServer) GetPrompt(ctx context.Context, name string, args map[string]string) (json.RawMessage, error) {
	return s.call(ctx, "prompts/get", map[string]any{"name": name, "arguments": args})
}

func (s *sseServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	ch := make(chan Response, 1)
	s.pending.Store(id, ch)
	defer s.pending.Delete(id)

	if err := s.postMessage(id, method, params); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout: %s", method)
	}
}

func (s *sseServer) postMessage(id int64, method string, params any) error {
	p, _ := json.Marshal(params)
	var req Request
	if id != 0 {
		req = Request{JSONRPC: "2.0", ID: &id, Method: method, Params: p}
	} else {
		req = Request{JSONRPC: "2.0", Method: method, Params: p}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}

	s.mu.Lock()
	postURL := s.postURL
	s.mu.Unlock()

	httpReq, err := http.NewRequest(http.MethodPost, postURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	s.applyAuth(httpReq)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sse: post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sse: status %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (s *sseServer) applyAuth(req *http.Request) {
	if s.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
		return
	}
	switch s.cfg.Auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+s.cfg.Auth.Token)
	case "basic":
		req.SetBasicAuth(s.cfg.Auth.User, s.cfg.Auth.Token)
	}
}

func (s *sseServer) sseLoop(ctx context.Context, url string, endpointCh chan string, errCh chan error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		errCh <- err
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	s.applyAuth(req)

	resp, err := s.client.Do(req)
	if err != nil {
		errCh <- err
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		errCh <- fmt.Errorf("status %d: %s", resp.StatusCode, body)
		return
	}

	endpointSent := false
	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			switch eventType {
			case "endpoint":
				if !endpointSent {
					endpointSent = true
					endpointCh <- data
				}
			case "message":
				var r Response
				if err := json.Unmarshal([]byte(data), &r); err == nil && r.ID != nil {
					if ch, ok := s.pending.Load(*r.ID); ok {
						ch.(chan Response) <- r
					}
				}
			}
			eventType, data = "", ""
		}
	}
}
