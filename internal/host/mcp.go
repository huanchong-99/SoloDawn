package host

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const internalToolMessage = "the node failed to run the tool; the cause is in its log"

var emptyToolSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

// contractToolOrder lists the tools in the table order of contracts/mcp-tools.md
// sections 1, 2, 3 and 5.
var contractToolOrder = []string{
	"post",
	"ask",
	"reply",
	"read_thread",
	"list_members",
	"list_tasks",
	"note_put",
	"note_list",
	"start_task",
	"push_branch",
	"run_checks",
	"submit_task",
	"submit_review",
	"request_decision",
	"net_status",
	"propose_brief",
	"set_task",
	"approve",
}

// schemaKeywords are the JSON Schema keywords allowed in contracts/mcp-tools.md
// section 5.
var schemaKeywords = []string{
	"type",
	"enum",
	"default",
	"minimum",
	"maximum",
	"items",
	"properties",
	"required",
	"additionalProperties",
}

// ServeMCP runs the solodawn MCP server for the CLI process that token names,
// reading JSON-RPC frames from in and writing replies to out until the stream
// ends or ctx is cancelled (contracts/local.md section 3,
// contracts/mcp-tools.md). It closes out before returning, and refuses a token
// that names no active session.
func (h *Host) ServeMCP(ctx context.Context, token string, in io.Reader, out io.WriteCloser) error {
	rc := inputCloser(in, out)
	caller, ok := h.Sessions.Caller(token)
	if !ok {
		rc.Close()
		out.Close()
		return &Error{Code: "forbidden", Message: "unknown session token"}
	}
	srv, err := h.newMCPServer(token, caller)
	if err != nil {
		rc.Close()
		out.Close()
		return err
	}
	tr := &mcpTransport{
		io:      mcp.IOTransport{Reader: rc, Writer: out},
		methods: map[jsonrpc.ID]string{},
	}
	return srv.Run(ctx, tr)
}

func inputCloser(in io.Reader, out io.WriteCloser) io.ReadCloser {
	if closer, ok := in.(io.ReadCloser); ok && any(in) != any(out) {
		return closer
	}
	return io.NopCloser(in)
}

func (h *Host) newMCPServer(token string, initial Caller) (*mcp.Server, error) {
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
		if err := h.addMCPTool(srv, token, initial, name, listed[name]); err != nil {
			return nil, err
		}
	}
	return srv, nil
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

func (h *Host) addMCPTool(srv *mcp.Server, token string, initial Caller, name string, tool Tool) error {
	schema, err := schemaForCaller(name, initial, tool.Schema)
	if err != nil {
		return err
	}
	handler := func(ctx context.Context, req *mcp.CallToolRequest, _ json.RawMessage) (*mcp.CallToolResult, any, error) {
		return h.invokeTool(ctx, token, name, req.Params.Arguments), nil, nil
	}
	mcp.AddTool[json.RawMessage, any](srv, &mcp.Tool{
		Name:        name,
		Description: tool.Description,
		InputSchema: schema,
	}, handler)
	return nil
}

func (h *Host) invokeTool(ctx context.Context, token, name string, raw json.RawMessage) *mcp.CallToolResult {
	caller, ok := h.Sessions.Caller(token)
	if !ok {
		return toolErrorResult(&Error{Code: "forbidden", Message: "unknown session token"})
	}
	tool, allowed := ToolsFor(caller)[name]
	if !allowed || tool.Call == nil {
		return toolErrorResult(&Error{Code: "forbidden", Message: "tool is not available in this session"})
	}
	done := h.Sessions.Calling(caller.SessionKey)
	defer done()
	return h.runToolCall(ctx, caller, tool.Call, raw)
}

func (h *Host) runToolCall(ctx context.Context, caller Caller, call func(context.Context, *Host, Caller, json.RawMessage) (any, error), raw json.RawMessage) *mcp.CallToolResult {
	args := raw
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	value, err := call(ctx, h, caller, args)
	if err != nil {
		return toolFailureResult(err)
	}
	encoded, err := mcpEncodeJSON(value)
	if err != nil {
		return toolFailureResult(err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: encoded}},
	}
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

func mcpEncodeJSON(value any) (string, error) {
	if raw, ok := value.(json.RawMessage); ok && json.Valid(raw) {
		return strings.TrimSpace(string(raw)), nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func schemaForCaller(name string, caller Caller, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return emptyToolSchema, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	if unwrapNullableType(root["type"]) != "object" {
		return nil, errors.New("host: tool input schema must have type object")
	}
	cleanSchemaMap(root, true)
	projectCallerSchema(caller, name, root)
	encoded, _ := mcpEncodeJSON(root)
	return json.RawMessage(encoded), nil
}

// projectCallerSchema adapts the superset tool schemas registered in Tools to
// the caller's session kind and secretary role (contracts/mcp-tools.md section 5).
func projectCallerSchema(caller Caller, name string, root map[string]any) {
	if caller.Kind != GroupSession && name != "ask" {
		dropSessionDefaults(root)
	}
	if name == "post" && !caller.Secretary {
		projectPostSchema(caller.Kind, root)
	}
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

func unwrapNullableType(value any) any {
	list, _ := value.([]any)
	if len(list) != 2 || list[0] != "null" {
		return value
	}
	if name, ok := list[1].(string); ok {
		return name
	}
	return value
}

func dropExtraKeyword(key string, _ any) bool {
	return !slices.Contains(schemaKeywords, key)
}

func cleanSchemaMap(m map[string]any, root bool) {
	if kind, ok := m["type"]; ok {
		m["type"] = unwrapNullableType(kind)
	}
	maps.DeleteFunc(m, dropExtraKeyword)
	if req, _ := m["required"].([]any); len(req) == 0 {
		delete(m, "required")
	}
	if items, ok := m["items"].(map[string]any); ok {
		cleanSchemaMap(items, false)
	}
	cleanProperties(m, root)
}

func cleanProperties(m map[string]any, root bool) {
	props, hasProps := m["properties"].(map[string]any)
	if !hasProps {
		if root {
			m["properties"] = map[string]any{}
			setDefaultClosed(m)
		}
		return
	}
	cleanPropertyEntries(props)
	if m["type"] == "object" {
		setDefaultClosed(m)
	}
}

func cleanPropertyEntries(props map[string]any) {
	for _, prop := range props {
		if sub, ok := prop.(map[string]any); ok {
			cleanSchemaMap(sub, false)
		}
	}
}

func setDefaultClosed(m map[string]any) {
	if _, ok := m["additionalProperties"]; !ok {
		m["additionalProperties"] = false
	}
}

func toolRank(name string) int {
	i := slices.Index(contractToolOrder, name)
	if i < 0 {
		return len(contractToolOrder)
	}
	return i
}

func compareToolNames(a, b string) int {
	return cmp.Or(cmp.Compare(toolRank(a), toolRank(b)), cmp.Compare(a, b))
}

type listedTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func sortListedTools(a, b listedTool) int {
	return compareToolNames(a.Name, b.Name)
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
	switch method {
	case "tools/list":
		var list struct {
			Tools []listedTool `json:"tools"`
		}
		json.Unmarshal(raw, &list)
		slices.SortFunc(list.Tools, sortListedTools)
		out, _ := mcpEncodeJSON(list)
		return json.RawMessage(out)
	case "tools/call":
		var call struct {
			Content json.RawMessage `json:"content"`
			IsError bool            `json:"isError"`
		}
		json.Unmarshal(raw, &call)
		out, _ := mcpEncodeJSON(call)
		return json.RawMessage(out)
	default:
		return raw
	}
}
