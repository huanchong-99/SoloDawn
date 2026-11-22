package home_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	wpushBrowserOne = device(61)
	wpushBrowserTwo = device(62)
	wpushHomeNode   = device(63)
	wpushHostNode   = device(64)
	wpushGuestDev   = device(65)
	wpushGroupTwo   = id(72)
	wpushGuestMem   = id(74)
	wpushAIMem      = id(76)
	wpushTaskID     = id(77)
)

type wpushBrowserKeys struct {
	uaPriv *ecdh.PrivateKey
	auth   []byte
	sub    proto.PushSubscription
	vapid  proto.VAPIDKeys
}

func wpushP256(seed byte) (*ecdh.PrivateKey, string, string) {
	priv, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{seed}, 32))
	if err != nil {
		panic(err)
	}
	return priv, base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(priv.Bytes())
}

func wpushMakeKeys(seed byte, endpoint string) wpushBrowserKeys {
	uaPriv, p256dh, _ := wpushP256(seed)
	_, vapidPub, vapidPriv := wpushP256(seed + 10)
	auth := bytes.Repeat([]byte{seed + 20}, 16)
	return wpushBrowserKeys{
		uaPriv: uaPriv,
		auth:   auth,
		sub: proto.PushSubscription{
			Endpoint: endpoint,
			P256DH:   p256dh,
			Auth:     base64.RawURLEncoding.EncodeToString(auth),
		},
		vapid: proto.VAPIDKeys{Public: vapidPub, Private: vapidPriv},
	}
}

func wpushDecrypt(t *testing.T, k wpushBrowserKeys, wire []byte) []byte {
	t.Helper()
	if len(wire) < 21 {
		t.Fatalf("push body too short: %d", len(wire))
	}
	rs := binary.BigEndian.Uint32(wire[16:20])
	idLen := int(wire[20])
	if rs != webpush.MaxRecordSize || len(wire) < 21+idLen+16 {
		t.Fatalf("unexpected header rs=%d idLen=%d total=%d", rs, idLen, len(wire))
	}
	return wpushOpenGCM(t, k, wire[:16], wire[21:21+idLen], wire[21+idLen:])
}

func wpushAEAD(t *testing.T, k wpushBrowserKeys, salt, asPubBytes []byte) (cipher.AEAD, []byte) {
	t.Helper()
	asPub, err := ecdh.P256().NewPublicKey(asPubBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := k.uaPriv.ECDH(asPub)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), k.uaPriv.PublicKey().Bytes()...), asPubBytes...)
	ikm, err1 := hkdf.Key(sha256.New, shared, k.auth, string(info), 32)
	cek, err2 := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, err3 := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, err4 := aes.NewCipher(cek)
	gcm, err5 := cipher.NewGCM(block)
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		t.Fatal(err)
	}
	return gcm, nonce
}

func wpushOpenGCM(t *testing.T, k wpushBrowserKeys, salt, asPubBytes, ciphertext []byte) []byte {
	t.Helper()
	gcm, nonce := wpushAEAD(t, k, salt, asPubBytes)
	padded, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatal(err)
	}
	trimmed := bytes.TrimRight(padded, "\x00")
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != 0x02 {
		t.Fatalf("missing RFC 8188 delimiter: %x", padded)
	}
	return trimmed[:len(trimmed)-1]
}

type wpushCaptured struct {
	path     string
	encoding string
	authHdr  string
	ttl      string
	wire     []byte
}

func wpushStatusFor(path string) int {
	switch path {
	case "/404":
		return http.StatusNotFound
	case "/410":
		return http.StatusGone
	default:
		return http.StatusCreated
	}
}

