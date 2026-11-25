package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const internalToolMessage = "the node failed to run the tool; the cause is in its log"

var emptyToolSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

// ServeMCP runs the solodawn MCP server for the CLI process that token names,
// reading JSON-RPC frames from in and writing replies to out until the stream
// ends or ctx is cancelled (contracts/local.md section 3,
// contracts/mcp-tools.md). It closes out before returning, and refuses a token
// that names no active session.
func (h *Host) ServeMCP(ctx context.Context, token string, in io.Reader, out io.WriteCloser) error {
	caller, ok := h.mcpCaller(token)
	if !ok {
		out.Close()
		return &Error{Code: "forbidden", Message: "unknown session token"}
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	if caller.External {
		defer h.ExternalPresent(caller.Group, caller.Member)()
		go h.untilRemoved(ctx, stop, aiGroupKey{group: caller.Group, member: caller.Member})
	}
	srv := h.newMCPServer(token, caller)
	tr := &mcpTransport{
		io:      mcp.IOTransport{Reader: io.NopCloser(in), Writer: out},
		methods: map[jsonrpc.ID]string{},
	}
	return srv.Run(ctx, tr)
}

// untilRemoved ends the MCP connection of an external AI whose member is gone.
func (h *Host) untilRemoved(ctx context.Context, stop context.CancelFunc, key aiGroupKey) {
	defer stop()
	for again := true; again; {
		woken := extWaker(h, key)
		if _, again = h.ExternalMember(key.member); again {
			again, _ = extSlept(ctx, h, key.member, woken, nil)
		}
	}
}

func (h *Host) newMCPServer(token string, initial Caller) *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "solodawn", Version: "1"},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
			Logger:       slog.New(slog.DiscardHandler),
		},
	)
	srv.AddReceivingMiddleware(mcpMiddleware)
	listed := ToolsFor(initial)
	for _, name := range slices.Sorted(maps.Keys(listed)) {
		h.addMCPTool(srv, token, initial, name, listed[name])
	}
	return srv
}

func mcpMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "server/discover" {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
		}
		res, err := next(ctx, method, req)
		if err != nil {
			return res, err
		}
		return wrapValidationResult(res), nil
	}
}

func wrapValidationResult(res mcp.Result) mcp.Result {
	if callRes, ok := res.(*mcp.CallToolResult); ok && callRes.GetError() != nil {
		return toolErrorResult(&Error{Code: "bad_request", Message: callRes.GetError().Error()})
	}
	return res
}

func (h *Host) addMCPTool(srv *mcp.Server, token string, initial Caller, name string, tool Tool) {
	schema := schemaForCaller(name, initial, tool.Schema)
	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ json.RawMessage) (*mcp.CallToolResult, any, error) {
		return h.invokeTool(ctx, token, name, req.Params.Arguments), nil, nil
	}
	mcp.AddTool[json.RawMessage, any](srv, &mcp.Tool{
		Name:        name,
		Description: tool.Description,
		InputSchema: schema,
	}, handler)
}

func (h *Host) invokeTool(ctx context.Context, token, name string, raw json.RawMessage) *mcp.CallToolResult {
	caller, ok := h.mcpCaller(token)
	if !ok {
		return toolErrorResult(&Error{Code: "forbidden", Message: "unknown session token"})
	}
	tool, allowed := ToolsFor(caller)[name]
	if !allowed {
		return toolErrorResult(&Error{Code: "forbidden", Message: "tool is not available in this session"})
	}
	if tool.For != nil && !tool.For(caller) {
		return toolErrorResult(refuseForRun(name, neededKinds(caller, tool.For)))
	}
	done := h.Sessions.Calling(caller.SessionKey)
	defer done()
	return h.runToolCall(ctx, caller, name, tool.Call, raw)
}

// mcpCaller is the caller a session token, or an external AI's member id,
// stands for.
func (h *Host) mcpCaller(token string) (Caller, bool) {
	if c, ok := h.Sessions.Caller(token); ok {
		return c, true
	}
	return externalCaller(h, token)
}

