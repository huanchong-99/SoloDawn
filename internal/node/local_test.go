package node

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	localClockMs = int64(1_763_600_000_000)
	localGroupID = "AAAAAAAAAAAAAAAAAAAAAQ"
)

type localRig struct {
	db     *store.Store
	owner  identity.Key
	device identity.Key
	loop   *home.Loop
	host   *host.Host
	seq    int64
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func testSeedKey(tag byte) identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = tag
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}

func testNonce(n int64) string {
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], uint64(n))
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func testDeviceAdd(owner identity.Key, ver int, kind, name, id string) proto.Envelope {
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: proto.ID(testNonce(int64(ver))), Timestamp: localClockMs},
		DeviceChange:  proto.DeviceChange{User: owner.ID(), Version: ver, Op: "add", Device: id, Kind: kind, Name: name},
	}
	raw, _ := json.Marshal(entry)
	return owner.Sign(string(raw))
}

func testOwnerLog(t *testing.T, owner, dev identity.Key) *identity.Log {
	t.Helper()
	log, err := identity.Replay(owner.ID(), []proto.Envelope{
		testDeviceAdd(owner, 1, "browser", "owner", owner.ID()),
		testDeviceAdd(owner, 2, "node", "workstation", dev.ID()),
	})
	mustNoErr(t, err)
	return log
}

func testGroupCreate(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Name string `json:"name"`
	}
	if ref := c.Decode(&p); ref != nil {
		return home.Result{}, ref
	}
	grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Checks: []string{}, Created: c.Now}
	mem := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-owner"}, Group: s.ID, Kind: "human", Name: "Owner", User: s.Owner},
		Role:      "admin",
	}
	return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(mem)}, Answer: store.Outcome{Num: 7}, GroupsChanged: true}, nil
}

func testNoteCmd(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Text string `json:"text"`
	}
	if ref := c.Decode(&p); ref != nil {
		return home.Result{}, ref
	}
	if p.Text == "refuse" {
		return home.Result{}, &home.Refusal{Code: "conflict", Msg: "note refused"}
	}
	row := proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: string(c.Nonce)}, Group: s.ID, Text: p.Text, By: "m-owner", Timestamp: c.Now}
	return home.Result{Rows: []home.Row{home.RowOf(row)}}, nil
}

func newLocalRig(t *testing.T, extra map[string]home.Func) *localRig {
	t.Helper()
	db, err := store.Open(":memory:")
	mustNoErr(t, err)
	t.Cleanup(func() { db.Close() })

	owner, dev := testSeedKey(1), testSeedKey(2)
	now := func() time.Time { return time.UnixMilli(localClockMs) }
	cmds := home.Table{"group/create": testGroupCreate, "note": testNoteCmd}
	for k, fn := range extra {
		cmds[k] = fn
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store:  db,
		Home:   dev.ID(),
		Owner:  owner.ID(),
		Logs:   func(user string) *identity.Log { return testLogFor(t, user, owner, dev) },
		Tables: home.Tables{Commands: cmds},
		Now:    now,
	})
	mustNoErr(t, err)
	t.Cleanup(loop.Close)

	h, _, err := host.New(db, dev, now)
	mustNoErr(t, err)
	return &localRig{db: db, owner: owner, device: dev, loop: loop, host: h}
}

func testLogFor(t *testing.T, user string, owner, dev identity.Key) *identity.Log {
	if user == owner.ID() {
		return testOwnerLog(t, owner, dev)
	}
	return nil
}

func (r *localRig) payload(fields string) string {
	r.seq++
	return fmt.Sprintf(`{"n":%q,"ts":%d,"g":%q,%s}`, testNonce(100+r.seq), localClockMs, localGroupID, fields)
}

type pushCapture struct {
	resets   chan proto.Reset
	rowsRaw  chan string
	rows     chan proto.Rows
	attached chan string
}

func capturePushes(t *testing.T) *pushCapture {
	t.Helper()
	// Single reader per channel: the test that called capturePushes.
	cap := &pushCapture{
		resets:   make(chan proto.Reset, 4),
		rowsRaw:  make(chan string, 4),
		rows:     make(chan proto.Rows, 4),
		attached: make(chan string, 2),
	}
	host.Pushes["reset"] = func(_ *host.Host, from string, raw json.RawMessage) {
		var frame proto.Reset
		if from != "" && proto.Decode(raw, &frame) == nil {
			cap.resets <- frame
		}
	}
	host.Pushes["rows"] = func(_ *host.Host, from string, raw json.RawMessage) {
		var frame proto.Rows
		if from != "" && proto.Decode(raw, &frame) == nil {
			cap.rowsRaw <- string(raw)
			cap.rows <- frame
		}
	}
	host.Attached["local_test"] = func(_ *host.Host, device string) {
		cap.attached <- device
	}
	t.Cleanup(func() {
		delete(host.Pushes, "reset")
		delete(host.Pushes, "rows")
		delete(host.Attached, "local_test")
	})
	return cap
}

