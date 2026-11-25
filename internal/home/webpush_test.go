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
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	wpushSecMem     = id(75)
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
	case "/500":
		return http.StatusInternalServerError
	default:
		return http.StatusCreated
	}
}

func wpushServer(t *testing.T, onPath func(string)) (*httptest.Server, <-chan wpushCaptured) {
	t.Helper()
	got := make(chan wpushCaptured, 32) // single reader: wpushRecv in tests
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if onPath != nil {
			onPath(req.URL.Path)
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

func wpushSignOn(by identity.Key, n byte, grp string, k wpushBrowserKeys, events []string) proto.Envelope {
	return briefSign(by, proto.PushOn{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(grp)},
		Sub:         k.sub,
		VAPID:       k.vapid,
		On:          events,
	})
}

func wpushAttachRaw(w *runWorld, key identity.Key) *runPeer {
	c := runNewClient()
	return w.attachClient(key, c, c)
}

func wpushCmdOK(t *testing.T, w *runWorld, signer identity.Key, env proto.Envelope) {
	t.Helper()
	p := wpushAttachRaw(w, signer)
	w.loop.Cmd(p.conn, 1, env)
	p.gets(`{"re":1,"ok":true}`)
	w.loop.Detach(p.conn)
	<-p.conn.Gone()
}

func wpushOwnerMemberOf(grp string) proto.ID {
	h := sha256.Sum256([]byte("SoloDawn group owner " + grp))
	return proto.ID(base64.RawURLEncoding.EncodeToString(h[:16]))
}

func wpushStartWorld(t *testing.T, db *store.Store, homeDev identity.Key, emit func() []home.Row) *runWorld {
	t.Helper()
	tables := home.NodeTables()
	tables.Commands = maps.Clone(tables.Commands)
	if emit != nil {
		tables.Commands["emit"] = func(*home.State, home.Command) (home.Result, *home.Refusal) {
			return home.Result{Rows: emit()}, nil
		}
	}
	return runStart(t, db, tables, func(cfg *home.LoopConfig) {
		cfg.Home = homeDev.ID()
		cfg.Owner = wpushBrowserOne.ID()
		cfg.Logs = func(user string) *identity.Log {
			switch user {
			case wpushBrowserOne.ID():
				return wpushLog(t, wpushBrowserOne, "browser",
					wpushLogEntry{wpushBrowserTwo, "browser"},
					wpushLogEntry{homeDev, "node"},
					wpushLogEntry{wpushHostNode, "node"},
				)
			case wpushGuestDev.ID():
				return wpushLog(t, wpushGuestDev, "browser")
			}
			return nil
		}
	})
}

func wpushFoundGroup(t *testing.T, w *runWorld, grpID, name string, nBase byte) {
	t.Helper()
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: proto.ID(grpID), Timestamp: clock, Group: proto.ID(grpID)},
			Op:          "create",
		},
		Name: name, Kind: "project", Repo: "r", Base: "main", Forge: "none", Checks: []string{}, MemberName: "Owner",
	}
	wpushCmdOK(t, w, wpushBrowserOne, briefSign(wpushBrowserOne, create))
	addSec := proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: proto.ID(id(nBase)), Timestamp: clock, Group: proto.ID(grpID)},
			Op:          "add_ai", ID: proto.ID(wpushSecMem),
		},
		CLI: "claude", Name: "Dawn", Account: "api_key", Secretary: true, Approvers: []string{},
	}
	wpushCmdOK(t, w, wpushHostNode, briefSign(wpushHostNode, addSec))
	addAI := proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: proto.ID(id(nBase + 1)), Timestamp: clock, Group: proto.ID(grpID)},
			Op:          "add_ai", ID: proto.ID(wpushAIMem),
		},
		CLI: "codex", Name: "Codex", Account: "api_key", Approvers: []string{},
	}
	wpushCmdOK(t, w, wpushHostNode, briefSign(wpushHostNode, addAI))
}