func (h *Host) runToolCall(ctx context.Context, caller Caller, name string, call func(context.Context, *Host, Caller, json.RawMessage) (any, error), raw json.RawMessage) *mcp.CallToolResult {
	args := raw
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	start := time.Now()
	value, err := call(ctx, h, caller, args)
	h.diag.Info("tool", diagSession(caller.SessionKey, "name", name, "code", diagCode(err), "ms", time.Since(start).Milliseconds())...)
	if err != nil {
		return toolFailureResult(err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: encodeToolValue(value)}},
	}
}

func encodeToolValue(value any) string {
	if raw, ok := value.(json.RawMessage); ok {
		return strings.TrimSpace(string(raw))
	}
	return marshalWire(value)
}

func toolFailureResult(err error) *mcp.CallToolResult {
	var hostErr *Error
	if errors.As(err, &hostErr) {
		return toolErrorResult(hostErr)
	}
	return toolErrorResult(&Error{Code: "internal", Message: internalToolMessage})
}

func toolErrorResult(err *Error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		IsError: true,
	}
}

func schemaForCaller(name string, caller Caller, raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return emptyToolSchema
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	dec.Decode(&root)
	projectCallerSchema(caller, name, root)
	return json.RawMessage(marshalWire(root))
}

// projectCallerSchema adapts the superset tool schemas registered in Tools to
// the caller's session kind and secretary role (contracts/mcp-tools.md section 5).
func projectCallerSchema(caller Caller, name string, root map[string]any) {
	if (caller.External || caller.Kind != GroupSession) && name != "ask" {
		dropSessionDefaults(root)
	}
	if name == "post" && !caller.Secretary {
		projectPostSchema(postKind(caller), root)
	}
}

// postKind gives an external caller the post kinds of a group session.
func postKind(c Caller) Kind {
	if c.External {
		return GroupSession
	}
	return c.Kind
}

func dropSessionDefaults(root map[string]any) {
	if req, ok := root["required"].([]any); ok {
		kept := slices.DeleteFunc(req, isSessionDefault)
		if len(kept) == 0 {
			delete(root, "required")
			return
		}
		root["required"] = kept
	}
}

func isSessionDefault(item any) bool {
	return item == "task" || item == "to"
}

func projectPostSchema(kind Kind, root map[string]any) {
	if props, ok := root["properties"].(map[string]any); ok {
		delete(props, "task")
		delete(props, "group")
		trimPostKinds(kind, props)
	}
}

func trimPostKinds(kind Kind, props map[string]any) {
	if kindSchema, ok := props["kind"].(map[string]any); ok {
		if enum, ok := kindSchema["enum"].([]any); ok {
			kindSchema["enum"] = slices.DeleteFunc(enum, func(item any) bool {
				return dropPostKind(kind, item)
			})
		}
	}
}

func dropPostKind(kind Kind, item any) bool {
	if item == "summary" {
		return true
	}
	return kind == GroupSession && item == "handback"
}

type mcpTransport struct {
	io      mcp.IOTransport
	mu      sync.Mutex // guards methods across Read and Write
	methods map[jsonrpc.ID]string
}

func (t *mcpTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	conn, err := t.io.Connect(ctx)
	return &mcpConn{Connection: conn, mu: &t.mu, methods: t.methods}, err
}

type mcpConn struct {
	mcp.Connection
	mu      *sync.Mutex
	methods map[jsonrpc.ID]string
}

func (c *mcpConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	msg, err := c.Connection.Read(ctx)
	if req, ok := msg.(*jsonrpc.Request); ok {
		c.mu.Lock()
		c.methods[req.ID] = req.Method
		c.mu.Unlock()
	}
	return msg, err
}

func (c *mcpConn) Write(ctx context.Context, msg jsonrpc.Message) error {
	if resp, ok := msg.(*jsonrpc.Response); ok {
		c.mu.Lock()
		method := c.methods[resp.ID]
		delete(c.methods, resp.ID)
		c.mu.Unlock()
		if resp.Error == nil {
			resp.Result = rewriteMCPResult(method, resp.Result)
		}
	}
	return c.Connection.Write(ctx, msg)
}

func rewriteMCPResult(method string, raw json.RawMessage) json.RawMessage {
	if method != "tools/call" {
		return raw
	}
	var call struct {
		Content json.RawMessage `json:"content"`
		IsError bool            `json:"isError"`
	}
	json.Unmarshal(raw, &call)
	return json.RawMessage(marshalWire(call))
}
