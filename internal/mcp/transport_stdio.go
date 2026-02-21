package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type stdioServer struct {
	cfg    ServerConfig
	logger *slog.Logger

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner

	nextID  atomic.Int64
	pending sync.Map // int64 → chan Response
}

func newStdioServer(cfg ServerConfig) *stdioServer {
	return &stdioServer{cfg: cfg, logger: slog.Default()}
}

func (s *stdioServer) Name() string { return s.cfg.Name }

func (s *stdioServer) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd := exec.CommandContext(ctx, s.cfg.Command, s.cfg.Args...)
	for k, v := range s.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdio: StdinPipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdio: StdoutPipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("stdio: start: %w", err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.scanner = bufio.NewScanner(stdout)
	s.scanner.Buffer(make([]byte, 4<<20), 4<<20)

	go s.readLoop()

	initResult, err := s.call(ctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "llm-router", "version": "1.0.0"},
	})
	if err != nil {
		_ = s.stopLocked()
		return fmt.Errorf("stdio: initialize: %w", err)
	}
	s.logger.Info("mcp stdio connected", "server", s.cfg.Name, "result", string(initResult))
	_ = s.notifyLocked("notifications/initialized", nil)
	return nil
}

func (s *stdioServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked()
}

func (s *stdioServer) stopLocked() error {
	if s.cmd == nil {
		return nil
	}
	_ = s.stdin.Close()
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.cmd = nil
	return nil
}

func (s *stdioServer) Health(_ context.Context) error {
	s.mu.Lock()
	alive := s.cmd != nil
	s.mu.Unlock()
	if !alive {
		return fmt.Errorf("stdio process not running")
	}
	return nil
}

func (s *stdioServer) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := s.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	return result.Tools, json.Unmarshal(raw, &result)
}

func (s *stdioServer) CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	return s.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
}

func (s *stdioServer) ListResources(ctx context.Context) ([]Resource, error) {
	raw, err := s.call(ctx, "resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Resources []Resource `json:"resources"`
	}
	return result.Resources, json.Unmarshal(raw, &result)
}

func (s *stdioServer) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	return s.call(ctx, "resources/read", map[string]any{"uri": uri})
}

func (s *stdioServer) ListPrompts(ctx context.Context) ([]Prompt, error) {
	raw, err := s.call(ctx, "prompts/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Prompts []Prompt `json:"prompts"`
	}
	return result.Prompts, json.Unmarshal(raw, &result)
}

func (s *stdioServer) GetPrompt(ctx context.Context, name string, args map[string]string) (json.RawMessage, error) {
	return s.call(ctx, "prompts/get", map[string]any{"name": name, "arguments": args})
}

func (s *stdioServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	ch := make(chan Response, 1)
	s.pending.Store(id, ch)
	defer s.pending.Delete(id)

	if err := s.sendLocked(id, method, params); err != nil {
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

func (s *stdioServer) notifyLocked(method string, params any) error {
	return s.sendLocked(0, method, params)
}

func (s *stdioServer) sendLocked(id int64, method string, params any) error {
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
	b = append(b, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stdin == nil {
		return fmt.Errorf("stdio: not connected")
	}
	_, err = s.stdin.Write(b)
	return err
}

func (s *stdioServer) readLoop() {
	for s.scanner.Scan() {
		var resp Response
		if err := json.Unmarshal(s.scanner.Bytes(), &resp); err != nil {
			continue
		}
		if resp.ID == nil {
			continue
		}
		if ch, ok := s.pending.Load(*resp.ID); ok {
			ch.(chan Response) <- resp
		}
	}
}