func wpushServer(t *testing.T) (*httptest.Server, <-chan wpushCaptured) {
	t.Helper()
	got := make(chan wpushCaptured, 32) // single reader: wpushRecv in tests
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
			return
		}
		rw.WriteHeader(wpushStatusFor(req.URL.Path))
		got <- wpushCaptured{
			path:     req.URL.Path,
			encoding: req.Header.Get("Content-Encoding"),
			authHdr:  req.Header.Get("Authorization"),
			ttl:      req.Header.Get("TTL"),
			wire:     body,
		}
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func wpushRecv(t *testing.T, ch <-chan wpushCaptured, n int) []wpushCaptured {
	t.Helper()
	out := make([]wpushCaptured, n)
	for i := range n {
		out[i] = <-ch
	}
	return out
}

type wpushLogEntry struct {
	dev  identity.Key
	kind string
}

func wpushLog(t *testing.T, first identity.Key, firstKind string, rest ...wpushLogEntry) *identity.Log {
	t.Helper()
	l := &identity.Log{User: first.ID()}
	for i, item := range append([]wpushLogEntry{{dev: first, kind: firstKind}}, rest...) {
		p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":%q,"name":"dev"}`, first.ID(), i+1, item.dev.ID(), item.kind)
		if _, err := l.Apply(first.Sign(p)); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func wpushState(t *testing.T, homeDev identity.Key) *home.State {
	t.Helper()
	s := home.NewState(groupID, homeDev.ID(), wpushBrowserOne.ID())
	s.Logs[wpushBrowserOne.ID()] = wpushLog(t, wpushBrowserOne, "browser",
		wpushLogEntry{wpushBrowserTwo, "browser"},
		wpushLogEntry{homeDev, "node"},
		wpushLogEntry{wpushHostNode, "node"},
	)
	s.Logs[wpushGuestDev.ID()] = wpushLog(t, wpushGuestDev, "browser")
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: groupID}, Name: "Website <Alpha> & Co", Home: homeDev.ID(), Secretary: aiID}
	own := person(ownerID, wpushBrowserOne, "owner")
	own.Name = "Owner"
	s.People[ownerID] = own
	sec := ai(aiID, wpushHostNode)
	sec.User, sec.Name, sec.Secretary = wpushBrowserOne.ID(), "Dawn", true
	s.AIs[aiID] = sec
	wrk := ai(wpushAIMem, wpushHostNode)
	wrk.User, wrk.Name = wpushBrowserOne.ID(), "Codex"
	s.AIs[wpushAIMem] = wrk
	s.Tasks[wpushTaskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: wpushTaskID}, Group: groupID, Num: 1,
		Title: "Ship encrypted Web Push reminders\nDetails ignored", Slug: "ship-encrypted-web-push-reminders",
		State: "merged", Step: "merge", Assignee: wpushAIMem,
	}
	s.Seq = 10
	return s
}

func wpushSignOn(by identity.Key, n byte, grp string, k wpushBrowserKeys, events []string) proto.Envelope {
	return briefSign(by, proto.PushOn{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(grp)},
		Sub:         k.sub,
		VAPID:       k.vapid,
		On:          events,
	})
}

func wpushSubBrowser(t *testing.T, s *home.State, by identity.Key, n byte, k wpushBrowserKeys, events []string) {
	t.Helper()
	res := briefApply(t, s, disk{}, receipts{}, by, wpushSignOn(by, n, groupID, k, events))
	equal(t, res.Answer, store.Outcome{OK: true})
	home.After["push"](s, res)
}