func TestLocalLinkCommandAndPushCrossLink(t *testing.T) {
	cap := capturePushes(t)
	rig := newLocalRig(t, nil)
	link := AttachLocal(rig.host, rig.loop)

	mustEqual(t, link.Path(), "local")
	mustEqual(t, link.PathChanged(), (<-chan struct{})(nil))
	homeConn, ok := rig.host.Home(rig.device.ID())
	mustEqual(t, []any{homeConn, ok}, []any{host.Home(link), true})
	mustEqual(t, recvFrame(t, cap.attached), rig.device.ID())

	ctx := context.Background()
	ans, err := rig.host.Command(ctx, rig.device.ID(), rig.payload(`"t":"group","op":"create","name":"Alpha"`))
	mustNoErr(t, err)
	var created proto.CmdResult
	mustNoErr(t, proto.Decode(ans, &created))
	mustEqual(t, created.Num, 7)

	checkLocalSync(t, ctx, link, cap)

	_, err = rig.host.Command(ctx, rig.device.ID(), rig.payload(`"t":"note","text":"hello <&>"`))
	mustNoErr(t, err)
	checkNotePush(t, recvFrame(t, cap.rowsRaw), recvFrame(t, cap.rows))

	_, err = rig.host.Command(ctx, rig.device.ID(), rig.payload(`"t":"note","text":"refuse"`))
	var hostErr *host.Error
	mustEqual(t, errors.As(err, &hostErr), true)
	mustEqual(t, *hostErr, host.Error{Code: "conflict", Message: "note refused"})
}

func checkLocalSync(t *testing.T, ctx context.Context, link *LocalLink, cap *pushCapture) {
	t.Helper()
	ans, err := link.Request(ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: localGroupID, Since: 0, StateOnly: false}
	})
	mustNoErr(t, err)
	var synced proto.Synced
	mustNoErr(t, proto.Decode(ans, &synced))
	mustEqual(t, []any{synced.Group, synced.Seq}, []any{localGroupID, int64(2)})
	mustEqual(t, recvFrame(t, cap.resets).Group, localGroupID)
	recvFrame(t, cap.rowsRaw)
	initial := recvFrame(t, cap.rows)
	mustEqual(t, []any{initial.Group, len(initial.Rows)}, []any{localGroupID, 2})

	missAns, err := link.Request(ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: "missing-group", Since: 0, StateOnly: true}
	})
	mustNoErr(t, err)
	var missErr proto.ErrorResponse
	mustNoErr(t, proto.Decode(missAns, &missErr))
	mustEqual(t, missErr.Err.Code, "not_found")
}

func checkNotePush(t *testing.T, raw string, frame proto.Rows) {
	t.Helper()
	mustEqual(t, frame.Group, localGroupID)
	mustEqual(t, len(frame.Rows), 1)
	mustEqual(t, strings.Contains(raw, `"hello <&>"`), true)
	mustEqual(t, strings.Contains(raw, "\n"), false)
	var note proto.NoteRow
	mustNoErr(t, proto.Decode(frame.Rows[0], &note))
	mustEqual(t, note.Text, "hello <&>")
}

func TestLocalLinkInboxForGroupCreatedAfterAttach(t *testing.T) {
	rig := newLocalRig(t, map[string]home.Func{
		"jump": func(s *home.State, _ home.Command) (home.Result, *home.Refusal) {
			ai := proto.AIMemberRow{
				MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-ai"}, Group: s.ID, Kind: "ai", Name: "Coder", User: s.Owner},
				Host:      s.Home,
			}
			return home.Result{Rows: []home.Row{home.RowOf(ai)}, Items: []home.Item{{AI: "m-ai", Why: "mention"}}}, nil
		},
	})
	link := AttachLocal(rig.host, rig.loop)

	type pushedFrame struct {
		from string
		raw  json.RawMessage
	}
	// Single reader: this test.
	pushed := make(chan pushedFrame, 2)
	origPush := link.push
	link.push = func(from string, raw json.RawMessage) {
		origPush(from, raw)
		pushed <- pushedFrame{from: from, raw: raw}
	}

	ctx := context.Background()
	_, err := rig.host.Command(ctx, rig.device.ID(), rig.payload(`"t":"group","op":"create","name":"AfterAttach"`))
	mustNoErr(t, err)
	_, err = rig.host.Command(ctx, rig.device.ID(), rig.payload(`"t":"jump"`))
	mustNoErr(t, err)

	got := recvFrame(t, pushed)
	mustEqual(t, got.from, rig.device.ID())
	var items home.ItemsPush
	mustNoErr(t, json.Unmarshal(got.raw, &items))
	mustEqual(t, items, home.ItemsPush{
		Group: localGroupID,
		Items: []home.QueuedItem{{ID: 1, Item: home.Item{AI: "m-ai", Why: "mention"}}},
	})
}