func TestWebPushCommandForms(t *testing.T) {
	s := group(t)
	k := wpushMakeKeys(1, "https://push.example/send/one")

	offExtra := browser.Sign(fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"off":true,"sub":123,"on":"bad"}`, id(1), clock, groupID))
	resOff := briefApply(t, s, disk{}, receipts{}, browser, offExtra)
	equal(t, resOff.Answer, store.Outcome{OK: true})
	equal(t, len(resOff.Writes), 1)

	var onMap map[string]any
	json.Unmarshal(briefRaw(t, proto.PushOn{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(id(2)), Timestamp: clock, Group: proto.ID(groupID)},
		Sub:         k.sub, VAPID: k.vapid, On: []string{},
	}), &onMap)
	onMap["off"] = false
	resOnFalse := briefApply(t, s, disk{}, receipts{}, browser, briefSign(browser, onMap))
	equal(t, resOnFalse.Answer, store.Outcome{OK: true})
	equal(t, len(resOnFalse.Writes), 1)

	badOff := browser.Sign(fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"off":false}`, id(3), clock, groupID))
	briefRefused(t, briefApply(t, s, disk{}, receipts{}, browser, badOff), "bad_request")

	home.After["push"](s, home.Result{Writes: []home.Write{setting{"k", "v"}}})
}

func TestWebPushReminderKindsTitlesAndNoLeak(t *testing.T) {
	srv, got := wpushServer(t, nil)
	var nextRows []home.Row
	homeDev := device(91)
	w := wpushStartWorld(t, runOpen(t), homeDev, func() []home.Row { return nextRows })
	wpushFoundGroup(t, w, groupID, "Website <Alpha> & Co", 10)

	k := wpushMakeKeys(4, srv.URL+"/kinds")
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 1, groupID, k, []string{"mention", "approval", "decision", "brief", "done", "confirm"}))

	secretCmd := "curl -H 'Authorization: Bearer topsecret' https://internal.example"
	secretText := "Private approval explanation text"
	rows, want := wpushReminderFixtures(t, string(wpushOwnerMemberOf(groupID)), secretCmd, secretText)
	nextRows = rows
	emitEnv := wpushBrowserTwo.Sign(fmt.Sprintf(`{"t":"emit","n":%q,"ts":%d,"g":%q}`, id(2), clock, groupID))
	wpushCmdOK(t, w, wpushBrowserTwo, emitEnv)

	for _, call := range wpushRecv(t, got, len(want)) {
		wpushCheckReminder(t, k, call, secretCmd, secretText, want)
	}
	equal(t, len(want), 0)
}

