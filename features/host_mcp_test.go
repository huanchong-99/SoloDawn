//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	stepSets["host_mcp"] = func(sc *godog.ScenarioContext, f fixtures) {
		hostMCPSteps(sc, f.t)
	}
}

type mcpHarness struct {
	t        *testing.T
	db       *store.Store
	h        *host.Host
	key      host.SessionKey
	sockPath string
	token    string
	session  *mcp.ClientSession
	listed   *mcp.ListToolsResult
	shimCode int
	shimErr  bytes.Buffer
}

func hostMCPSteps(sc *godog.ScenarioContext, t *testing.T) {
	var st *mcpHarness
	cleanup := afterScenario(sc)

	resetTools := func() {
		saved := maps.Clone(host.Tools)
		host.Tools = map[string]host.Tool{}
		cleanup(func() { host.Tools = saved })
	}

	sc.Step(`^a host with an active task session at level (\d+) and a local MCP socket$`, func(level int) error {
		resetTools()
		st = &mcpHarness{t: t, key: host.SessionKey{Group: "g1", Member: "m7", Task: "t12"}}
		return st.start(cleanup, level)
	})
	sc.Step(`^MCP tools "([^"]+)" for all sessions and "([^"]+)" for task sessions above level 1$`, func(echoName, checkName string) {
		host.Tools[echoName] = host.Tool{
			Description: "echo text",
			Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
			Call: func(_ context.Context, h *host.Host, c host.Caller, args json.RawMessage) (any, error) {
				var in struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return nil, err
				}
				return map[string]any{
					"caller": c.Name,
					"echo":   in.Text,
					"open":   h.Sessions.Calls(c.SessionKey),
				}, nil
			},
		}
		host.Tools[checkName] = host.Tool{
			Description: "checks above level 1",
			For:         func(c host.Caller) bool { return c.Kind == host.TaskSession && c.Level > 1 },
			Call: func(context.Context, *host.Host, host.Caller, json.RawMessage) (any, error) {
				return map[string]bool{"ok": true}, nil
			},
		}
	})
	sc.Step(`^MCP failure tools "([^"]+)" and "([^"]+)"$`, func(conflictName, crashName string) {
		host.Tools[conflictName] = host.Tool{
			Call: func(context.Context, *host.Host, host.Caller, json.RawMessage) (any, error) {
				return nil, &host.Error{Code: "conflict", Message: "the worktree has uncommitted changes"}
			},
		}
		host.Tools[crashName] = host.Tool{
			Call: func(context.Context, *host.Host, host.Caller, json.RawMessage) (any, error) {
				return nil, errors.New("sqlite disk error")
			},
		}
	})
	sc.Step(`^an MCP client connects through the shim with the session's token$`, func() error {
		session, err := st.connectClient(cleanup, st.token)
		if err != nil {
			return err
		}
		st.session = session
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		listed, err := session.ListTools(ctx, nil)
		if err != nil {
			return ctxCallErr(ctx, start, "MCP ListTools", nil, err)
		}
		st.listed = listed
		return nil
	})
	sc.Step(`^the server lists "([^"]+)" and "([^"]+)" with object schemas$`, func(first, second string) error {
		return st.expectListed(first, second)
	})
	sc.Step(`^the server lists only "([^"]+)"$`, func(only string) error {
		return st.expectListed(only)
	})
	sc.Step(`^calling "([^"]+)" with "([^"]*)" returns '([^']*)'$`, func(name, input, want string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		res, err := st.session.CallTool(ctx, &mcp.CallToolParams{
			Name:      name,
			Arguments: map[string]any{"text": input},
		})
		if err != nil {
			return ctxCallErr(ctx, start, "MCP tool "+name, nil, err)
		}
		if res.IsError || len(res.Content) != 1 {
			return fmt.Errorf("got result %+v", res)
		}
		text, ok := res.Content[0].(*mcp.TextContent)
		if !ok || text.Text != want {
			return fmt.Errorf("got %+v, want %q", res.Content[0], want)
		}
		return nil
	})
	sc.Step(`^no MCP tool call remains open on the session$`, func() error {
		if open := st.h.Sessions.Calls(st.key); open != 0 {
			return fmt.Errorf("%d tool calls remain open", open)
		}
		return nil
	})
	sc.Step(`^calling "([^"]+)" is refused by the MCP server$`, func(name string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		if _, err := st.session.CallTool(ctx, &mcp.CallToolParams{Name: name}); err == nil {
			return fmt.Errorf("tool %s unexpectedly succeeded", name)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ctxEnded(ctx, start, "MCP refusal of "+name, nil)
		}
		return nil
	})
	sc.Step(`^calling "([^"]+)" returns tool error "([^"]*)"$`, func(name, want string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		res, err := st.session.CallTool(ctx, &mcp.CallToolParams{Name: name})
		if err != nil {
			return ctxCallErr(ctx, start, "MCP tool "+name, nil, err)
		}
		if !res.IsError || len(res.Content) != 1 {
			return fmt.Errorf("expected tool error, got %+v", res)
		}
		text, ok := res.Content[0].(*mcp.TextContent)
		if !ok || text.Text != want {
			return fmt.Errorf("got %+v, want %q", res.Content[0], want)
		}
		return nil
	})
	sc.Step(`^the token state is "([^"]+)"$`, func(state string) error {
		switch state {
		case "unknown":
			st.token = "unknown-session-token"
		case "forgotten":
			st.h.Sessions.Forget(st.token)
		case "ended":
			return st.h.Sessions.End(st.key)
		}
		return nil
	})
	sc.Step(`^an MCP client connecting through the shim fails to initialise$`, func() error {
		if session, err := st.connectClient(cleanup, st.token); err == nil {
			session.Close()
			return errors.New("MCP client unexpectedly initialised with an invalid token")
		}
		return nil
	})
	sc.Step(`^the MCP shim is invoked with "([^"]*)"$`, func(raw string) {
		st = &mcpHarness{t: t}
		dial := func() (net.Conn, error) {
			return nil, errors.New("dial should not be called on invalid arguments")
		}
		st.shimCode = host.RunShim(strings.Fields(raw), dial, strings.NewReader(""), io.Discard, &st.shimErr)
	})
	sc.Step(`^the shim exits with code (\d+) and prints "([^"]*)" on stderr$`, func(wantCode int, wantErr string) error {
		if st.shimCode != wantCode || st.shimErr.String() != wantErr+"\n" {
			return fmt.Errorf("exit %d, stderr %q; want %d and %q", st.shimCode, st.shimErr.String(), wantCode, wantErr)
		}
		return nil
	})
}