func TestLocalLinkFraming(t *testing.T) {
	cap := capturePushes(t)
	rig := newLocalRig(t, nil)
	link := AttachLocal(rig.host, rig.loop)
	ctx := context.Background()

	mustEqual(t, link.Send(math.NaN()) != nil, true)
	buildNaN := func(int64) any { return math.NaN() }
	_, nanErr := link.Request(ctx, buildNaN)
	mustEqual(t, nanErr != nil, true)

	for _, raw := range []string{`"scalar"`, `{"id":"not-int"}`, `{"t":"ack","g":"` + localGroupID + `","item":1}`} {
		mustNoErr(t, link.Send(json.RawMessage(raw)))
	}

	checkBadRequests(t, rig, link)
	checkClientInbound(t, link, cap)
}

func checkBadRequests(t *testing.T, rig *localRig, link *LocalLink) {
	t.Helper()
	ctx := context.Background()
	_, err := rig.host.Command(ctx, rig.device.ID(), rig.payload(`"t":"group","op":"create","name":"Gamma"`))
	mustNoErr(t, err)

	env := rig.device.Sign(rig.payload(`"t":"note","text":"x"`))
	var ids []int64
	cases := []func(int64) any{
		func(id int64) any { ids = append(ids, id); return map[string]any{"t": "cmd", "id": id} },
		func(id int64) any { ids = append(ids, id); return map[string]any{"t": "nope", "id": id, "env": env} },
		func(id int64) any { ids = append(ids, id); return map[string]any{"t": "sync", "id": id} },
		func(id int64) any {
			ids = append(ids, id)
			return map[string]any{"t": "nope", "id": id, "g": localGroupID, "since": 0, "state_only": false}
		},
		func(id int64) any { ids = append(ids, id); return map[string]any{"id": id} },
	}
	for _, build := range cases {
		ans, err := link.Request(ctx, build)
		mustNoErr(t, err)
		var errResp proto.ErrorResponse
		mustNoErr(t, proto.Decode(ans, &errResp))
		mustEqual(t, errResp.Err.Code, "bad_request")
	}
	mustEqual(t, ids, []int64{3, 4, 5, 6, 7})
}

func checkClientInbound(t *testing.T, link *LocalLink, cap *pushCapture) {
	t.Helper()
	client := localClient{link}
	mustEqual(t, client.Done(), (<-chan struct{})(nil))
	mustNoErr(t, client.Send(map[string]any{"re": "bad", "t": "rows"}))
	mustNoErr(t, client.Send(map[string]any{"re": 999, "ok": true, "t": "rows"}))
	mustEqual(t, len(cap.rows), 0)
	mustEqual(t, client.Send(math.NaN()) != nil, true)
}

func TestLocalLinkCancellationAndClose(t *testing.T) {
	// Single reader per channel: the blocking jump command and this test.
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	rig := newLocalRig(t, map[string]home.Func{
		"jump": func(_ *home.State, _ home.Command) (home.Result, *home.Refusal) {
			entered <- struct{}{}
			<-release
			return home.Result{}, nil
		},
	})
	link := AttachLocal(rig.host, rig.loop)
	_, err := rig.host.Command(context.Background(), rig.device.ID(), rig.payload(`"t":"group","op":"create","name":"Beta"`))
	mustNoErr(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancelErr := startJumpRequest(ctx, rig, link)
	<-entered
	cancel()
	mustEqual(t, errors.Is(<-cancelErr, context.Canceled), true)
	release <- struct{}{}

	_, err = rig.host.Command(context.Background(), rig.device.ID(), rig.payload(`"t":"note","text":"after-cancel"`))
	mustNoErr(t, err)

	closeErr := startJumpRequest(context.Background(), rig, link)
	<-entered
	mustNoErr(t, link.Close())
	mustEqual(t, <-closeErr, io.EOF)
	close(release)

	checkClosedLink(t, rig, link)
}

func startJumpRequest(ctx context.Context, rig *localRig, link *LocalLink) <-chan error {
	// Single reader: caller of startJumpRequest.
	done := make(chan error, 1)
	env := rig.device.Sign(rig.payload(`"t":"jump"`))
	go func() {
		build := func(id int64) any {
			return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
		}
		_, err := link.Request(ctx, build)
		done <- err
	}()
	return done
}

func checkClosedLink(t *testing.T, rig *localRig, link *LocalLink) {
	t.Helper()
	mustEqual(t, link.isClosed(), true)
	_, open := rig.host.Home(rig.device.ID())
	mustEqual(t, open, false)
	mustEqual(t, link.Send(map[string]string{"t": "pad"}), io.EOF)
	build := func(id int64) any { return map[string]any{"t": "cmd", "id": id} }
	_, err := link.Request(context.Background(), build)
	mustEqual(t, err, io.EOF)
}

func recvFrame[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatal("timed out waiting for frame")
		return zero
	}
}
