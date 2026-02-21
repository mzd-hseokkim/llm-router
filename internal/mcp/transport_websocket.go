package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// maxWSMessageBytes limits incoming WebSocket message size to 4 MB.
const maxWSMessageBytes = 4 << 20

type wsServer struct {
	cfg    ServerConfig
	logger *slog.Logger

	mu        sync.Mutex
	conn      *websocket.Conn
	connected bool

	nextID  atomic.Int64
	pending sync.Map // int64 → chan Response
}

func newWSServer(cfg ServerConfig) *wsServer {
	return &wsServer{cfg: cfg, logger: slog.Default()}
}

func (s *wsServer) Name() string { return s.cfg.Name }

func (s *wsServer) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, s.cfg.URL, s.authHeaders())
	if err != nil {
		return fmt.Errorf("websocket: dial %s: %w", s.cfg.URL, err)
	}
	conn.SetReadLimit(maxWSMessageBytes)

	s.mu.Lock()
	s.conn = conn
	s.connected = true
	s.mu.Unlock()

	go s.readLoop()

	initResult, err := s.call(ctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "llm-router", "version": "1.0.0"},
	})
	if err != nil {
		_ = s.Close()
		return fmt.Errorf("websocket: initialize: %w", err)
	}
	s.logger.Info("mcp websocket connected", "server", s.cfg.Name, "result", string(initResult))
	_ = s.wsSend(0, "notifications/initialized", nil)
	return nil
}

func (s *wsServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	if s.conn != nil {
		err := s.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = err
		return s.conn.Close()
	}
	return nil
}

func (s *wsServer) Health(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected {
		return fmt.Errorf("not connected")
	}
	return nil
}

func (s *wsServer) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := s.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	return result.Tools, json.Unmarshal(raw, &result)
}

func (s *wsServer) CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	return s.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
}

func (s *wsServer) ListResources(ctx context.Context) ([]Resource, error) {
	raw, err := s.call(ctx, "resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Resources []Resource `json:"resources"`
	}
	return result.Resources, json.Unmarshal(raw, &result)
}

func (s *wsServer) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	return s.call(ctx, "resources/read", map[string]any{"uri": uri})
}

func (s *wsServer) ListPrompts(ctx context.Context) ([]Prompt, error) {
	raw, err := s.call(ctx, "prompts/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Prompts []Prompt `json:"prompts"`
	}
	return result.Prompts, json.Unmarshal(raw, &result)
}

func (s *wsServer) GetPrompt(ctx context.Context, name string, args map[string]string) (json.RawMessage, error) {
	return s.call(ctx, "prompts/get", map[string]any{"name": name, "arguments": args})
}

func (s *wsServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	ch := make(chan Response, 1)
	s.pending.Store(id, ch)
	defer s.pending.Delete(id)

	if err := s.wsSend(id, method, params); err != nil {
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

func (s *wsServer) wsSend(id int64, method string, params any) error {
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
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("websocket: not connected")
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

func (s *wsServer) readLoop() {
	for {
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			s.mu.Lock()
			s.connected = false
			s.mu.Unlock()
			return
		}
		var resp Response
		if err := json.Unmarshal(msg, &resp); err != nil || resp.ID == nil {
			continue
		}
		if ch, ok := s.pending.Load(*resp.ID); ok {
			ch.(chan Response) <- resp
		}
	}
}

func (s *wsServer) authHeaders() http.Header {
	h := make(http.Header)
	if s.cfg.APIKey != "" {
		h.Set("Authorization", "Bearer "+s.cfg.APIKey)
		return h
	}
	switch s.cfg.Auth.Type {
	case "bearer":
		h.Set("Authorization", "Bearer "+s.cfg.Auth.Token)
	case "basic":
		req := &http.Request{Header: make(http.Header)}
		req.SetBasicAuth(s.cfg.Auth.User, s.cfg.Auth.Token)
		h.Set("Authorization", req.Header.Get("Authorization"))
	}
	return h
}
