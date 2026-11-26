package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
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
func (c *netStubCtl) Finish()                                       {}

func TestNetStatusToolRegistrationAndSchema(t *testing.T) {
	tool, ok := host.Tools["net_status"]
	if !ok || tool.Description == "" || tool.For != nil || tool.Call == nil {
		t.Fatalf("net_status registration = %+v (ok=%v)", tool, ok)
	}
	checkNetStatusGoldenSchema(t, tool.Schema)
	checkNetStatusInvalidArgs(t, tool)
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

func TestNetStatusDefaultHost(t *testing.T) {
	h, _, err := host.New(sesOpenStore(t), sesKey("ns-default"), time.Now)
	mustNoErr(t, err)
	want := netStatusResult{Paths: []netPath{}, Rendezvous: []netRendezvous{}}
	if got := callNetStatusDecoded(t, h); !reflect.DeepEqual(got, want) {
		t.Fatalf("default = %+v, want %+v", got, want)
	}
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

func TestNetStatusPathsSorting(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, ownerKey := sesKey("ns-ses-node"), sesKey("ns-ses-owner")
	logs := seedOwnedLogs(t, db, nodeKey, ownerKey)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, time.Now(), home.Commands)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})

	addTestSesConn(s, "dev-unwelcomed", false, newNetStubCtl("direct"))
	addTestSesConn(s, "dev-empty-path", true, newNetStubCtl(""))
	addTestSesConn(s, "dev-b", true, newNetStubCtl("direct"))
	addTestSesConn(s, "dev-c", true, newNetStubCtl("relay"))
	addTestSesConn(s, "dev-a", true, newNetStubCtl("relay"))
	addTestSesConn(s, "dev-a", true, newNetStubCtl("direct"))

	want := []netPath{
		{Peer: "dev-a", Path: "direct"},
		{Peer: "dev-a", Path: "relay"},
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
	if err := n.waitRVPath(rvBad); err == nil {
		t.Fatal("waitRVPath of a rendezvous that refuses the dial = nil")
	}

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

}
