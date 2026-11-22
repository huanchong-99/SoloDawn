package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type netStubCtl struct {
	path string
	done chan struct{} // closed on Close; read through Done
}

func newNetStubCtl(path string) *netStubCtl {
	return &netStubCtl{path: path, done: make(chan struct{})}
}

func (c *netStubCtl) Send(any) error                                { return nil }
func (c *netStubCtl) Path() string                                  { return c.path }
func (c *netStubCtl) PathChanged() <-chan struct{}                  { return nil }
func (c *netStubCtl) Done() <-chan struct{}                         { return c.done }
func (c *netStubCtl) Read(context.Context) (json.RawMessage, error) { return nil, io.EOF }
func (c *netStubCtl) Close() error                                  { return nil }

type netStubRTTCtl struct {
	*netStubCtl
	rtt int
	ok  bool
}

func (c *netStubRTTCtl) RTTMillis() (int, bool) { return c.rtt, c.ok }

func TestNetStatusToolRegistrationAndSchema(t *testing.T) {
	tool, ok := host.Tools["net_status"]
	if !ok || tool.Description == "" || tool.For != nil || tool.Call == nil {
		t.Fatalf("net_status registration = %+v (ok=%v)", tool, ok)
	}
	checkNetStatusGoldenSchema(t, tool.Schema)
	checkNetStatusInvalidArgs(t, tool)
	checkNetStatusMCPWire(t)
}

func checkNetStatusGoldenSchema(t *testing.T, got json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	mustNoErr(t, err)
	var lists proto.ToolLists
	mustNoErr(t, decodeStrictJSON(data, &lists))
	idx := slices.IndexFunc(lists.Secretary.Tools, func(tl proto.Tool) bool { return tl.Name == "net_status" })
	if idx < 0 {
		t.Fatal("net_status missing in tools.json")
	}
	var gotInput proto.ToolInput
	mustNoErr(t, decodeStrictJSON(got, &gotInput))
	if !reflect.DeepEqual(gotInput, lists.Secretary.Tools[idx].InputSchema) {
		t.Fatalf("schema = %+v, want %+v", gotInput, lists.Secretary.Tools[idx].InputSchema)
	}
}

func decodeStrictJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func checkNetStatusInvalidArgs(t *testing.T, tool host.Tool) {
	t.Helper()
	db := sesOpenStore(t)
	h, _, err := host.New(db, sesKey("ns-bad-args"), time.Now)
	mustNoErr(t, err)
	for _, raw := range []string{`[]`, `null`, `"text"`, `{`} {
		_, callErr := tool.Call(t.Context(), h, host.Caller{}, json.RawMessage(raw))
		var hostErr *host.Error
		if !errors.As(callErr, &hostErr) || hostErr.Code != "bad_request" {
			t.Fatalf("Call(%s) err = %v, want bad_request", raw, callErr)
		}
	}
}

func checkNetStatusMCPWire(t *testing.T) {
	t.Helper()
	db := sesOpenStore(t)
	h, _, err := host.New(db, sesKey("ns-mcp"), time.Now)
	mustNoErr(t, err)
	key := host.SessionKey{Group: "g1", Member: "m1"}
	mustNoErr(t, h.Sessions.Create(host.Session{SessionKey: key, Kind: host.GroupSession, Folder: "repo", Agent: "codex", Home: "h1", Name: "Codex"}))
	tok := h.Sessions.Admit(key, 3, false)
	defer h.Sessions.Forget(tok)

	okResp := callMCPToolOnce(t, h, tok, `{}`)
	if okResp.IsError {
		t.Fatalf("MCP net_status unexpected error: %s", okResp.Text)
	}
	if len(proto.UnexpectedNulls([]byte(okResp.Text))) != 0 {
		t.Fatalf("unexpected nulls in %s", okResp.Text)
	}
	badResp := callMCPToolOnce(t, h, tok, `{"extra":1}`)
	if !badResp.IsError {
		t.Fatalf("expected MCP schema validation error, got %s", badResp.Text)
	}
}