func TestWebPushPrincipalAndCommandForms(t *testing.T) {
	s := wpushState(t, device(90))
	k := wpushMakeKeys(1, "https://push.example/send/one")

	briefRefused(t, briefApply(t, s, disk{}, receipts{}, wpushHostNode, wpushSignOn(wpushHostNode, 1, groupID, k, []string{"approval"})), "forbidden")

	browserAI := id(80)
	s.AIs[browserAI] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: browserAI}, User: wpushBrowserOne.ID()},
		Host:      wpushBrowserOne.ID(),
	}
	asEnv := wpushBrowserOne.Sign(fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"as":%q,"off":true}`, id(2), clock, groupID, browserAI))
	briefRefused(t, briefApply(t, s, disk{}, receipts{}, wpushBrowserOne, asEnv), "forbidden")

	offExtra := wpushBrowserOne.Sign(fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"off":true,"sub":123,"on":"bad"}`, id(3), clock, groupID))
	resOff := briefApply(t, s, disk{}, receipts{}, wpushBrowserOne, offExtra)
	equal(t, resOff.Answer, store.Outcome{OK: true})
	equal(t, len(resOff.Writes), 1)

	var onMap map[string]any
	json.Unmarshal(briefRaw(t, proto.PushOn{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(id(4)), Timestamp: clock, Group: proto.ID(groupID)},
		Sub:         k.sub, VAPID: k.vapid, On: []string{},
	}), &onMap)
	onMap["off"] = false
	resOnFalse := briefApply(t, s, disk{}, receipts{}, wpushBrowserOne, briefSign(wpushBrowserOne, onMap))
	equal(t, resOnFalse.Answer, store.Outcome{OK: true})
	equal(t, len(resOnFalse.Writes), 1)

	badOff := wpushBrowserOne.Sign(fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"off":false}`, id(5), clock, groupID))
	briefRefused(t, briefApply(t, s, disk{}, receipts{}, wpushBrowserOne, badOff), "bad_request")
}

func TestWebPushReminderKindsTitlesAndNoLeak(t *testing.T) {
	srv, got := wpushServer(t)
	s := wpushState(t, device(91))
	k := wpushMakeKeys(4, srv.URL+"/kinds")
	wpushSubBrowser(t, s, wpushBrowserOne, 1, k, []string{"mention", "approval", "decision", "brief", "done"})

	wpushVerifyIgnoredRows(t, s, got)
	secretCmd := "curl -H 'Authorization: Bearer topsecret' https://internal.example"
	secretText := "Private approval explanation text"
	rows, want := wpushReminderFixtures(t, s, secretCmd, secretText)
	home.After["push"](s, home.Result{Rows: rows})

	for _, call := range wpushRecv(t, got, len(want)) {
		wpushCheckReminder(t, k, call, secretCmd, secretText, want)
	}
	equal(t, len(want), 0)
}

func wpushVerifyIgnoredRows(t *testing.T, s *home.State, got <-chan wpushCaptured) {
	t.Helper()
	updated := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(10), Seq: 15}, Pos: 11, Group: groupID, By: wpushAIMem, Kind: "approval",
		Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: "ls", Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
		State: &proto.CardState{Status: "pending"},
	}
	aiBrief := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(11), Seq: 16}, Pos: 16, Group: groupID, By: aiID, Kind: "brief",
		Body:  briefRaw(t, proto.BriefBody{Title: "AI brief", Criteria: []string{"c"}, Tasks: []proto.BriefTask{}, Requester: wpushAIMem}),
		State: &proto.CardState{Status: "pending"},
	}
	aiMention := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(12), Seq: 17}, Pos: 17, Group: groupID, By: ownerID,
		Kind: "text", Text: "Hello AI", Mentions: []string{wpushAIMem}, Body: json.RawMessage(`{}`),
	}
	home.After["push"](s, home.Result{
		Writes: []home.Write{setting{"k", "v"}},
		Rows:   []home.Row{home.RowOf(s.Tasks[wpushTaskID]), home.RowOf(updated), home.RowOf(aiBrief), home.RowOf(aiMention)},
	})
	equal(t, len(got), 0)
}

func wpushReminderFixtures(t *testing.T, s *home.State, secretCmd, secretText string) ([]home.Row, map[proto.ID]proto.Reminder) {
	t.Helper()
	longWhy := strings.Repeat("界", 81) + "\r\nsecond line"
	emptyWhy := "\nsecond line"
	exact80 := strings.Repeat("é", 80) + "\nextra line"
	rows := []home.Row{
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(20), Seq: 20}, Pos: 20, Group: groupID, By: wpushAIMem,
			Kind: "approval", Text: secretText, Mentions: []string{ownerID},
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: secretCmd, Why: &longWhy, Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(21), Seq: 21}, Pos: 21, Group: groupID, By: wpushAIMem,
			Kind: "approval", Text: secretText,
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Write", Command: secretCmd, Category: "file", Approvers: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(22), Seq: 22}, Pos: 22, Group: groupID, By: wpushAIMem,
			Kind: "decision", Text: secretText,
			Body:  briefRaw(t, proto.DecisionBody{Question: exact80, Options: []proto.DecisionOption{}, To: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(23), Seq: 23}, Pos: 23, Group: groupID, By: aiID,
			Kind: "brief", Text: secretText,
			Body:  briefRaw(t, proto.BriefBody{Title: "Plan <v1> & rollout\rMore", Criteria: []string{"c"}, Tasks: []proto.BriefTask{}, Requester: ownerID}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(24), Seq: 24}, Pos: 24, Group: groupID, By: aiID,
			Kind: "summary", Text: secretText,
			Body: briefRaw(t, proto.SummaryBody{Task: wpushTaskID, Score: 95, Rounds: 1, Minutes: 5}),
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(25), Seq: 25}, Pos: 25, Group: groupID, By: ownerID,
			Kind: "text", Text: secretText, Mentions: []string{ownerID}, Body: json.RawMessage(`{}`),
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(26), Seq: 26}, Pos: 26, Group: groupID, By: wpushAIMem,
			Kind: "approval", Text: secretText,
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: secretCmd, Why: &emptyWhy, Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
	}
	want := map[proto.ID]proto.Reminder{
		proto.ID(id(20)): {Group: proto.ID(groupID), Msg: proto.ID(id(20)), GroupName: s.Group.Name, Kind: "approval", Who: "Codex", Title: strings.Repeat("界", 80)},
		proto.ID(id(21)): {Group: proto.ID(groupID), Msg: proto.ID(id(21)), GroupName: s.Group.Name, Kind: "approval", Who: "Codex"},
		proto.ID(id(22)): {Group: proto.ID(groupID), Msg: proto.ID(id(22)), GroupName: s.Group.Name, Kind: "decision", Who: "Codex", Title: strings.Repeat("é", 80)},
		proto.ID(id(23)): {Group: proto.ID(groupID), Msg: proto.ID(id(23)), GroupName: s.Group.Name, Kind: "brief", Who: "Dawn", Title: "Plan <v1> & rollout"},
		proto.ID(id(24)): {Group: proto.ID(groupID), Msg: proto.ID(id(24)), GroupName: s.Group.Name, Kind: "done", Who: "Dawn", Title: "Ship encrypted Web Push reminders"},
		proto.ID(id(25)): {Group: proto.ID(groupID), Msg: proto.ID(id(25)), GroupName: s.Group.Name, Kind: "mention", Who: "Owner"},
		proto.ID(id(26)): {Group: proto.ID(groupID), Msg: proto.ID(id(26)), GroupName: s.Group.Name, Kind: "approval", Who: "Codex"},
	}
	return rows, want
}

func wpushCheckReminder(t *testing.T, k wpushBrowserKeys, call wpushCaptured, secretCmd, secretText string, want map[proto.ID]proto.Reminder) {
	t.Helper()
	equal(t, call.encoding, "aes128gcm")
	equal(t, call.ttl, "86400")
	if !strings.Contains(call.authHdr, "k="+k.vapid.Public) {
		t.Fatalf("bad auth header: %s", call.authHdr)
	}
	wpushNoSubstrings(t, call.wire, secretCmd, secretText, "Codex")
	plain := wpushDecrypt(t, k, call.wire)
	wpushCheckPlain(t, plain, secretCmd, secretText, want)
}

func wpushNoSubstrings(t *testing.T, data []byte, forbidden ...string) {
	t.Helper()
	for _, s := range forbidden {
		if bytes.Contains(data, []byte(s)) {
			t.Fatalf("payload leaked %q", s)
		}
	}
}

func wpushCheckPlain(t *testing.T, plain []byte, secretCmd, secretText string, want map[proto.ID]proto.Reminder) {
	t.Helper()
	wpushNoSubstrings(t, plain, `\u003c`, secretCmd, secretText)
	equal(t, proto.UnexpectedNulls(plain), []string{})
	var got proto.Reminder
	if err := proto.Decode(plain, &got); err != nil {
		t.Fatalf("decode %s: %v", plain, err)
	}
	exp, ok := want[got.Msg]
	if !ok || got != exp {
		t.Fatalf("got %+v; want %+v", got, exp)
	}
	if exp.Title == "" {
		wpushNoSubstrings(t, plain, `"title"`)
	}
	delete(want, got.Msg)
}

func TestWebPushEligibilityPerBrowserMemberAndRevocation(t *testing.T) {
	srv, got := wpushServer(t)
	s := wpushState(t, device(92))
	gst := person(wpushGuestMem, wpushGuestDev, "member")
	gst.Name = "Guest"
	s.People[wpushGuestMem] = gst

	kOne := wpushMakeKeys(11, srv.URL+"/b1")
	kTwo := wpushMakeKeys(12, srv.URL+"/b2")
	kGuest := wpushMakeKeys(13, srv.URL+"/guest")
	wpushSubBrowser(t, s, wpushBrowserOne, 1, kOne, []string{"approval"})
	wpushSubBrowser(t, s, wpushBrowserTwo, 1, kTwo, []string{"decision", "approval"})
	wpushSubBrowser(t, s, wpushGuestDev, 1, kGuest, []string{"approval"})

	s.Online[wpushBrowserOne.ID()] = true
	s.Online[wpushHostNode.ID()] = true
	approvalFor := func(n byte, user string) home.Result {
		return home.Result{Rows: []home.Row{home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(n), Seq: int64(n)}, Pos: int64(n),
			Group: groupID, By: wpushAIMem, Kind: "approval", Mentions: []string{ownerID, wpushGuestMem},
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: "npm i", Category: "command", Approvers: []string{user}}),
			State: &proto.CardState{Status: "pending"},
		})}}
	}
	home.After["push"](s, approvalFor(30, wpushBrowserOne.ID()))
	equal(t, wpushRecv(t, got, 1)[0].path, "/b2")

	delete(s.Online, wpushBrowserOne.ID())
	home.After["push"](s, home.Result{Rows: []home.Row{home.RowOf(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(31), Seq: 31}, Pos: 31, Group: groupID, By: wpushAIMem, Kind: "decision",
		Body:  briefRaw(t, proto.DecisionBody{Question: "Port?", Options: []proto.DecisionOption{}, To: []string{wpushBrowserOne.ID()}}),
		State: &proto.CardState{Status: "pending"},
	})}})
	equal(t, wpushRecv(t, got, 1)[0].path, "/b2")

	delete(s.People, wpushGuestMem)
	home.After["push"](s, approvalFor(32, wpushGuestDev.ID()))

	revoke := fmt.Sprintf(`{"t":"device","user":%q,"ver":5,"op":"revoke","dev":%q}`, wpushBrowserOne.ID(), wpushBrowserTwo.ID())
	s.Commit(home.Result{Rows: []home.Row{home.RowOf(proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: wpushBrowserOne.ID() + ":5", Seq: 33},
		DeviceChange: proto.DeviceChange{User: wpushBrowserOne.ID(), Version: 5, Op: "revoke", Device: wpushBrowserTwo.ID()},
		Env:          wpushBrowserOne.Sign(revoke),
	})}})
	home.After["push"](s, approvalFor(34, wpushBrowserOne.ID()))
	equal(t, wpushRecv(t, got, 1)[0].path, "/b1")
	equal(t, len(got), 0)
}

func TestWebPushLoopStartupAcrossGroupsEvictionAndRollback(t *testing.T) {
	srv, got := wpushServer(t)
	db := runOpen(t)
	start := func() *runWorld {
		return runStart(t, db, home.NodeTables(), func(cfg *home.LoopConfig) {
			cfg.Home = wpushHomeNode.ID()
			cfg.Owner = wpushBrowserOne.ID()
			cfg.Logs = func(user string) *identity.Log {
				if user == wpushBrowserOne.ID() {
					return wpushLog(t, wpushBrowserOne, "browser",
						wpushLogEntry{wpushBrowserTwo, "browser"},
						wpushLogEntry{wpushHomeNode, "node"},
						wpushLogEntry{wpushHostNode, "node"},
					)
				}
				return nil
			}
		})
	}

	w := start()
	wpushFoundTwoGroups(t, w)
	k1 := wpushMakeKeys(31, srv.URL+"/b1")
	k2 := wpushMakeKeys(32, srv.URL+"/410")
	b1 := w.attach(wpushBrowserOne)
	b2 := w.attach(wpushBrowserTwo)
	w.loop.Cmd(b1.conn, 1, wpushSignOn(wpushBrowserOne, 90, groupID, k1, []string{"mention"}))
	b1.gets(`{"re":1,"ok":true}`)
	w.loop.Cmd(b2.conn, 1, wpushSignOn(wpushBrowserTwo, 91, groupID, k2, []string{"mention"}))
	b2.gets(`{"re":1,"ok":true}`)
	firstLoop := w.loop
	firstLoop.Close()

	// Restart: StartLoop loads push_sub automatically; 410 evicts b2 in SQLite too.
	w = start()
	firstLoop.Close() // closing an earlier loop does not clear the active loop
	wpushPostGroupTwoMention(t, w, 92, "First mention")
	wpushRecv(t, got, 2)
	w.loop.Close()

	// After second restart, b2 stays evicted while b1 still receives reminders, until b1 sends off: true.
	w = start()
	wpushPostGroupTwoMention(t, w, 93, "Second mention")
	calls := wpushRecv(t, got, 1)
	equal(t, calls[0].path, "/b1")
	var rem proto.Reminder
	if err := proto.Decode(wpushDecrypt(t, k1, calls[0].wire), &rem); err != nil {
		t.Fatal(err)
	}
	equal(t, rem, proto.Reminder{Group: proto.ID(wpushGroupTwo), Msg: proto.ID(id(93)), GroupName: "Two", Kind: "mention", Who: "Codex"})

	wpushVerifyOffAndErrors(t, w, db, srv.URL, got)
}

func wpushAttachRaw(w *runWorld, key identity.Key) *runPeer {
	c := runNewClient()
	return w.attachClient(key, c, c)
}

func wpushFoundTwoGroups(t *testing.T, w *runWorld) {
	t.Helper()
	b1 := wpushAttachRaw(w, wpushBrowserOne)
	host := wpushAttachRaw(w, wpushHostNode)
	for i, grp := range []struct{ id, name string }{{groupID, "One"}, {wpushGroupTwo, "Two"}} {
		create := proto.GroupCreate{
			GroupCommand: proto.GroupCommand{
				HomeCommand: proto.HomeCommand{Type: "group", Nonce: proto.ID(grp.id), Timestamp: clock, Group: proto.ID(grp.id)},
				Op:          "create",
			},
			Name: grp.name, Kind: "project", Repo: "r", Base: "main", Forge: "none", Checks: []string{}, MemberName: "Owner",
		}
		w.loop.Cmd(b1.conn, int64(i+1), briefSign(wpushBrowserOne, create))
		b1.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, i+1))
		addAI := proto.MemberAddAI{
			MemberCommand: proto.MemberCommand{
				HomeCommand: proto.HomeCommand{Type: "member", Nonce: proto.ID(id(byte(81 + i))), Timestamp: clock, Group: proto.ID(grp.id)},
				Op:          "add_ai", ID: proto.ID(wpushAIMem),
			},
			CLI: "codex", Name: "Codex", Account: "api_key", Approvers: []string{},
		}
		w.loop.Cmd(host.conn, int64(i+1), briefSign(wpushHostNode, addAI))
		host.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, i+1))
	}
	w.loop.Detach(b1.conn)
	w.loop.Detach(host.conn)
}

func wpushOwnerMemberOf(grp string) proto.ID {
	h := sha256.Sum256([]byte("SoloDawn group owner " + grp))
	return proto.ID(base64.RawURLEncoding.EncodeToString(h[:16]))
}

func wpushPostGroupTwoMention(t *testing.T, w *runWorld, nonce byte, text string) {
	t.Helper()
	host := w.attach(wpushHostNode)
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(nonce)), Timestamp: clock, Group: proto.ID(wpushGroupTwo), As: proto.ID(wpushAIMem)},
		Kind:        "text", Text: text, Mentions: []proto.ID{wpushOwnerMemberOf(wpushGroupTwo)}, Body: proto.EmptyBody{},
	}
	w.loop.Cmd(host.conn, 1, briefSign(wpushHostNode, post))
	host.gets(`{"re":1,"ok":true}`)
	w.loop.Detach(host.conn)
}

func wpushVerifyOffAndErrors(t *testing.T, w *runWorld, db *store.Store, baseURL string, got <-chan wpushCaptured) {
	t.Helper()
	b1 := wpushAttachRaw(w, wpushBrowserOne)
	offEnv := briefSign(wpushBrowserOne, proto.PushOff{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(id(94)), Timestamp: clock, Group: proto.ID(groupID)},
		Off:         true,
	})
	w.loop.Cmd(b1.conn, 1, offEnv)
	b1.gets(`{"re":1,"ok":true}`)
	w.loop.Detach(b1.conn)
	<-b1.conn.Gone()
	wpushPostGroupTwoMention(t, w, 95, "After off")
	equal(t, len(got), 0)

	// Re-subscribe b1 to /404, then drop push_sub: a 404 reports the failed eviction write,
	// a PushOn rolls back without polluting memory, and StartLoop refuses to start.
	kErr := wpushMakeKeys(33, baseURL+"/404")
	b1 = wpushAttachRaw(w, wpushBrowserOne)
	w.loop.Cmd(b1.conn, 1, wpushSignOn(wpushBrowserOne, 96, groupID, kErr, []string{"mention"}))
	b1.gets(`{"re":1,"ok":true}`)
	w.loop.Detach(b1.conn)
	<-b1.conn.Gone()

	runExec(t, db, "DROP TABLE push_sub")
	wpushPostGroupTwoMention(t, w, 97, "Trigger failed store eviction")
	wpushRecv(t, got, 1)
	if !strings.Contains(w.reported(), "push_sub") {
		t.Fatal("expected reported push_sub write error")
	}

	b2 := wpushAttachRaw(w, wpushBrowserTwo)
	kRollback := wpushMakeKeys(34, baseURL+"/rollback")
	w.loop.Cmd(b2.conn, 1, wpushSignOn(wpushBrowserTwo, 98, groupID, kRollback, []string{"mention"}))
	if ans := b2.next(); !strings.Contains(ans, `"code":"internal"`) {
		t.Fatalf("expected internal error on failed Put, got %s", ans)
	}
	w.reported()
	w.loop.Detach(b2.conn)
	<-b2.conn.Gone()
	wpushPostGroupTwoMention(t, w, 99, "Rolled-back b2 must not receive push")
	equal(t, len(got), 0)
	w.loop.Close()

	// After Close(), h.loop is nil: an After["push"] 404 without a loop must not touch the closed db or report.
	if _, err := home.StartLoop(home.LoopConfig{Store: db, Home: wpushHomeNode.ID(), Owner: wpushBrowserOne.ID(), Logs: func(string) *identity.Log { return nil }, Tables: home.NodeTables()}); err == nil {
		t.Fatal("expected StartLoop error when push_sub is missing")
	}
}
