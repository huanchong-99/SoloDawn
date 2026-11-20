package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testMCPSockEnv = "SOLODAWN_TEST_MCP_SOCK"

// TestMain lets tests start this binary as "solodawn mcp --token <t>" so that
// mcp.CommandTransport and fakecli run the real stdio shim over a Unix socket.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		dial := func() (net.Conn, error) {
			return net.Dial("unix", os.Getenv(testMCPSockEnv))
		}
		os.Exit(RunShim(os.Args[2:], dial, os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func saveTools(t *testing.T, next map[string]Tool) {
	t.Helper()
	before := maps.Clone(Tools)
	Tools = next
	t.Cleanup(func() { Tools = before })
}

func shortSocketPath(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "sdm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "sock")
}

// startTestSocket listens on a Unix socket, reads the {"mcp":"<token>"} opening
// line on each connection, and hands the stream to h.ServeMCP.
func startTestSocket(t *testing.T, h *Host) string {
	t.Helper()
	path := shortSocketPath(t)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		ln.Close()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				r := bufio.NewReader(c)
				line, err := r.ReadBytes('\n')
				if err != nil {
					c.Close()
					return
				}
				var open shimOpening
				if proto.Decode(line, &open) != nil || open.MCP == "" {
					c.Close()
					return
				}
				h.ServeMCP(ctx, open.MCP, r, c)
			}(conn)
		}
	}()
	return path
}

