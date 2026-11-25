package node

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestLocalGroupTransfer(t *testing.T) {
	owner, old, next := testSeedKey(1), testSeedKey(2), testSeedKey(3)
	entries := append(testOwnerLog(t, owner, old).Entries, testDeviceAdd(owner, 3, "node", "next", next.ID()))
	newer := append(entries, testDeviceAdd(owner, 4, "node", "extra", testSeedKey(4).ID()))
	src := newClientNodeRig(t, "old", old, owner, newer, home.Table{"group/create": testTransferGroup})
	dst := newClientNodeRig(t, "next", next, owner, entries, home.Table{"group/create": testTransferGroup})
	group, other := string(sesID("transfer-group")), string(sesID("other-group"))
	seedGroupWithHostedAI(t, src, group)
	seedGroupWithHostedAI(t, dst, other)
	source := &nodeRuntime{loop: src.loop, logs: src.logs}
	dest := &nodeRuntime{loop: dst.loop, logs: dst.logs, sessions: dst.sessions}
	path := filepath.Join(t.TempDir(), "group.sqlite")
	target := src.loop.Attach(owner.ID(), nilClient{})
	t.Cleanup(func() { src.loop.Detach(target) })
	mustEqual(t, len(src.loop.Groups(target)), 1)
	serveGroupCLI(t, source)
	var stdout, stderr bytes.Buffer
	mustEqual(t, groupCommand([]string{"export", group, path, "--to", next.ID()}, &stdout, &stderr), 0)
	mustEqual(t, stderr.String(), "")
	mustEqual(t, len(src.loop.Groups(target)), 0)
	assertMovedRequests(t, src, group, next.ID())
	mustNoErr(t, sendGroupCmd(dest, Command{Cmd: "group.import", File: path}))
	mustEqual(t, len(dst.logs.Log(owner.ID()).Entries), 4)
	mustNoErr(t, dst.loop.ExportGroup(other, filepath.Join(t.TempDir(), "copy.sqlite"), "")) // after the device event
	checkSingleString(t, dst.db, "SELECT id FROM row WHERE g='"+other+"' AND r='device'", owner.ID()+":4")
	local := AttachLocal(dst.host, dst.loop)
	t.Cleanup(func() { local.Close() })
	answer, err := local.Request(t.Context(), func(id int64) any {
		return proto.FetchRow{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group, Limit: 1}, Row: "group", RowID: group}
	})
	mustNoErr(t, err)
	var fetched proto.Fetched
	mustNoErr(t, proto.Decode(answer, &fetched))
	var row proto.GroupRow
	mustNoErr(t, proto.Decode(fetched.Rows[0], &row))
	mustEqual(t, row.Home, next.ID())
	mustEqual(t, row.Name, "Moved group")
	mustNoErr(t, sendGroupCmd(source, Command{Cmd: "group.import", File: path}))
	mustEqual(t, len(src.loop.Groups(target)), 1)
}

type nilClient struct{}

func (nilClient) Done() <-chan struct{} { return nil }
func (nilClient) Send(any) error        { return nil }

func testTransferGroup(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	group := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Moved group", Checks: []string{}, Created: c.Now, Home: s.Home}
	owner := proto.HumanMemberRow{MemberRow: proto.MemberRow{
		RowHeader: proto.RowHeader{Type: "member", ID: string(sesID("transfer-owner"))}, Group: s.ID, Kind: "human", User: s.Owner,
	}, Role: "owner"}
	return home.Result{Rows: []home.Row{home.RowOf(group), home.RowOf(owner)}, GroupsChanged: true}, nil
}

func sendGroupCmd(n *nodeRuntime, cmd Command) error {
	raw, err := encodeCtl(cmd)
	if err != nil {
		return err
	}
	_, err = n.runLocalCmd(n.ctx, cmd.Cmd, raw)
	return err
}