func wpushReminderFixtures(t *testing.T, ownerMem, secretCmd, secretText string) ([]home.Row, map[proto.ID]proto.Reminder) {
	t.Helper()
	longWhy := strings.Repeat("界", 81) + "\r\nsecond line"
	emptyWhy := "\nsecond line"
	exact80 := strings.Repeat("é", 80) + "\nextra line"
	grpName := "Website <Alpha> & Co"
	taskRow := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: wpushTaskID}, Group: groupID, Num: 1,
		Title: "Ship encrypted Web Push reminders\nDetails ignored", Slug: "ship-encrypted-web-push-reminders",
		State: "merged", Step: "merge", Assignee: wpushAIMem,
	}
	updatedCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(15)}, Pos: 1, Group: groupID, By: wpushAIMem, Kind: "approval",
		Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: "ls", Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
		State: &proto.CardState{Status: "approved"},
	}
	aiBrief := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(16)}, Group: groupID, By: wpushSecMem, Kind: "brief",
		Body:  briefRaw(t, proto.BriefBody{Title: "AI brief", Criteria: []string{"c"}, Tasks: []proto.BriefTask{}, Requester: wpushAIMem}),
		State: &proto.CardState{Status: "pending"},
	}
	aiMention := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id(17)}, Group: groupID, By: ownerMem,
		Kind: "text", Text: "Hello AI", Mentions: []string{wpushAIMem}, Body: json.RawMessage(`{}`),
	}
	rows := []home.Row{
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(27)}, Group: groupID, By: "system",
			Kind: "confirm", Text: secretText,
			Body:  briefRaw(t, proto.ConfirmBody{What: "direct", AI: wpushAIMem, To: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(taskRow),
		home.RowOf(updatedCard),
		home.RowOf(aiBrief),
		home.RowOf(aiMention),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(20)}, Group: groupID, By: wpushAIMem,
			Kind: "approval", Text: secretText, Mentions: []string{ownerMem},
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: secretCmd, Why: &longWhy, Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(21)}, Group: groupID, By: wpushAIMem,
			Kind: "approval", Text: secretText,
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Write", Command: secretCmd, Category: "file", Approvers: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(22)}, Group: groupID, By: wpushAIMem,
			Kind: "decision", Text: secretText,
			Body:  briefRaw(t, proto.DecisionBody{Question: exact80, Options: []proto.DecisionOption{}, To: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(23)}, Group: groupID, By: wpushSecMem,
			Kind: "brief", Text: secretText,
			Body:  briefRaw(t, proto.BriefBody{Title: "Plan <v1> & rollout\rMore", Criteria: []string{"c"}, Tasks: []proto.BriefTask{}, Requester: ownerMem}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(24)}, Group: groupID, By: wpushSecMem,
			Kind: "summary", Text: secretText,
			Body: briefRaw(t, proto.SummaryBody{Task: wpushTaskID, Score: 95, Rounds: 1, Minutes: 5}),
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(25)}, Group: groupID, By: ownerMem,
			Kind: "text", Text: secretText, Mentions: []string{ownerMem}, Body: json.RawMessage(`{}`),
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(26)}, Group: groupID, By: wpushAIMem,
			Kind: "approval", Text: secretText,
			Body:  briefRaw(t, proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMem, Tool: "Bash", Command: secretCmd, Why: &emptyWhy, Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
			State: &proto.CardState{Status: "pending"},
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(28)}, Group: groupID, By: "system",
			Kind: "system", Mentions: []string{ownerMem},
			Body: briefRaw(t, proto.TaskFailedBody{SystemCode: proto.SystemCode{Code: "task_failed"}, Task: wpushTaskID, Reason: "failures"}),
		}),
		home.RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(29)}, Group: groupID, By: "system",
			Kind: "system", Mentions: []string{ownerMem},
			Body: briefRaw(t, proto.LoginInvalidBody{SystemCode: proto.SystemCode{Code: "login_invalid"}, AI: wpushSecMem, Detail: &secretText}),
		}),
	}
	want := map[proto.ID]proto.Reminder{
		proto.ID(id(28)): {Group: proto.ID(groupID), Msg: proto.ID(id(28)), GroupName: grpName, Kind: "mention", Who: "Codex", Title: "Ship encrypted Web Push reminders"},
		proto.ID(id(29)): {Group: proto.ID(groupID), Msg: proto.ID(id(29)), GroupName: grpName, Kind: "mention", Who: "Dawn"},
		proto.ID(id(27)): {Group: proto.ID(groupID), Msg: proto.ID(id(27)), GroupName: grpName, Kind: "confirm", Who: "Codex"},
		proto.ID(id(20)): {Group: proto.ID(groupID), Msg: proto.ID(id(20)), GroupName: grpName, Kind: "approval", Who: "Codex", Title: strings.Repeat("界", 80)},
		proto.ID(id(21)): {Group: proto.ID(groupID), Msg: proto.ID(id(21)), GroupName: grpName, Kind: "approval", Who: "Codex"},
		proto.ID(id(22)): {Group: proto.ID(groupID), Msg: proto.ID(id(22)), GroupName: grpName, Kind: "decision", Who: "Codex", Title: strings.Repeat("é", 80)},
		proto.ID(id(23)): {Group: proto.ID(groupID), Msg: proto.ID(id(23)), GroupName: grpName, Kind: "brief", Who: "Dawn", Title: "Plan <v1> & rollout"},
		proto.ID(id(24)): {Group: proto.ID(groupID), Msg: proto.ID(id(24)), GroupName: grpName, Kind: "done", Who: "Dawn", Title: "Ship encrypted Web Push reminders"},
		proto.ID(id(25)): {Group: proto.ID(groupID), Msg: proto.ID(id(25)), GroupName: grpName, Kind: "mention", Who: "Owner"},
		proto.ID(id(26)): {Group: proto.ID(groupID), Msg: proto.ID(id(26)), GroupName: grpName, Kind: "approval", Who: "Codex"},
	}
	return rows, want
}

func wpushCheckReminder(t *testing.T, k wpushBrowserKeys, call wpushCaptured, secretCmd, secretText string, want map[proto.ID]proto.Reminder) {
	t.Helper()
	equal(t, call.encoding, "aes128gcm")
	equal(t, call.ttl, "86400")
	wpushCheckVAPIDHeader(t, call.authHdr, k.vapid.Public)
	wpushNoSubstrings(t, call.wire, secretCmd, secretText, "Codex")
	plain := wpushDecrypt(t, k, call.wire)
	wpushCheckPlain(t, plain, secretCmd, secretText, want)
}