func connectSDKClient(t *testing.T, sockPath, token string) *mcp.ClientSession {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "mcp", "--token", token)
	cmd.Env = append(os.Environ(), testMCPSockEnv+"="+sockPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func loadGoldenToolLists(t *testing.T) proto.ToolLists {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var lists proto.ToolLists
	if err := dec.Decode(&lists); err != nil {
		t.Fatal(err)
	}
	return lists
}

func registerSupersetTools(t *testing.T, lists proto.ToolLists) {
	t.Helper()
	encode := func(in proto.ToolInput) json.RawMessage {
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	find := func(list proto.ToolList, name string) json.RawMessage {
		i := slices.IndexFunc(list.Tools, func(tl proto.Tool) bool { return tl.Name == name })
		if i < 0 {
			t.Fatalf("missing tool %s", name)
		}
		return encode(list.Tools[i].InputSchema)
	}
	noop := func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	}
	saveTools(t, map[string]Tool{
		"post":             {Description: "post a message", Schema: find(lists.Secretary, "post"), Call: noop},
		"ask":              {Description: "ask an AI", Schema: find(lists.Secretary, "ask"), Call: noop},
		"reply":            {Description: "reply to a call", Schema: find(lists.Secretary, "reply"), Call: noop},
		"read_thread":      {Description: "read a thread", Schema: find(lists.Secretary, "read_thread"), Call: noop},
		"list_members":     {Description: "list members", Schema: nil, Call: noop},
		"list_tasks":       {Description: "list tasks", Schema: find(lists.Secretary, "list_tasks"), Call: noop},
		"note_put":         {Description: "save a note", Schema: find(lists.Secretary, "note_put"), Call: noop},
		"note_list":        {Description: "list notes", Schema: find(lists.Secretary, "note_list"), Call: noop},
		"start_task":       {Description: "start a task", Schema: find(lists.Group, "start_task"), For: func(c Caller) bool { return c.Kind == GroupSession && !c.Secretary }, Call: noop},
		"push_branch":      {Description: "push branch", Schema: nil, For: func(c Caller) bool { return c.Kind == TaskSession && c.Level > 1 }, Call: noop},
		"run_checks":       {Description: "run checks", Schema: nil, For: func(c Caller) bool { return c.Kind == TaskSession && c.Level > 1 }, Call: noop},
		"submit_task":      {Description: "submit task", Schema: find(lists.TaskLevel23, "submit_task"), For: func(c Caller) bool { return c.Kind == TaskSession && c.Level > 1 }, Call: noop},
		"submit_review":    {Description: "submit review", Schema: find(lists.Review, "submit_review"), For: func(c Caller) bool { return c.Kind == ReviewSession }, Call: noop},
		"request_decision": {Description: "request a decision", Schema: find(lists.Secretary, "request_decision"), Call: noop},
		"net_status":       {Description: "network status", Schema: nil, Call: noop},
		"propose_brief":    {Description: "propose a brief", Schema: find(lists.Secretary, "propose_brief"), For: func(c Caller) bool { return c.Secretary }, Call: noop},
		"set_task":         {Description: "update a task", Schema: find(lists.Secretary, "set_task"), For: func(c Caller) bool { return c.Secretary }, Call: noop},
	})
}

func TestToolListsMatchGoldenSchemas(t *testing.T) {
	lists := loadGoldenToolLists(t)
	registerSupersetTools(t, lists)

	h, _ := seamHost(t, seamStore(t))
	sock := startTestSocket(t, h)

	cases := []struct {
		name    string
		session Session
		level   int
		want    proto.ToolList
	}{
		{
			name:    "secretary",
			session: Session{SessionKey: SessionKey{Group: "g1", Member: "m1"}, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: "h1", Name: "Secretary", Secretary: true},
			level:   1,
			want:    lists.Secretary,
		},
		{
			name:    "group",
			session: Session{SessionKey: SessionKey{Group: "g1", Member: "m2"}, Kind: GroupSession, Folder: "repo", Agent: "codex", Home: "h1", Name: "Codex"},
			level:   1,
			want:    lists.Group,
		},
		{
			name:    "review",
			session: Session{SessionKey: SessionKey{Group: "g1", Member: "m2", Task: "t9"}, Kind: ReviewSession, Folder: "review/9", Agent: "codex", Home: "h1", Name: "Codex"},
			level:   1,
			want:    lists.Review,
		},
		{
			name:    "task_level_1",
			session: Session{SessionKey: SessionKey{Group: "g1", Member: "m3", Task: "t1"}, Kind: TaskSession, Folder: "wt/1-a", Agent: "claude", Home: "h1", Name: "Claude"},
			level:   1,
			want:    lists.TaskLevel1,
		},
		{
			name:    "task_level_2",
			session: Session{SessionKey: SessionKey{Group: "g1", Member: "m3", Task: "t2"}, Kind: TaskSession, Folder: "wt/2-b", Agent: "claude", Home: "h1", Name: "Claude"},
			level:   2,
			want:    lists.TaskLevel23,
		},
		{
			name:    "task_level_3",
			session: Session{SessionKey: SessionKey{Group: "g1", Member: "m3", Task: "t3"}, Kind: TaskSession, Folder: "wt/3-c", Agent: "claude", Home: "h1", Name: "Claude"},
			level:   3,
			want:    lists.TaskLevel23,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := h.Sessions.Create(tc.session); err != nil {
				t.Fatal(err)
			}
			token := h.Sessions.Admit(tc.session.SessionKey, tc.level, false)
			rawResult := rawToolsListOverSocket(t, sock, token)

			var wire map[string]any
			if err := json.Unmarshal(rawResult, &wire); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(slices.Sorted(maps.Keys(wire)), []string{"tools"}) {
				t.Fatalf("tools/list keys = %v, want [tools]", maps.Keys(wire))
			}
			for _, entry := range wire["tools"].([]any) {
				obj := entry.(map[string]any)
				if desc, ok := obj["description"].(string); !ok || desc == "" {
					t.Fatalf("missing description in %v", obj)
				}
				delete(obj, "description")
			}
			stripped, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			dec := json.NewDecoder(bytes.NewReader(stripped))
			dec.DisallowUnknownFields()
			var got proto.ToolList
			if err := dec.Decode(&got); err != nil {
				t.Fatalf("strict decode into proto.ToolList: %v\n%s", err, stripped)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func rawToolsListOverSocket(t *testing.T, sockPath, token string) json.RawMessage {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeOpeningLine(conn, token); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(conn)
	fmt.Fprintln(conn, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	var initResp struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := dec.Decode(&initResp); err != nil || initResp.Error != nil {
		t.Fatalf("initialize: %v (%v)", err, initResp.Error)
	}
	fmt.Fprintln(conn, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	fmt.Fprintln(conn, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var listResp struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := dec.Decode(&listResp); err != nil || listResp.Error != nil {
		t.Fatalf("tools/list: %v (%v)", err, listResp.Error)
	}
	return listResp.Result
}

func TestSchemaNormalisationAndOrdering(t *testing.T) {
	messy := json.RawMessage(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title": "MessyArgs",
		"description": "top description",
		"type": ["null", "object"],
		"required": [],
		"properties": {
			"tags": {
				"type": ["null", "array"],
				"description": "tag list",
				"items": {
					"title": "TagItem",
					"type": "object",
					"required": [],
					"properties": {
						"label": {"type": "string", "title": "Label"}
					}
				}
			},
			"free_input": {
				"type": "object",
				"description": "unconstrained nested map like approve.input"
			},
			"open_obj": {
				"type": "object",
				"properties": {"k": {"type": "string"}},
				"additionalProperties": true
			},
			"choice": {
				"enum": ["a", "b"],
				"title": "UntypedEnum"
			},
			"weird_str": {
				"type": "string",
				"properties": {"ignored": true}
			}
		}
	}`)

	caller := Caller{Session: Session{Kind: TaskSession}, Level: 2}
	norm, err := schemaForCaller("custom", caller, messy)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(norm, &got); err != nil {
		t.Fatal(err)
	}
	wantJSON := `{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"tags": {
				"type": "array",
				"items": {
					"type": "object",
					"additionalProperties": false,
					"properties": {
						"label": {"type": "string"}
					}
				}
			},
			"free_input": {
				"type": "object"
			},
			"open_obj": {
				"type": "object",
				"properties": {"k": {"type": "string"}},
				"additionalProperties": true
			},
			"choice": {
				"enum": ["a", "b"]
			},
			"weird_str": {
				"type": "string",
				"properties": {"ignored": true}
			}
		}
	}`
	var want map[string]any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %s\nwant %v", norm, want)
	}

	// Root object schema without a properties map gets empty properties and additionalProperties: false.
	bareNorm, err := schemaForCaller("bare", caller, json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`))
	if err != nil || string(bareNorm) != `{"additionalProperties":false,"properties":{},"type":"object"}` {
		t.Fatalf("bare object schema = %s (%v)", bareNorm, err)
	}
	openNorm, err := schemaForCaller("open", caller, json.RawMessage(`{"type":"object","additionalProperties":true}`))
	if err != nil || string(openNorm) != `{"additionalProperties":true,"properties":{},"type":"object"}` {
		t.Fatalf("open root schema = %s (%v)", openNorm, err)
	}

	// Tool ordering puts contract tools in table order (with post at index 0)
	// before custom tools in alphabetical order.
	saveTools(t, map[string]Tool{
		"zebra":   {},
		"alpha":   {},
		"approve": {},
		"ask":     {},
		"post":    {},
	})
	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamTask, Working)
	sock := startTestSocket(t, h)
	raw := rawToolsListOverSocket(t, sock, h.Sessions.Admit(seamTask, 2, false))
	var listed proto.ToolList
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range listed.Tools {
		names = append(names, tl.Name)
	}
	wantOrder := []string{"post", "ask", "approve", "alpha", "zebra"}
	if !slices.Equal(names, wantOrder) {
		t.Fatalf("tool order = %v, want %v", names, wantOrder)
	}
}

func TestInvalidSchemasAreRefusedAndCloseStream(t *testing.T) {
	badSchemas := []string{
		`{`,
		`null`,
		`{}`,
		`{"type":"string"}`,
		`{"type":["null"]}`,
		`{"type":["null","object","string"]}`,
		`{"type":["object","null"]}`,
		`{"type":["string","object"]}`,
		`{"type":["null",7]}`,
	}
	for _, bad := range badSchemas {
		saveTools(t, map[string]Tool{"broken": {Schema: json.RawMessage(bad)}})
		h, _ := seamHost(t, seamStore(t))
		seamIn(t, h.Sessions, seamTask, Working)
		token := h.Sessions.Admit(seamTask, 2, false)
		inR, inW := io.Pipe()
		defer inW.Close()
		outR, outW := io.Pipe()
		defer outR.Close()
		err := h.ServeMCP(t.Context(), token, inR, outW)
		if err == nil {
			t.Fatalf("schema %s was accepted", bad)
		}
		if _, werr := inW.Write([]byte("x")); !errors.Is(werr, io.ErrClosedPipe) {
			t.Fatalf("schema %s left input open: %v", bad, werr)
		}
		if _, rerr := outR.Read(make([]byte, 1)); !errors.Is(rerr, io.EOF) {
			t.Fatalf("schema %s left output open: %v", bad, rerr)
		}
	}
}

func TestToolCallsTrackLiveCallerCountAndErrors(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	secKey := SessionKey{Group: "g1", Member: "m1"}
	err := h.Sessions.Create(Session{
		SessionKey: secKey,
		Kind:       GroupSession,
		Folder:     "repo",
		Agent:      "claude",
		Home:       "h1",
		Name:       "Secretary",
		Secretary:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Sessions.Move(secKey, Dormant, Working); err != nil {
		t.Fatal(err)
	}
	h.Sessions.SetTurn(secKey, Turn{Kind: ItemsTurn, Thread: "t1", Depth: 1})

	// Hold two concurrent calls inside barrier to verify Calling increments to 2.
	// arrived is read by the test goroutine; release is closed by the test goroutine.
	arrived := make(chan int, 2)
	release := make(chan struct{})

	saveTools(t, map[string]Tool{
		"inspect": {
			Description: "inspect caller and args",
			Schema:      json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer","minimum":1},"raw":{"type":"string"}},"additionalProperties":false}`),
			Call: func(_ context.Context, gotH *Host, c Caller, args json.RawMessage) (any, error) {
				return map[string]any{
					"same_host": gotH == h,
					"turn_n":    c.Turn.N,
					"turn_kind": c.Turn.Kind,
					"thread":    c.Turn.Thread,
					"depth":     c.Turn.Depth,
					"calls":     gotH.Sessions.Calls(c.SessionKey),
					"args":      string(args),
					"html":      "<tag>&",
				}, nil
			},
		},
		"barrier": {
			Call: func(_ context.Context, gotH *Host, c Caller, _ json.RawMessage) (any, error) {
				arrived <- gotH.Sessions.Calls(c.SessionKey)
				<-release
				return map[string]bool{"ok": true}, nil
			},
		},
		"sec_only": {
			For: func(c Caller) bool { return c.Secretary },
			Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
				return map[string]bool{"ok": true}, nil
			},
		},
		"nil_call": {
			Description: "registered without a Call function",
		},
		"domain_err": {
			Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
				return nil, fmt.Errorf("outer: %w", &Error{Code: "secret", Message: "this looks like a secret"})
			},
		},
		"unexpected_err": {
			Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
				return nil, errors.New("sensitive sqlite path /private/db")
			},
		},
		"bad_output": {
			Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
				return math.NaN(), nil
			},
		},
		"bad_raw": {
			Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
				return json.RawMessage("{"), nil
			},
		},
	})

	sock := startTestSocket(t, h)
	token := h.Sessions.Admit(secKey, 1, false)
	cs := connectSDKClient(t, sock, token)
	ctx := t.Context()

	// Calling an unlisted tool fails with a JSON-RPC error without rewriting Error into Result.
	if _, unlistedErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: "unlisted_tool"}); unlistedErr == nil {
		t.Fatal("unlisted_tool unexpectedly succeeded")
	}

	// First call preserves exact raw JSON (large integer > 1e6, <, >, &) and unescaped output.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "inspect",
		Arguments: json.RawMessage(`{"n":1500000,"raw":"<a>&<b>"}`),
	})
	if err != nil || res.IsError {
		t.Fatalf("inspect failed: %v, %+v", err, res)
	}
	text1 := res.Content[0].(*mcp.TextContent).Text
	var inspectOut map[string]any
	if err := json.Unmarshal([]byte(text1), &inspectOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text1, `"html":"<tag>&"`) || inspectOut["args"] != `{"n":1500000,"raw":"<a>&<b>"}` || inspectOut["calls"] != 1.0 || inspectOut["turn_n"] != 1.0 || inspectOut["same_host"] != true {
		t.Fatalf("unexpected inspect output: %s", text1)
	}
	if h.Sessions.Calls(secKey) != 0 {
		t.Fatalf("calls after return = %d", h.Sessions.Calls(secKey))
	}

	// SetTurn on the session updates Caller.Turn for the next call on the same connection.
	h.Sessions.SetTurn(secKey, Turn{Kind: HandbackTurn, Thread: "t2", Depth: 0})
	res2, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "inspect"})
	if err != nil || res2.IsError {
		t.Fatalf("second inspect failed: %v, %+v", err, res2)
	}
	text2 := res2.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text2, `"turn_n":2`) || !strings.Contains(text2, `"turn_kind":"handback"`) || !strings.Contains(text2, `"thread":"t2"`) {
		t.Fatalf("live turn was not observed: %s", text2)
	}

	// Two concurrent calls increment Calls to 2 and return to 0 once released.
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: "barrier"})
			if callErr != nil || r.IsError {
				t.Errorf("barrier: %v, %+v", callErr, r)
			}
		}()
	}
	counts := []int{<-arrived, <-arrived}
	slices.Sort(counts)
	if !slices.Equal(counts, []int{1, 2}) || h.Sessions.Calls(secKey) != 2 {
		t.Fatalf("concurrent counts = %v, current = %d", counts, h.Sessions.Calls(secKey))
	}
	close(release)
	wg.Wait()
	if h.Sessions.Calls(secKey) != 0 {
		t.Fatalf("calls after barrier = %d", h.Sessions.Calls(secKey))
	}

	// Schema validation failure answers bad_request tool error.
	badArg, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "inspect",
		Arguments: map[string]any{"unknown_field": 1},
	})
	if err != nil || !badArg.IsError || !strings.HasPrefix(badArg.Content[0].(*mcp.TextContent).Text, "bad_request: ") {
		t.Fatalf("expected bad_request tool error, got %v / %+v", err, badArg)
	}

	// Domain *Error (wrapped) keeps code: message; non-*Error and unmarshalable output answer internal without leaking details.
	domRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "domain_err"})
	if err != nil || !domRes.IsError || domRes.Content[0].(*mcp.TextContent).Text != "secret: this looks like a secret" {
		t.Fatalf("domain_err = %v / %+v", err, domRes)
	}
	for _, toolName := range []string{"unexpected_err", "bad_output", "bad_raw"} {
		intRes, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: toolName})
		want := "internal: " + internalToolMessage
		if callErr != nil || !intRes.IsError || intRes.Content[0].(*mcp.TextContent).Text != want {
			t.Fatalf("%s = %v / %+v, want %q", toolName, callErr, intRes, want)
		}
	}

	// Nil Call function and dynamic Secretary revocation both refuse with forbidden.
	nilRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "nil_call"})
	if err != nil || !nilRes.IsError || nilRes.Content[0].(*mcp.TextContent).Text != "forbidden: tool is not available in this session" {
		t.Fatalf("nil_call = %v / %+v", err, nilRes)
	}
	err = h.Sessions.Update(secKey, func(s *Session) { s.Secretary = false })
	if err != nil {
		t.Fatal(err)
	}
	secRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "sec_only"})
	if err != nil || !secRes.IsError || secRes.Content[0].(*mcp.TextContent).Text != "forbidden: tool is not available in this session" {
		t.Fatalf("sec_only after update = %v / %+v", err, secRes)
	}

	// Forgetting the token while connected refuses subsequent tool calls.
	h.Sessions.Forget(token)
	forgotRes, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "inspect"})
	if err != nil || !forgotRes.IsError || forgotRes.Content[0].(*mcp.TextContent).Text != "forbidden: unknown session token" {
		t.Fatalf("inspect after Forget = %v / %+v", err, forgotRes)
	}
}