type mcpCallText struct {
	Text    string
	IsError bool
}

func callMCPToolOnce(t *testing.T, h *host.Host, token, args string) mcpCallText {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1) // single reader: callMCPToolOnce
	go func() { done <- h.ServeMCP(t.Context(), token, inR, outW) }()
	dec := json.NewDecoder(outR)
	writeMCPLine(t, inW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	readMCPResult(t, dec)
	writeMCPLine(t, inW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	writeMCPLine(t, inW, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"net_status","arguments":%s}}`, args))
	res := readMCPResult(t, dec)
	inW.Close()
	outR.Close()
	<-done
	return decodeMCPCallText(t, res)
}

func writeMCPLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	_, err := fmt.Fprintln(w, line)
	mustNoErr(t, err)
}

func readMCPResult(t *testing.T, dec *json.Decoder) json.RawMessage {
	t.Helper()
	var msg struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	mustNoErr(t, dec.Decode(&msg))
	if msg.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", msg.Error)
	}
	return msg.Result
}

func decodeMCPCallText(t *testing.T, raw json.RawMessage) mcpCallText {
	t.Helper()
	var call struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	mustNoErr(t, json.Unmarshal(raw, &call))
	return mcpCallText{Text: call.Content[0].Text, IsError: call.IsError}
}

func TestNetStatusDefaultHost(t *testing.T) {
	savedIfaces := ifaceAddrs
	t.Cleanup(func() { ifaceAddrs = savedIfaces })

	db := sesOpenStore(t)
	h, _, err := host.New(db, sesKey("ns-default"), time.Now)
	mustNoErr(t, err)

	ifaceAddrs = ipv4OnlyAddrs(t)
	gotEmpty := callNetStatusDecoded(t, h)
	wantEmpty := netStatusResult{
		Paths:      []netPath{},
		NAT:        netNAT{UDP: false, MappedPorts: false, IPv6: false},
		Rendezvous: []netRendezvous{},
	}
	if !reflect.DeepEqual(gotEmpty, wantEmpty) {
		t.Fatalf("empty default = %+v, want %+v", gotEmpty, wantEmpty)
	}

	ifaceAddrs = sampleIfaceAddrs(t)
	putRV := func(tx *store.Tx) error {
		_, txErr := tx.Exec(`INSERT INTO setting(key, value) VALUES('rendezvous', '["wss://rv1.example/rv","wss://rv2.example/rv"]')`)
		return txErr
	}
	mustNoErr(t, db.Write(putRV))
	gotWithRV := callNetStatusDecoded(t, h)
	wantWithRV := netStatusResult{
		Paths: []netPath{},
		NAT:   netNAT{UDP: false, MappedPorts: false, IPv6: true},
		Rendezvous: []netRendezvous{
			{URL: "wss://rv1.example/rv", OK: false},
			{URL: "wss://rv2.example/rv", OK: false},
		},
	}
	if !reflect.DeepEqual(gotWithRV, wantWithRV) {
		t.Fatalf("configured default = %+v, want %+v", gotWithRV, wantWithRV)
	}
}

func ipv4OnlyAddrs(t *testing.T) func() ([]net.Addr, error) {
	t.Helper()
	ip, ipnet, err := net.ParseCIDR("192.0.2.10/24")
	mustNoErr(t, err)
	ipnet.IP = ip
	v6ll, v6llNet, err := net.ParseCIDR("fe80::1/64")
	mustNoErr(t, err)
	v6llNet.IP = v6ll
	return func() ([]net.Addr, error) { return []net.Addr{ipnet, v6llNet}, nil }
}

func callNetStatusDecoded(t *testing.T, h *host.Host) netStatusResult {
	t.Helper()
	val, err := host.Tools["net_status"].Call(t.Context(), h, host.Caller{}, json.RawMessage(`{}`))
	mustNoErr(t, err)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	mustNoErr(t, enc.Encode(val))
	raw := bytes.TrimSpace(buf.Bytes())
	if nulls := proto.UnexpectedNulls(raw); len(nulls) != 0 {
		t.Fatalf("unexpected nulls %v in %s", nulls, raw)
	}
	var out netStatusResult
	mustNoErr(t, proto.Decode(raw, &out))
	return out
}

func TestNetStatusPathsSortingAndOptionalRTT(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("ns-ses-node"), sesKey("ns-ses-owner")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, time.Now(), home.Commands)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})

	addTestSesConn(s, "dev-unwelcomed", false, newNetStubCtl("direct"))
	addTestSesConn(s, "dev-empty-path", true, newNetStubCtl(""))
	addTestSesConn(s, "dev-b", true, newNetStubCtl("direct"))
	addTestSesConn(s, "dev-c", true, &netStubRTTCtl{netStubCtl: newNetStubCtl("relay"), rtt: 99, ok: false})
	addTestSesConn(s, "dev-a", true, &netStubRTTCtl{netStubCtl: newNetStubCtl("relay"), rtt: 210, ok: true})
	addTestSesConn(s, "dev-a", true, &netStubRTTCtl{netStubCtl: newNetStubCtl("direct"), rtt: 0, ok: true})

	zero, relayRTT := 0, 210
	want := []netPath{
		{Peer: "dev-a", Path: "direct", RTTMS: &zero},
		{Peer: "dev-a", Path: "relay", RTTMS: &relayRTT},
		{Peer: "dev-b", Path: "direct"},
		{Peer: "dev-c", Path: "relay"},
	}
	got := s.netPaths()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("netPaths = %#v, want %#v", got, want)
	}
}

func addTestSesConn(s *Sessions, device string, welcomed bool, raw Ctl) {
	s.mu.Lock()
	s.conns[&sesConn{device: device, welcomed: welcomed, raw: raw}] = true
	s.mu.Unlock()
}

func TestNetStatusLiveNodeRuntime(t *testing.T) {
	savedIfaces, savedInstall := ifaceAddrs, installService
	t.Cleanup(func() {
		ifaceAddrs = savedIfaces
		installService = savedInstall
	})
	installService = func(string, string) error { return nil }
	ifaceAddrs = sampleIfaceAddrs(t)

	rvGood := startTestRVWithRelay(t)
	rvBad := "ws://127.0.0.1:1/rv"
	dir := socketRoot(t)
	l := openSocket(t, dir)
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	done := make(chan struct{}) // single reader: TestNetStatusLiveNodeRuntime
	go func() {
		n.serve(l)
		close(done)
	}()

	browserKey, rtcConn, _ := pairNodeOverRendezvous(t, dir, n, rvGood, "net-box")
	t.Cleanup(func() { rtcConn.Close() })
	mustNoErr(t, n.saveRendezvous([]string{rvGood, rvBad}))
	n.syncRendezvous()
	mustNoErr(t, n.waitRVPath(rvGood))

	checkLiveNodeNetStatus(t, n, browserKey.ID(), rvGood, rvBad)
	n.Close()
	<-done
}

func checkLiveNodeNetStatus(t *testing.T, n *nodeRuntime, peerID, rvGood, rvBad string) {
	t.Helper()
	waitCond(t, func() bool {
		st := callNetStatusDecoded(t, n.host)
		return len(st.Paths) == 1 && st.Paths[0].Peer == peerID && st.Paths[0].Path != ""
	})
	got := callNetStatusDecoded(t, n.host)
	wantRV := []netRendezvous{{URL: rvGood, OK: true}, {URL: rvBad, OK: false}}
	if !got.NAT.UDP || got.NAT.MappedPorts || !got.NAT.IPv6 || !reflect.DeepEqual(got.Rendezvous, wantRV) {
		t.Fatalf("live net_status = %+v", got)
	}

	savedSocks := n.socks
	n.socks = nil
	noUDP := callNetStatusDecoded(t, n.host)
	n.socks = savedSocks
	if noUDP.NAT.UDP {
		t.Fatalf("expected UDP false without peer sockets, got %+v", noUDP.NAT)
	}
}
