package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 message.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 message.
type JSONRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCError holds a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolHandler is the function signature for MCP tool implementations.
type ToolHandler func(ctx context.Context, params json.RawMessage) (any, error)

// Server is the MCP stdio server.
type Server struct {
	config       *config.Config
	transport    transport.Transport
	tools        map[string]ToolHandler
	defs         []ToolDef
	profileLevel config.PermissionLevel
	approver     security.Approver
	rateLimiter  *security.RateLimiter
	auditLogger  *security.AuditLogger
	version      string
}

// ServerOption is a functional option for Server.
type ServerOption func(*Server)

// WithProfile sets the permission level for the server.
func WithProfile(level config.PermissionLevel) ServerOption {
	return func(s *Server) { s.profileLevel = level }
}

// WithApprover sets the approver for write operations.
func WithApprover(a security.Approver) ServerOption {
	return func(s *Server) { s.approver = a }
}

// WithRateLimiter sets the rate limiter for write operations.
func WithRateLimiter(rl *security.RateLimiter) ServerOption {
	return func(s *Server) { s.rateLimiter = rl }
}

// WithAuditLogger sets the audit logger.
func WithAuditLogger(al *security.AuditLogger) ServerOption {
	return func(s *Server) { s.auditLogger = al }
}

// WithVersion sets the server version string.
func WithVersion(v string) ServerOption {
	return func(s *Server) { s.version = v }
}

// NewServer creates an MCP server backed by cfg and t, with all tools registered.
func NewServer(cfg *config.Config, t transport.Transport, opts ...ServerOption) *Server {
	s := &Server{
		config:       cfg,
		transport:    t,
		tools:        make(map[string]ToolHandler),
		profileLevel: config.LevelAdmin,
		version:      "0.1.0",
	}
	for _, opt := range opts {
		opt(s)
	}
	registerTools(s)
	return s
}

// Run reads newline-delimited JSON-RPC requests from stdin and writes responses to stdout.
// All debug output goes to stderr.
func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &RPCError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		// Notifications have no ID — process but do not respond.
		if req.ID == nil {
			s.dispatch(ctx, &req) //nolint:errcheck
			continue
		}

		result, rpcErr := s.dispatch(ctx, &req)

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}

		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "mcp: encode response: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp: stdin read: %w", err)
	}
	return nil
}

// Dispatch routes a JSON-RPC request to the appropriate handler and returns
// the result or an RPC error. It is exported so that external packages (e.g.
// integration test suites) can invoke tool handlers directly without going
// through the stdin/stdout loop.
func (s *Server) Dispatch(ctx context.Context, req *JSONRPCRequest) (any, *RPCError) {
	return s.dispatch(ctx, req)
}

// dispatch routes a request to the appropriate handler.
func (s *Server) dispatch(ctx context.Context, req *JSONRPCRequest) (any, *RPCError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "remops",
				"version": s.version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}, nil

	case "notifications/initialized":
		// No-op acknowledgement.
		return nil, nil

	case "tools/list":
		return map[string]any{"tools": s.defs}, nil

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &RPCError{Code: -32602, Message: "invalid params"}
		}

		handler, ok := s.tools[p.Name]
		if !ok {
			return nil, &RPCError{Code: -32601, Message: fmt.Sprintf("tool not found: %s", p.Name)}
		}

		result, err := handler(ctx, p.Arguments)
		if err != nil {
			return nil, &RPCError{Code: -32603, Message: err.Error()}
		}
		return result, nil

	default:
		return nil, &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}
}