func TestOmittedArgumentsDeliverEmptyObjectAndContextCancelsServer(t *testing.T) {
	var seenArgs string
	saveTools(t, map[string]Tool{
		"no_args": {
			Call: func(context.Context, *Host, Caller, json.RawMessage) (any, error) {
				return map[string]bool{"ok": true}, nil
			},
		},
		"capture": {
			Call: func(_ context.Context, _ *Host, _ Caller, args json.RawMessage) (any, error) {
				seenArgs = string(args)
				return map[string]bool{"ok": true}, nil
			},
		},
	})
	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamTask, Working)
	token := h.Sessions.Admit(seamTask, 2, false)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	ctx, cancel := context.WithCancel(t.Context())
	// errCh is read once below after cancel closes the server.
	errCh := make(chan error, 1)
	go func() {
		errCh <- h.ServeMCP(ctx, token, bufio.NewReader(serverConn), serverConn)
	}()

	clientConn.SetDeadline(time.Now().Add(5 * time.Second))
	dec := json.NewDecoder(clientConn)
	// Claude Code's server/discover probe gets -32601.
	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","id":"probe-1","method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
	var probe struct {
		ID    string `json:"id"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := dec.Decode(&probe); err != nil || probe.ID != "probe-1" || probe.Error == nil || probe.Error.Code != -32601 {
		t.Fatalf("probe reply = %+v (%v)", probe, err)
	}

	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"codex","version":"1"}}}`)
	var initReply map[string]any
	if err := dec.Decode(&initReply); err != nil || initReply["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize = %v (%v)", initReply, err)
	}
	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	// Call "capture" with no "arguments" property at all in JSON-RPC params.
	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"capture"}}`)
	var callReply struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError *bool `json:"isError"`
		} `json:"result"`
	}
	if err := dec.Decode(&callReply); err != nil || callReply.Result.IsError == nil || *callReply.Result.IsError || seenArgs != "{}" {
		t.Fatalf("call without arguments = %+v, seenArgs = %q, err = %v", callReply, seenArgs, err)
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("ServeMCP after cancel = %v, want context.Canceled", err)
	}
}

type closeCounter struct {
	io.Reader
	io.Writer
	closes int
}

func (c *closeCounter) Close() error {
	c.closes++
	return nil
}

func TestRefusedTokenClosesDistinctAndSharedStreamsOnce(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	shared := &closeCounter{Reader: strings.NewReader(""), Writer: io.Discard}
	err := h.ServeMCP(t.Context(), "unknown", shared, shared)
	seamCode(t, err, "forbidden")
	if shared.closes != 1 {
		t.Fatalf("shared stream closed %d times, want 1", shared.closes)
	}

	inCounter := &closeCounter{Reader: strings.NewReader("")}
	outCounter := &closeCounter{Writer: io.Discard}
	err = h.ServeMCP(t.Context(), "unknown", inCounter, outCounter)
	seamCode(t, err, "forbidden")
	if inCounter.closes != 1 || outCounter.closes != 1 {
		t.Fatalf("distinct streams closed %d and %d times, want 1 and 1", inCounter.closes, outCounter.closes)
	}
}

func TestFakeCLIClaudeAndCodexFlowsAgainstRealShimAndMCPServer(t *testing.T) {
	const denyText = `{"behavior": "deny", "message": "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."}`
	calls := 0
	var whoamiTurns []int64
	saveTools(t, map[string]Tool{
		"approve": {
			Description: "permission prompt tool",
			Schema:      json.RawMessage(`{"type":"object","properties":{"tool_name":{"type":"string"},"input":{"type":"object"},"tool_use_id":{"type":"string"}},"required":["tool_name","input","tool_use_id"],"additionalProperties":false}`),
			Call: func(_ context.Context, _ *Host, c Caller, _ json.RawMessage) (any, error) {
				if c.SessionKey != seamTask || c.Level != 2 {
					return nil, &Error{Code: "forbidden", Message: "wrong caller"}
				}
				calls++
				if calls == 1 {
					return json.RawMessage(denyText), nil
				}
				return json.RawMessage(`{"behavior": "allow"}`), nil
			},
		},
		"whoami": {
			Description: "echo scripted resumed token after verifying caller",
			Call: func(_ context.Context, _ *Host, c Caller, _ json.RawMessage) (any, error) {
				if c.SessionKey != seamTask || c.Level != 2 {
					return nil, &Error{Code: "forbidden", Message: "wrong caller"}
				}
				whoamiTurns = append(whoamiTurns, c.Turn.N)
				return json.RawMessage(`{"token": "tok-resumed"}`), nil
			},
		},
	})

	h, _ := seamHost(t, seamStore(t))
	seamIn(t, h.Sessions, seamTask, Working)
	sock := startTestSocket(t, h)
	t.Setenv(testMCPSockEnv, sock)
	logPath := filepath.Join(t.TempDir(), "fakecli.log")
	t.Setenv("SOLODAWN_FAKECLI_LOG", logPath)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Claude Code approval + retry + resume (server/discover -> initialize 2025-11-25 -> tools/list -> tools/call).
	tokClaude1 := h.Sessions.Admit(seamTask, 2, false)
	cfgPath := writeTestClaudeMCPConfig(t, exe, tokClaude1)
	const claudeSession = "797fa93e-af8a-4768-b847-42582d5583a0"
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", filepath.Join("fakecli", "testdata", "claude-approval.json"))
	runFakeClaude(t, []string{"-p", "--session-id", claudeSession, "--mcp-config", cfgPath},
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"try"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Approved."}]}}`)
	h.Sessions.Forget(tokClaude1)

	h.Sessions.SetTurn(seamTask, Turn{Kind: ItemsTurn})
	tokClaude2 := h.Sessions.Admit(seamTask, 2, false)
	cfgPath2 := writeTestClaudeMCPConfig(t, exe, tokClaude2)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", filepath.Join("fakecli", "testdata", "claude-resume.json"))
	runFakeClaude(t, []string{"-p", "--resume", claudeSession, "--mcp-config", cfgPath2},
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"whoami"}]}}`)
	h.Sessions.Forget(tokClaude2)

	// 2. Codex resume + tool call with _meta.threadId (initialize 2025-06-18 -> tools/list -> tools/call).
	h.Sessions.SetTurn(seamTask, Turn{Kind: ItemsTurn})
	tokCodex := h.Sessions.Admit(seamTask, 2, false)
	t.Setenv("SOLODAWN_FAKE_CODEX_SCRIPT", filepath.Join("fakecli", "testdata", "codex-resume.json"))
	runFakeCodexResume(t, exe, tokCodex)
	h.Sessions.Forget(tokCodex)

	if calls != 2 || !slices.Equal(whoamiTurns, []int64{1, 2}) {
		t.Fatalf("calls = %d, whoamiTurns = %v", calls, whoamiTurns)
	}
}

func writeTestClaudeMCPConfig(t *testing.T, exe, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"solodawn": map[string]any{
				"command": exe,
				"args":    []string{"mcp", "--token", token},
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runFakeClaude(t *testing.T, args []string, turns ...string) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	var stderr bytes.Buffer
	// codeCh is read once at the end of runFakeClaude.
	codeCh := make(chan int, 1)
	go func() {
		code := fakecli.Run("claude", "test", args, inR, outW, &stderr)
		outW.Close()
		codeCh <- code
	}()
	dec := json.NewDecoder(outR)
	for _, turn := range turns {
		if _, err := fmt.Fprintln(inW, turn); err != nil {
			t.Fatal(err)
		}
		for {
			var msg map[string]any
			if err := dec.Decode(&msg); err != nil {
				t.Fatalf("claude decode: %v (stderr: %s)", err, stderr.String())
			}
			if msg["type"] == "result" {
				break
			}
		}
	}
	inW.Close()
	io.Copy(io.Discard, outR)
	if code := <-codeCh; code != 0 || stderr.Len() != 0 {
		t.Fatalf("fake claude exit %d, stderr %q", code, stderr.String())
	}
}

func runFakeCodexResume(t *testing.T, exe, token string) {
	t.Helper()
	const thread = "01a1f63f-701c-78c1-bbf3-f1ac61c56b59"
	cmdJSON, _ := json.Marshal(exe)
	argsJSON, _ := json.Marshal([]string{"mcp", "--token", token})
	args := []string{
		"app-server",
		"-c", "mcp_servers.solodawn.command=" + string(cmdJSON),
		"-c", "mcp_servers.solodawn.args=" + string(argsJSON),
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	var stderr bytes.Buffer
	// codeCh is read once at the end of runFakeCodexResume.
	codeCh := make(chan int, 1)
	go func() {
		code := fakecli.Run("codex", "test", args, inR, outW, &stderr)
		outW.Close()
		codeCh <- code
	}()
	dec := json.NewDecoder(outR)
	readUntil := func(match func(map[string]any) bool) {
		for {
			var msg map[string]any
			if err := dec.Decode(&msg); err != nil {
				t.Fatalf("codex decode: %v (stderr: %s)", err, stderr.String())
			}
			if match(msg) {
				return
			}
		}
	}
	fmt.Fprintln(inW, `{"method":"initialize","id":1,"params":{"clientInfo":{"name":"test","version":"0"}}}`)
	readUntil(func(m map[string]any) bool { return m["id"] == 1.0 })
	readUntil(func(m map[string]any) bool { return m["method"] == "remoteControl/status/changed" })
	fmt.Fprintln(inW, `{"method":"initialized"}`)
	fmt.Fprintf(inW, "{\"method\":\"thread/resume\",\"id\":2,\"params\":{\"threadId\":%q}}\n", thread)
	readUntil(func(m map[string]any) bool { return m["id"] == 2.0 })
	readUntil(func(m map[string]any) bool { return m["method"] == "thread/tokenUsage/updated" })
	fmt.Fprintf(inW, "{\"method\":\"turn/start\",\"id\":3,\"params\":{\"threadId\":%q,\"input\":[{\"type\":\"text\",\"text\":\"check\"}]}}\n", thread)
	readUntil(func(m map[string]any) bool { return m["method"] == "turn/completed" })
	inW.Close()
	io.Copy(io.Discard, outR)
	if code := <-codeCh; code != 0 || stderr.Len() != 0 {
		t.Fatalf("fake codex exit %d, stderr %q", code, stderr.String())
	}
}

type brokenWriteConn struct {
	net.Conn
	err    error
	closed bool
}

func (b *brokenWriteConn) Write([]byte) (int, error) { return 0, b.err }
func (b *brokenWriteConn) Close() error {
	b.closed = true
	return nil
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestRunShimArgumentsErrorsAndHalfClose(t *testing.T) {
	neverDial := func() (net.Conn, error) {
		t.Fatal("dial called on invalid arguments")
		return nil, nil
	}
	for _, badArgs := range [][]string{
		nil,
		{},
		{"--token", ""},
		{"--token"},
		{"--unknown"},
		{"--token", "tok", "extra"},
	} {
		var out, errBuf bytes.Buffer
		code := RunShim(badArgs, neverDial, strings.NewReader(""), &out, &errBuf)
		if code != 2 || out.Len() != 0 || errBuf.String() != shimUsage+"\n" {
			t.Fatalf("args %q: code %d, out %q, err %q", badArgs, code, out.String(), errBuf.String())
		}
	}

	// Dial failure formats \r and \n onto a single stderr line and returns 1.
	var out, errBuf bytes.Buffer
	code := RunShim([]string{"--token", "t1"}, func() (net.Conn, error) {
		return nil, errors.New("dial failed\r\nsecond line\nthird")
	}, strings.NewReader(""), &out, &errBuf)
	if code != 1 || errBuf.String() != "dial failed  second line third\n" {
		t.Fatalf("dial failure: code %d, err %q", code, errBuf.String())
	}

	// Opening write failure closes conn and returns 1.
	bc := &brokenWriteConn{err: errors.New("broken opening write")}
	errBuf.Reset()
	code = RunShim([]string{"--token", "t1"}, func() (net.Conn, error) { return bc, nil }, strings.NewReader(""), &out, &errBuf)
	if code != 1 || !bc.closed || errBuf.String() != "broken opening write\n" {
		t.Fatalf("write failure: code %d, closed %v, err %q", code, bc.closed, errBuf.String())
	}

	// Output copy failure returns 1.
	c1, c2 := net.Pipe()
	go func() {
		bufio.NewReader(c2).ReadBytes('\n')
		c2.Write([]byte("reply\n"))
		c2.Close()
	}()
	errBuf.Reset()
	inR, inW := io.Pipe()
	defer inW.Close()
	code = RunShim([]string{"--token", "t1"}, func() (net.Conn, error) { return c1, nil }, inR, failWriter{err: errors.New("stdout broken")}, &errBuf)
	if code != 1 || errBuf.String() != "stdout broken\n" {
		t.Fatalf("stdout failure: code %d, err %q", code, errBuf.String())
	}

	// Pipelined Unix socket: stdin closes immediately after writing request,
	// CloseWrite lets the server read EOF and still send its reply back to stdout,
	// and special characters in the token are written without HTML escaping.
	sock := shortSocketPath(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// serverOpening is read once after RunShim returns.
	serverOpening := make(chan string, 1)
	go func() {
		srv, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer srv.Close()
		r := bufio.NewReader(srv)
		first, _ := r.ReadString('\n')
		serverOpening <- first
		body, _ := io.ReadAll(r)
		time.Sleep(20 * time.Millisecond)
		srv.Write([]byte("echo:" + string(body)))
	}()

	out.Reset()
	errBuf.Reset()
	code = RunShim([]string{"--token", "tok<&>"}, func() (net.Conn, error) {
		return net.Dial("unix", sock)
	}, strings.NewReader("ping\n"), &out, &errBuf)
	if code != 0 || errBuf.Len() != 0 || out.String() != "echo:ping\n" {
		t.Fatalf("pipelined unix shim: code %d, out %q, err %q", code, out.String(), errBuf.String())
	}
	if gotOpen := <-serverOpening; gotOpen != "{\"mcp\":\"tok<&>\"}\n" {
		t.Fatalf("opening line = %q", gotOpen)
	}

	// net.Pipe has no CloseWrite; closing stdin falls back to conn.Close and returns 0.
	p1, p2 := net.Pipe()
	go func() {
		io.Copy(io.Discard, p2)
		p2.Close()
	}()
	out.Reset()
	errBuf.Reset()
	code = RunShim([]string{"--token", "pipe-tok"}, func() (net.Conn, error) { return p1, nil }, strings.NewReader("bye\n"), &out, &errBuf)
	if code != 0 || errBuf.Len() != 0 {
		t.Fatalf("pipe fallback: code %d, err %q", code, errBuf.String())
	}
}