func assertMovedRequests(t *testing.T, rig *clientNodeRig, group, to string) {
	t.Helper()
	local := AttachLocal(rig.host, rig.loop)
	t.Cleanup(func() { local.Close() })
	for _, build := range []func(int64) any{
		func(id int64) any {
			payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q}`, sesID("move-cmd"), localClockMs, group)
			return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: rig.key.Sign(payload)}
		},
		func(id int64) any {
			return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: group}
		},
		func(id int64) any {
			return proto.FetchRow{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group, Limit: 1}, Row: "group", RowID: group}
		},
	} {
		raw, err := local.Request(t.Context(), build)
		mustNoErr(t, err)
		var moved proto.ErrorResponse
		mustNoErr(t, proto.Decode(raw, &moved))
		mustEqual(t, moved.Err, proto.WireError{Code: "moved", Message: "The group has moved.", Home: to})
	}
}

func TestAIConnectionFollowsGroupMove(t *testing.T) {
	owner, old, next, ai := testSeedKey(1), testSeedKey(2), testSeedKey(3), testSeedKey(4)
	entries := append(testOwnerLog(t, owner, old).Entries,
		testDeviceAdd(owner, 3, "node", "next", next.ID()),
		testDeviceAdd(owner, 4, "node", "ai", ai.ID()))
	src := newClientNodeRig(t, "old", old, owner, entries, home.Table{"group/create": testTransferGroup})
	aiRig := newClientNodeRig(t, "ai", ai, owner, entries, nil)
	group := string(sesID("AI-move-group"))
	seedGroupWithHostedAI(t, src, group)
	key := host.SessionKey{Group: group, Member: string(sesID("AI-move-member"))}
	sess := host.Session{SessionKey: key, Home: old.ID(), Kind: host.GroupSession, Folder: "repo", Name: "AI"}
	mustNoErr(t, aiRig.host.Sessions.Create(sess))
	connected := make(chan string, 3)
	aiRig.host.Connect = func(_ context.Context, device string) { connected <- device }
	mustNoErr(t, aiRig.host.RecordHome(proto.Home{Group: proto.ID(group), Home: old.ID(), Name: "Moved group"}, []string{"wss://example.test/rv"}))
	mustNoErr(t, aiRig.host.EnsureHomeConnections())
	mustEqual(t, recvFrame(t, connected), old.ID())
	t.Cleanup(aiRig.host.Settle)
	startPairedSession(t, src, aiRig)
	path := filepath.Join(t.TempDir(), "group.sqlite")
	mustNoErr(t, src.loop.ExportGroup(group, path, next.ID()))
	mustEqual(t, recvFrame(t, connected), next.ID())
	saved, ok := aiRig.host.Sessions.Get(key)
	mustEqual(t, ok, true)
	mustEqual(t, saved.Home, next.ID())
	mustEqual(t, saved.Folder, "repo")
	checkSingleString(t, aiRig.db, "SELECT url FROM peer WHERE device='"+next.ID()+"'", "wss://example.test/rv")
}

func TestGroupCLIUsageAndErrors(t *testing.T) {
	var stderr bytes.Buffer
	mustEqual(t, groupCommand([]string{"export"}, io.Discard, &stderr), 2)
	mustEqual(t, stderr.String(), groupUsage.Error()+"\n")
	runtime := &nodeRuntime{}
	dir := serveGroupCLI(t, runtime)
	stderr.Reset()
	mustEqual(t, groupCommand([]string{"import", filepath.Join(dir, "missing.sqlite")}, io.Discard, &stderr), 1)
	if stderr.Len() == 0 {
		t.Fatal("missing SQLite import error")
	}
	path := filepath.Join(dir, "export.sqlite")
	to := testSeedKey(3).ID()
	got, err := groupArgs([]string{"export", localGroupID, path, "--to", to})
	mustNoErr(t, err)
	mustEqual(t, got, Command{Cmd: "group.export", Group: localGroupID, File: path, To: to})
	for _, args := range [][]string{
		{"import", ""},
		{"export", localGroupID, path, "--to"},
		{"export", localGroupID, path, "--node", to},
		{"export", localGroupID, path, "--to", "not a device"},
	} {
		_, err := groupArgs(args)
		mustEqual(t, err, groupUsage)
	}
	mustEqual(t, runtime.cmdGroup([]byte(`{"cmd":"group.import","file":"relative"}`)).Error(), "invalid group transfer arguments")
	mustEqual(t, runtime.exportGroup(Command{}).Error(), "invalid group or destination id")
}

func serveGroupCLI(t *testing.T, runtime *nodeRuntime) string {
	t.Helper()
	runtime.ctx = t.Context()
	dir := nodeTestDir(t)
	socket := openSocket(t, dir)
	go func() {
		conn, err := socket.Accept()
		if err == nil {
			runtime.handleLocal(conn)
		}
	}()
	return dir
}
