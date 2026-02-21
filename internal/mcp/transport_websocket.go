package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type wsServer struct {
	cfg    ServerConfig
	logger *slog.Logger

	mu        sync.Mutex
	conn      *wsConn
	connected bool

	nextID  atomic.Int64
	pending sync.Map // int64 → chan Response
}

func newWSServer(cfg ServerConfig) *wsServer {
	return &wsServer{cfg: cfg, logger: slog.Default()}
}

func (s *wsServer) Name() string { return s.cfg.Name }

func (s *wsServer) Connect(ctx context.Context) error {
	conn, err := dialWS(ctx, s.cfg.URL, s.authHeaders())
	if err != nil {
		return fmt.Errorf("websocket: dial %s: %w", s.cfg.URL, err)
	}
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
		return s.conn.close()
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
	return conn.write(b)
}

func (s *wsServer) readLoop() {
	for {
		msg, err := s.conn.read()
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

// --- minimal WebSocket client ---

type wsConn struct {
	conn net.Conn
	r    *bufio.Reader
	mu   sync.Mutex
}

func dialWS(ctx context.Context, rawURL string, headers http.Header) (*wsConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "wss" {
		return nil, fmt.Errorf("wss not supported; use ws://")
	}

	host := u.Host
	if u.Port() == "" {
		host += ":80"
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}

	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	reqStr := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n",
		path, u.Host)
	for k, vs := range headers {
		for _, v := range vs {
			reqStr += k + ": " + v + "\r\n"
		}
	}
	reqStr += "\r\n"

	if _, err := conn.Write([]byte(reqStr)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	r := bufio.NewReader(conn)
	statusLine, _ := r.ReadString('\n')
	if !strings.Contains(statusLine, "101") {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected status: %s", strings.TrimSpace(statusLine))
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}
	return &wsConn{conn: conn, r: r}, nil
}

func (c *wsConn) write(payload []byte) error {
	frame := wsFrame(payload)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.conn.Write(frame)
	return err
}

func (c *wsConn) read() ([]byte, error) {
	return wsReadFrame(c.r)
}

func (c *wsConn) close() error {
	return c.conn.Close()
}

func wsFrame(payload []byte) []byte {
	n := len(payload)
	var f []byte
	f = append(f, 0x81) // FIN + text
	switch {
	case n < 126:
		f = append(f, byte(n))
	case n < 65536:
		f = append(f, 126, byte(n>>8), byte(n))
	default:
		f = append(f, 127)
		for i := 7; i >= 0; i-- {
			f = append(f, byte(n>>(uint(i)*8)))
		}
	}
	return append(f, payload...)
}

func wsReadFrame(r *bufio.Reader) ([]byte, error) {
	_, err := r.ReadByte() // b0: FIN+opcode
	if err != nil {
		return nil, err
	}
	b1, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	masked := b1&0x80 != 0
	payloadLen := int64(b1 & 0x7f)
	switch payloadLen {
	case 126:
		var buf [2]byte
		if _, err := r.Read(buf[:]); err != nil {
			return nil, err
		}
		payloadLen = int64(buf[0])<<8 | int64(buf[1])
	case 127:
		var buf [8]byte
		if _, err := r.Read(buf[:]); err != nil {
			return nil, err
		}
		payloadLen = 0
		for _, bv := range buf {
			payloadLen = (payloadLen << 8) | int64(bv)
		}
	}
	var maskKey [4]byte
	if masked {
		if _, err := r.Read(maskKey[:]); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, payloadLen)
	if _, err := r.Read(payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return payload, nil
}