func wpushCheckVAPIDHeader(t *testing.T, hdr, pub string) {
	t.Helper()
	if !strings.Contains(hdr, "k="+pub) {
		t.Fatalf("missing VAPID public key in %s", hdr)
	}
	parts := strings.Split(strings.TrimPrefix(strings.Split(hdr, ",")[0], "vapid t="), ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected JWT in %s", hdr)
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !bytes.Contains(claims, []byte(`"sub":"https://github.com/huanchong-99/SoloDawn"`)) {
		t.Fatalf("unexpected VAPID JWT claims %s (err %v)", claims, err)
	}
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
	srv, got := wpushServer(t, nil)
	var nextRows []home.Row
	homeDev := device(92)
	w := wpushStartWorld(t, runOpen(t), homeDev, func() []home.Row { return nextRows })
	wpushFoundGroup(t, w, groupID, "Website", 10)

	emit := func(n byte, rows ...home.Row) {
		nextRows = rows
		wpushCmdOK(t, w, wpushHostNode, wpushHostNode.Sign(fmt.Sprintf(`{"t":"emit","n":%q,"ts":%d,"g":%q}`, id(n), clock, groupID)))
	}
	guestAdd := fmt.Sprintf(`{"t":"device","user":%q,"ver":1,"op":"add","dev":%q,"kind":"browser","name":"dev"}`, wpushGuestDev.ID(), wpushGuestDev.ID())
	emit(20,
		home.RowOf(proto.DeviceRow{
			RowHeader:    proto.RowHeader{Type: "device", ID: wpushGuestDev.ID() + ":1"},
			DeviceChange: proto.DeviceChange{User: wpushGuestDev.ID(), Version: 1, Op: "add", Device: wpushGuestDev.ID(), Kind: "browser", Name: "dev"},
			Env:          wpushGuestDev.Sign(guestAdd),
		}),
		home.RowOf(proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: wpushGuestMem}, Group: groupID, Kind: "human", User: wpushGuestDev.ID(), Name: "Guest"},
			Role:      "member",
		}),
	)

	kOne := wpushMakeKeys(11, srv.URL+"/b1")
	kTwo := wpushMakeKeys(12, srv.URL+"/b2")
	kGuest := wpushMakeKeys(13, srv.URL+"/guest")
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 21, groupID, kOne, []string{"approval"}))
	wpushCmdOK(t, w, wpushBrowserTwo, wpushSignOn(wpushBrowserTwo, 22, groupID, kTwo, []string{"decision", "approval"}))
	wpushCmdOK(t, w, wpushGuestDev, wpushSignOn(wpushGuestDev, 23, groupID, kGuest, []string{"approval"}))

	// BrowserOne and hostNode stay connected; browserTwo and guest are offline.
	// Approval names only owner, so guest gets nothing and online browserOne gets nothing; only browserTwo gets it.
	b1Online := wpushAttachRaw(w, wpushBrowserOne)
	hostOnline := wpushAttachRaw(w, wpushHostNode)
	postApproval := func(n byte) {
		p := proto.Post[proto.ApprovalBody]{
			HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(n)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(wpushAIMem)},
			Kind:        "approval", Text: "secret", Mentions: []proto.ID{wpushOwnerMemberOf(groupID), proto.ID(wpushGuestMem)},
			Body: proto.ApprovalBody{AI: wpushAIMem, Tool: "Bash", Command: "npm i", Category: "command", Approvers: []string{wpushBrowserOne.ID()}},
		}
		w.loop.Cmd(hostOnline.conn, int64(n), briefSign(wpushHostNode, p))
		hostOnline.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, n))
	}
	postApproval(30)
	equal(t, wpushRecv(t, got, 1)[0].path, "/b2")

	// BrowserOne goes offline; decision arrives -> browserOne subscribed only to approval, so only browserTwo gets it.
	w.loop.Detach(b1Online.conn)
	<-b1Online.conn.Gone()
	dec := proto.Post[proto.DecisionBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(31)), Timestamp: clock, Group: proto.ID(groupID), As: proto.ID(wpushAIMem)},
		Kind:        "decision", Text: "secret", Mentions: []proto.ID{},
		Body: proto.DecisionBody{Question: "Port?", Options: []proto.DecisionOption{{ID: "a", Label: "A"}}, To: []string{wpushBrowserOne.ID()}},
	}
	w.loop.Cmd(hostOnline.conn, 31, briefSign(wpushHostNode, dec))
	hostOnline.gets(`{"re":31,"ok":true}`)
	equal(t, wpushRecv(t, got, 1)[0].path, "/b2")

	// Revoke browserTwo in owner's device log -> subsequent approval goes only to unrevoked browserOne.
	revoke := fmt.Sprintf(`{"t":"device","user":%q,"ver":5,"op":"revoke","dev":%q}`, wpushBrowserOne.ID(), wpushBrowserTwo.ID())
	emit(32, home.RowOf(proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: wpushBrowserOne.ID() + ":5"},
		DeviceChange: proto.DeviceChange{User: wpushBrowserOne.ID(), Version: 5, Op: "revoke", Device: wpushBrowserTwo.ID()},
		Env:          wpushBrowserOne.Sign(revoke),
	}))
	postApproval(33)
	equal(t, wpushRecv(t, got, 1)[0].path, "/b1")
	w.loop.Detach(hostOnline.conn)
	w.loop.Close()
	equal(t, len(got), 0)
}