func (m *mcpHarness) start(cleanup func(func()), level int) error {
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "sdm")
	if err != nil {
		return err
	}
	cleanup(func() { os.RemoveAll(root) })
	local, err := node.ListenLocal(filepath.Join(root, "solodawn"))
	if err != nil {
		return err
	}
	cleanup(func() { local.Close() })
	m.sockPath = filepath.Join(root, "solodawn", "sock")

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	m.db = db

	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, time.Now)
	if err != nil {
		return err
	}
	m.h = h
	err = h.Sessions.Create(host.Session{
		SessionKey: m.key,
		Kind:       host.TaskSession,
		Folder:     "wt/12-mcp",
		Agent:      "claude",
		Home:       "home-device",
		Name:       "Claude Code",
	})
	if err != nil {
		return err
	}
	if err := h.Sessions.Move(m.key, host.Dormant, host.Working); err != nil {
		return err
	}
	m.token = h.Sessions.Admit(m.key, level, false)

	ctx, cancel := context.WithCancel(context.Background())
	cleanup(cancel)
	go acceptLocalMCP(ctx, local, h)
	return nil
}

func acceptLocalMCP(ctx context.Context, local *node.Local, h *host.Host) {
	for {
		conn, err := local.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			open, _, r, err := node.AcceptOpening(c)
			if err != nil {
				return
			}
			if open.MCP == "" {
				c.Close()
				return
			}
			h.ServeMCP(ctx, open.MCP, r, c)
		}(conn)
	}
}

func (m *mcpHarness) connectClient(cleanup func(func()), token string) (*mcp.ClientSession, error) {
	clientR, shimW := io.Pipe()
	shimR, clientW := io.Pipe()
	// done is read by the cleanup function below once RunShim returns.
	done := make(chan struct{})
	dial := func() (net.Conn, error) {
		return net.Dial("unix", m.sockPath)
	}
	go func() {
		defer close(done)
		defer shimW.Close()
		var stderr bytes.Buffer
		host.RunShim([]string{"--token", token}, dial, shimR, shimW, &stderr)
	}()
	cleanup(func() {
		clientW.Close()
		clientR.Close()
		<-done
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	client := mcp.NewClient(&mcp.Implementation{Name: "acceptance-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientR, Writer: clientW}, nil)
	if err != nil {
		return nil, ctxCallErr(ctx, start, "MCP connect", nil, err)
	}
	cleanup(func() { session.Close() })
	return session, nil
}

func (m *mcpHarness) expectListed(want ...string) error {
	var got []string
	for _, tool := range m.listed.Tools {
		if err := checkObjectSchema(tool.Name, tool.InputSchema); err != nil {
			return err
		}
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	wantSorted := slices.Clone(want)
	slices.Sort(wantSorted)
	if !slices.Equal(got, wantSorted) {
		return fmt.Errorf("listed %v, want %v", got, wantSorted)
	}
	return nil
}

func checkObjectSchema(name string, raw any) error {
	schema, ok := raw.(map[string]any)
	if !ok || schema["type"] != "object" || schema["additionalProperties"] != false {
		return fmt.Errorf("tool %s schema is not a closed object: %#v", name, raw)
	}
	if _, hasProps := schema["properties"].(map[string]any); !hasProps {
		return fmt.Errorf("tool %s schema lacks properties: %#v", name, schema)
	}
	return nil
}
