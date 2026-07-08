// Package mcp implements a minimal Model Context Protocol server over stdio,
// speaking newline-delimited JSON-RPC 2.0. It exposes registered tools that a
// client such as Claude can list and call.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
)

// defaultProtocolVersion is the MCP version echoed when a client sends none.
const defaultProtocolVersion = "2025-06-18"

// Handler runs a tool call with the raw JSON arguments and returns text output.
// A non-nil error marks the tool result as an error.
type Handler func(ctx context.Context, args json.RawMessage) (string, error)

// Tool is a callable the server exposes over MCP.
type Tool struct {
	// Name is the unique tool identifier.
	Name string
	// Description is a human-readable summary for the model.
	Description string
	// InputSchema is the JSON Schema for the tool's arguments.
	InputSchema map[string]any
	// Handler executes the tool.
	Handler Handler
}

// Server is a stdio MCP server exposing a set of tools.
type Server struct {
	// name identifies the server to clients.
	name string
	// version is the server version reported to clients.
	version string
	// tools holds registered tools keyed by name.
	tools map[string]Tool
	// order preserves tool registration order for listing.
	order []string
}

// NewServer returns a Server with the given name and version.
func NewServer(name, version string) *Server {
	return &Server{name: name, version: version, tools: make(map[string]Tool)}
}

// Register adds a tool. It panics on an empty name, a nil handler, or a
// duplicate name, all of which signal developer error.
func (s *Server) Register(t Tool) {
	if t.Name == "" {
		panic("mcp: Register with empty tool name")
	}
	if t.Handler == nil {
		panic("mcp: Register with nil handler for " + t.Name)
	}
	if _, dup := s.tools[t.Name]; dup {
		panic("mcp: Register called twice for " + t.Name)
	}
	s.tools[t.Name] = t
	s.order = append(s.order, t.Name)
}

// rpcRequest is an incoming JSON-RPC message. A missing id marks a notification.
type rpcRequest struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is an outgoing JSON-RPC message.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads newline-delimited JSON-RPC requests from r and writes responses to
// w until r is exhausted. Notifications receive no response.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(w)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if encErr := enc.Encode(parseErrorResponse()); encErr != nil {
				return encErr
			}
			continue
		}
		resp, respond := s.handle(ctx, req)
		if !respond {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// handle dispatches one request, returning the response and whether to send it.
func (s *Server) handle(ctx context.Context, req rpcRequest) (rpcResponse, bool) {
	if len(req.ID) == 0 {
		return rpcResponse{}, false // Notifications get no response.
	}
	switch req.Method {
	case "initialize":
		return s.ok(req.ID, s.initializeResult(req.Params)), true
	case "ping":
		return s.ok(req.ID, map[string]any{}), true
	case "tools/list":
		return s.ok(req.ID, map[string]any{"tools": s.toolList()}), true
	case "tools/call":
		return s.callTool(ctx, req.ID, req.Params), true
	default:
		return s.fail(req.ID, -32601, "method not found: "+req.Method), true
	}
}

// initializeResult builds the initialize response, echoing the client's protocol
// version when present.
func (s *Server) initializeResult(params json.RawMessage) map[string]any {
	version := defaultProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": s.name, "version": s.version},
	}
}

// toolInfo is a tool as advertised by tools/list.
type toolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolList returns the registered tools in registration order.
func (s *Server) toolList() []toolInfo {
	out := make([]toolInfo, 0, len(s.order))
	for _, name := range s.order {
		t := s.tools[name]
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, toolInfo{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	return out
}

// callTool runs a tools/call request. An unknown tool is a protocol error; a
// handler error is reported as an error tool result.
func (s *Server) callTool(ctx context.Context, id, params json.RawMessage) rpcResponse {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return s.fail(id, -32602, "invalid tool call params")
	}
	t, ok := s.tools[p.Name]
	if !ok {
		// MCP models an unknown tool name as invalid params (-32602), not
		// method-not-found: the tools/call method itself is valid.
		return s.fail(id, -32602, "unknown tool: "+p.Name)
	}
	out, err := t.Handler(ctx, p.Arguments)
	text := out
	if err != nil && text == "" {
		text = err.Error()
	}
	return s.ok(id, toolResult(text, err != nil))
}

// toolResult builds a tools/call result with a single text content block.
func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

// ok builds a success response.
func (s *Server) ok(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

// fail builds an error response.
func (s *Server) fail(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// parseErrorResponse is the response for an unparseable request line.
func parseErrorResponse() rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error"}}
}