func TestWebPushLoopStartupEvictionRollbackAndShutdown(t *testing.T) {
	var onPathMu sync.Mutex
	var on410 func()
	srv, got := wpushServer(t, func(path string) {
		onPathMu.Lock()
		fn := on410
		onPathMu.Unlock()
		if path == "/410" && fn != nil {
			fn()
		}
	})
	db := runOpen(t)
	start := func() *runWorld { return wpushStartWorld(t, db, wpushHomeNode, nil) }

	w := start()
	wpushFoundGroup(t, w, groupID, "One", 10)
	wpushFoundGroup(t, w, wpushGroupTwo, "Two", 14)

	closedSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closedSrv.URL + "/down"
	closedSrv.Close()

	// Network failure and 500 preserve both subscriptions across Close and restart.
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 80, groupID, wpushMakeKeys(21, closedURL), []string{"mention"}))
	wpushCmdOK(t, w, wpushBrowserTwo, wpushSignOn(wpushBrowserTwo, 81, groupID, wpushMakeKeys(22, srv.URL+"/500"), []string{"mention"}))
	wpushPostGroupTwoMention(t, w, 82, "Transient errors")
	equal(t, wpushRecv(t, got, 1)[0].path, "/500")
	firstLoop := w.loop
	firstLoop.Close()

	// Restart loads push_sub automatically; update b1 to /404 and b2 to /410 (concurrently replaced by /fresh).
	w = start()
	firstLoop.Close() // closing an earlier loop leaves the active loop intact
	k404 := wpushMakeKeys(31, srv.URL+"/404")
	k410 := wpushMakeKeys(32, srv.URL+"/410")
	kFresh := wpushMakeKeys(33, srv.URL+"/fresh")
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 83, groupID, k404, []string{"mention"}))
	wpushCmdOK(t, w, wpushBrowserTwo, wpushSignOn(wpushBrowserTwo, 84, groupID, k410, []string{"mention"}))
	onPathMu.Lock()
	on410 = func() {
		wpushCmdOK(t, w, wpushBrowserTwo, wpushSignOn(wpushBrowserTwo, 85, groupID, kFresh, []string{"mention"}))
	}
	onPathMu.Unlock()

	wpushPostGroupTwoMention(t, w, 86, "Evict 404 and replace 410")
	wpushRecv(t, got, 2)
	wpushWaitRowCount(t, db, wpushBrowserOne.ID(), 0)
	onPathMu.Lock()
	on410 = nil
	onPathMu.Unlock()

	// On the same running loop, b1 was evicted in memory while b2's /fresh stays active.
	wpushPostGroupTwoMention(t, w, 87, "In-memory check")
	equal(t, wpushRecv(t, got, 1)[0].path, "/fresh")
	w.loop.Close()

	// Across restart, b1 stays deleted in SQLite while b2's /fresh reloads and delivers.
	w = start()
	wpushPostGroupTwoMention(t, w, 88, "After restart")
	calls := wpushRecv(t, got, 1)
	equal(t, calls[0].path, "/fresh")
	var rem proto.Reminder
	if err := proto.Decode(wpushDecrypt(t, kFresh, calls[0].wire), &rem); err != nil {
		t.Fatal(err)
	}
	equal(t, rem, proto.Reminder{Group: proto.ID(wpushGroupTwo), Msg: proto.ID(id(88)), GroupName: "Two", Kind: "mention", Who: "Codex"})

	wpushVerifyOffErrorsAndCloseDrain(t, w, db, srv.URL, got)
}

func wpushWaitRowCount(t *testing.T, db *store.Store, dev string, want int) {
	t.Helper()
	poll(t, 2*time.Second, fmt.Sprintf("push_sub count %d on %s", want, dev), func() string {
		if n := runQuery[int](t, db, "SELECT count(*) FROM push_sub WHERE device = ?", dev); n != want {
			return fmt.Sprintf("count %d", n)
		}
		return ""
	})
}

func wpushPostGroupTwoMention(t *testing.T, w *runWorld, nonce byte, text string) {
	t.Helper()
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(id(nonce)), Timestamp: clock, Group: proto.ID(wpushGroupTwo), As: proto.ID(wpushAIMem)},
		Kind:        "text", Text: text, Mentions: []proto.ID{wpushOwnerMemberOf(wpushGroupTwo)}, Body: proto.EmptyBody{},
	}
	wpushCmdOK(t, w, wpushHostNode, briefSign(wpushHostNode, post))
}

func wpushVerifyOffErrorsAndCloseDrain(t *testing.T, w *runWorld, db *store.Store, baseURL string, got <-chan wpushCaptured) {
	t.Helper()
	offEnv := briefSign(wpushBrowserTwo, proto.PushOff{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(id(90)), Timestamp: clock, Group: proto.ID(groupID)},
		Off:         true,
	})
	wpushCmdOK(t, w, wpushBrowserTwo, offEnv)
	wpushPostGroupTwoMention(t, w, 91, "After off")
	equal(t, len(got), 0)

	// Subscribe b1 to /404, then drop push_sub: 404 reports the failed store delete,
	// PushOn fails with internal without polluting memory, and StartLoop fails on missing push_sub.
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 92, groupID, wpushMakeKeys(34, baseURL+"/404"), []string{"mention"}))
	runExec(t, db, "DROP TABLE push_sub")
	wpushPostGroupTwoMention(t, w, 93, "Failed store eviction")
	wpushRecv(t, got, 1)
	if !strings.Contains(w.reported(), "push_sub") {
		t.Fatal("expected reported push_sub write error")
	}

	b2 := wpushAttachRaw(w, wpushBrowserTwo)
	w.loop.Cmd(b2.conn, 1, wpushSignOn(wpushBrowserTwo, 94, groupID, wpushMakeKeys(35, baseURL+"/rollback"), []string{"mention"}))
	if ans := b2.next(); !strings.Contains(ans, `"code":"internal"`) {
		t.Fatalf("expected internal error on failed Put, got %s", ans)
	}
	w.reported()
	w.loop.Detach(b2.conn)
	<-b2.conn.Gone()
	wpushPostGroupTwoMention(t, w, 95, "Rolled-back b2 gets nothing")
	equal(t, len(got), 0)
	w.loop.Close()

	if _, err := home.StartLoop(home.LoopConfig{Store: db, Home: wpushHomeNode.ID(), Owner: wpushBrowserOne.ID(), Logs: func(string) *identity.Log { return nil }, Tables: home.NodeTables()}); err == nil {
		t.Fatal("expected StartLoop error when push_sub is missing")
	}

	wpushVerifyCloseWaits(t)
}

func wpushVerifyCloseWaits(t *testing.T) {
	t.Helper()
	entered := make(chan struct{})  // single reader: test goroutine below
	release := make(chan struct{})  // closed once by test goroutine
	closedCh := make(chan struct{}) // single reader: select check below
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		rw.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	w := wpushStartWorld(t, runOpen(t), device(95), nil)
	wpushFoundGroup(t, w, wpushGroupTwo, "Slow", 10)
	wpushCmdOK(t, w, wpushBrowserOne, wpushSignOn(wpushBrowserOne, 96, wpushGroupTwo, wpushMakeKeys(36, srv.URL+"/slow"), []string{"mention"}))
	wpushPostGroupTwoMention(t, w, 97, "Hold push")
	<-entered

	go func() {
		w.loop.Close()
		close(closedCh)
	}()
	select {
	case <-closedCh:
		t.Fatal("Close returned while a Web Push request was still in flight")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-closedCh
}
