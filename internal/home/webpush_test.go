package home

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const wpushTestClock = 1763600000000

var (
	wpushTestNow     = time.UnixMilli(wpushTestClock)
	wpushBrowserOne  = wpushTestDevice(61)
	wpushBrowserTwo  = wpushTestDevice(62)
	wpushHomeNode    = wpushTestDevice(63)
	wpushHostNode    = wpushTestDevice(64)
	wpushGuestDev    = wpushTestDevice(65)
	wpushGroupID     = wpushTestID(71)
	wpushGroupTwoID  = wpushTestID(72)
	wpushOwnerMember = wpushTestID(73)
	wpushGuestMember = wpushTestID(74)
	wpushSecMember   = wpushTestID(75)
	wpushAIMember    = wpushTestID(76)
	wpushTaskID      = wpushTestID(77)
)

func wpushTestDevice(seed byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)))
}

func wpushTestID(seed byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 16))
}

func wpushTestP256(seed byte) (*ecdh.PrivateKey, string, string) {
	scalar := bytes.Repeat([]byte{seed}, 32)
	priv, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		panic(err)
	}
	return priv, base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(priv.Bytes())
}

type wpushTestBrowserKeys struct {
	uaPriv *ecdh.PrivateKey
	auth   []byte
	sub    proto.PushSubscription
	vapid  proto.VAPIDKeys
}

func wpushMakeBrowserKeys(seed byte, endpoint string) wpushTestBrowserKeys {
	uaPriv, p256dh, _ := wpushTestP256(seed)
	_, vapidPub, vapidPriv := wpushTestP256(seed + 10)
	auth := bytes.Repeat([]byte{seed + 20}, wpushAuthLen)
	return wpushTestBrowserKeys{
		uaPriv: uaPriv,
		auth:   auth,
		sub: proto.PushSubscription{
			Endpoint: endpoint,
			P256DH:   p256dh,
			Auth:     base64.RawURLEncoding.EncodeToString(auth),
		},
		vapid: proto.VAPIDKeys{
			Public:  vapidPub,
			Private: vapidPriv,
		},
	}
}

func wpushDecryptRFC8291(t *testing.T, k wpushTestBrowserKeys, wire []byte) []byte {
	t.Helper()
	if len(wire) < 21 {
		t.Fatalf("push body too short: %d bytes", len(wire))
	}
	salt := wire[:16]
	rs := binary.BigEndian.Uint32(wire[16:20])
	idLen := int(wire[20])
	if rs != webpush.MaxRecordSize || len(wire) < 21+idLen+16 {
		t.Fatalf("unexpected header rs=%d idLen=%d total=%d", rs, idLen, len(wire))
	}
	asPubBytes := wire[21 : 21+idLen]
	ciphertext := wire[21+idLen:]
	asPub, err := ecdh.P256().NewPublicKey(asPubBytes)
	if err != nil {
		t.Fatalf("ephemeral public key: %v", err)
	}
	shared, err := k.uaPriv.ECDH(asPub)
	if err != nil {
		t.Fatalf("ecdh: %v", err)
	}
	info := append(append([]byte("WebPush: info\x00"), k.uaPriv.PublicKey().Bytes()...), asPubBytes...)
	ikm, err := hkdf.Key(sha256.New, shared, k.auth, string(info), 32)
	if err != nil {
		t.Fatalf("ikm hkdf: %v", err)
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		t.Fatalf("cek hkdf: %v", err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		t.Fatalf("nonce hkdf: %v", err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatalf("aes: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	padded, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("gcm open: %v", err)
	}
	trimmed := bytes.TrimRight(padded, "\x00")
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != 0x02 {
		t.Fatalf("missing RFC 8188 record delimiter: %x", padded)
	}
	return trimmed[:len(trimmed)-1]
}

type wpushCaptured struct {
	endpoint string
	encoding string
	authHdr  string
	ttl      string
	wire     []byte
}

type wpushRecorder struct {
	mu       sync.Mutex
	calls    []wpushCaptured
	status   map[string]int
	errFor   map[string]error
	onCall   func(req *http.Request)
	inFlight int
	maxSeen  int
}

func wpushInstallRecorder(t *testing.T) *wpushRecorder {
	t.Helper()
	wpushBusy.Wait()
	prevClient := wpushClient
	prevTimeout := wpushTimeout
	wpushMu.Lock()
	wpushSubs = map[string]map[string]wpushSub{}
	wpushQueue = nil
	wpushRunning = 0
	wpushMu.Unlock()

	rec := &wpushRecorder{
		status: map[string]int{},
		errFor: map[string]error{},
	}
	wpushClient = rec
	t.Cleanup(func() {
		wpushBusy.Wait()
		wpushClient = prevClient
		wpushTimeout = prevTimeout
		wpushMu.Lock()
		wpushSubs = map[string]map[string]wpushSub{}
		wpushQueue = nil
		wpushRunning = 0
		wpushMu.Unlock()
	})
	return rec
}

func (r *wpushRecorder) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()

	r.mu.Lock()
	r.inFlight++
	if r.inFlight > r.maxSeen {
		r.maxSeen = r.inFlight
	}
	hook := r.onCall
	fail := r.errFor[req.URL.String()]
	code := r.status[req.URL.String()]
	r.mu.Unlock()

	if hook != nil {
		hook(req)
	}

	r.mu.Lock()
	r.inFlight--
	if fail == nil {
		r.calls = append(r.calls, wpushCaptured{
			endpoint: req.URL.String(),
			encoding: req.Header.Get("Content-Encoding"),
			authHdr:  req.Header.Get("Authorization"),
			ttl:      req.Header.Get("TTL"),
			wire:     body,
		})
	}
	r.mu.Unlock()

	if fail != nil {
		return nil, fail
	}
	if code == 0 {
		code = http.StatusCreated
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (r *wpushRecorder) waitCalls() []wpushCaptured {
	wpushBusy.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

type wpushTestReceipts map[string]store.Outcome

func (r wpushTestReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := r[signer+":"+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (r wpushTestReceipts) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	r[signer+":"+nonce] = result
	return nil
}

type wpushLogItem struct {
	dev  identity.Key
	kind string
}

func wpushMakeLog(t *testing.T, first identity.Key, firstKind string, rest ...wpushLogItem) *identity.Log {
	t.Helper()
	log := &identity.Log{User: first.ID()}
	all := append([]wpushLogItem{{dev: first, kind: firstKind}}, rest...)
	for i, item := range all {
		payload := fmt.Sprintf(
			`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":%q,"name":"dev"}`,
			first.ID(), i+1, item.dev.ID(), item.kind,
		)
		if _, err := log.Apply(first.Sign(payload)); err != nil {
			t.Fatal(err)
		}
	}
	return log
}

func wpushTestState(t *testing.T) *State {
	t.Helper()
	s := NewState(wpushGroupID, wpushHomeNode.ID(), wpushBrowserOne.ID())
	s.Logs[wpushBrowserOne.ID()] = wpushMakeLog(
		t,
		wpushBrowserOne, "browser",
		wpushLogItem{dev: wpushBrowserTwo, kind: "browser"},
		wpushLogItem{dev: wpushHomeNode, kind: "node"},
		wpushLogItem{dev: wpushHostNode, kind: "node"},
	)
	s.Logs[wpushGuestDev.ID()] = wpushMakeLog(t, wpushGuestDev, "browser")
	s.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: wpushGroupID},
		Name:      "Website <Alpha> & Co",
		Home:      wpushHomeNode.ID(),
		Secretary: wpushSecMember,
	}
	s.People[wpushOwnerMember] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: wpushOwnerMember},
			Group:     wpushGroupID,
			Kind:      "human",
			User:      wpushBrowserOne.ID(),
			Name:      "Owner",
		},
		Role: "owner",
	}
	s.AIs[wpushSecMember] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: wpushSecMember},
			Group:     wpushGroupID,
			Kind:      "ai",
			User:      wpushBrowserOne.ID(),
			Name:      "Dawn",
		},
		Host:      wpushHostNode.ID(),
		Secretary: true,
	}
	s.AIs[wpushAIMember] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: wpushAIMember},
			Group:     wpushGroupID,
			Kind:      "ai",
			User:      wpushBrowserOne.ID(),
			Name:      "Codex",
		},
		Host: wpushHostNode.ID(),
	}
	s.Tasks[wpushTaskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: wpushTaskID},
		Group:     wpushGroupID,
		Num:       1,
		Title:     "Ship encrypted Web Push reminders\nDetails ignored",
		Slug:      "ship-encrypted-web-push-reminders",
		State:     "merged",
		Step:      "merge",
		Assignee:  wpushAIMember,
	}
	s.Seq = 10
	return s
}

func wpushSignPushOn(by identity.Key, n byte, group string, k wpushTestBrowserKeys, events []string) proto.Envelope {
	cmd := proto.PushOn{
		HomeCommand: proto.HomeCommand{
			Type:      "push",
			Nonce:     proto.ID(wpushTestID(n)),
			Timestamp: wpushTestClock,
			Group:     proto.ID(group),
		},
		Sub:   k.sub,
		VAPID: k.vapid,
		On:    events,
	}
	return by.Sign(string(wpushMarshal(cmd)))
}

func wpushNonCanonicalB64(valid string) string {
	chars := []byte(valid)
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	idx := strings.IndexByte(alphabet, chars[len(chars)-1])
	chars[len(chars)-1] = alphabet[idx^1]
	return string(chars)
}

func TestWebPushCommandFormsAndPrincipalChecks(t *testing.T) {
	wpushInstallRecorder(t)
	s := wpushTestState(t)
	k := wpushMakeBrowserKeys(1, "https://push.example/send/one?a=1&b=2")

	// A node of a member may not send push.
	resNode, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushHostNode.ID(), wpushSignPushOn(wpushHostNode, 1, wpushGroupID, k, []string{"approval"}), wpushTestNow)
	if err != nil || resNode.Answer.Code != "forbidden" {
		t.Fatalf("node push: got %+v, err %v", resNode, err)
	}

	// An AI speaking via as may not send push, even if its host is a browser in the log.
	browserAI := wpushTestID(80)
	s.AIs[browserAI] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: browserAI}, User: wpushBrowserOne.ID()},
		Host:      wpushBrowserOne.ID(),
	}
	asPayload := fmt.Sprintf(
		`{"t":"push","n":%q,"ts":%d,"g":%q,"as":%q,"off":true}`,
		wpushTestID(2), wpushTestClock, wpushGroupID, browserAI,
	)
	resAs, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushBrowserOne.Sign(asPayload), wpushTestNow)
	if err != nil || resAs.Answer.Code != "forbidden" {
		t.Fatalf("AI as push: got %+v, err %v", resAs, err)
	}

	// A browser added after a node, and a node added after a browser, are distinguished;
	// re-adding a browser as a node updates its latest kind, and a missing log is refused.
	nodeFirst := wpushMakeLog(t, wpushHostNode, "node", wpushLogItem{dev: wpushBrowserOne, kind: "browser"})
	if wpushBrowser(nodeFirst, wpushHostNode.ID()) || !wpushBrowser(nodeFirst, wpushBrowserOne.ID()) {
		t.Fatal("expected nodeFirst to classify node=false and browser=true")
	}
	retyped := wpushMakeLog(
		t,
		wpushBrowserOne, "browser",
		wpushLogItem{dev: wpushHostNode, kind: "node"},
		wpushLogItem{dev: wpushBrowserOne, kind: "node"},
	)
	if wpushBrowser(retyped, wpushBrowserOne.ID()) || wpushBrowser(nil, wpushBrowserOne.ID()) {
		t.Fatal("expected retyped device and nil log to be non-browser")
	}

	// off: true wins whatever else the payload holds, while off: false with a subscription is read as PushOn.
	offWithGarbage := fmt.Sprintf(
		`{"t":"push","n":%q,"ts":%d,"g":%q,"off":true,"sub":123,"on":"bad"}`,
		wpushTestID(3), wpushTestClock, wpushGroupID,
	)
	resOff, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushBrowserOne.Sign(offWithGarbage), wpushTestNow)
	if err != nil || !resOff.Answer.OK || len(resOff.Writes) != 1 {
		t.Fatalf("off with extra fields: got %+v, err %v", resOff, err)
	}
	if _, isDrop := resOff.Writes[0].(wpushDrop); !isDrop {
		t.Fatalf("expected wpushDrop, got %T", resOff.Writes[0])
	}

	var onMap map[string]any
	json.Unmarshal(wpushMarshal(proto.PushOn{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: proto.ID(wpushTestID(4)), Timestamp: wpushTestClock, Group: proto.ID(wpushGroupID)},
		Sub:         k.sub,
		VAPID:       k.vapid,
		On:          []string{},
	}), &onMap)
	onMap["off"] = false
	resOnWithFalse, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushBrowserOne.Sign(string(wpushMarshal(onMap))), wpushTestNow)
	if err != nil || !resOnWithFalse.Answer.OK || len(resOnWithFalse.Writes) != 1 {
		t.Fatalf("off:false beside PushOn: got %+v, err %v", resOnWithFalse, err)
	}
	putWrite, isPut := resOnWithFalse.Writes[0].(wpushPut)
	if !isPut || putWrite.sub.events == nil || len(putWrite.sub.events) != 0 {
		t.Fatalf("expected wpushPut with empty non-nil events, got %+v", resOnWithFalse.Writes[0])
	}
}

func TestWebPushValidationRefusals(t *testing.T) {
	wpushInstallRecorder(t)
	s := wpushTestState(t)
	valid := wpushMakeBrowserKeys(2, "https://push.example/send/valid")
	_, otherPub, _ := wpushTestP256(33)

	httpKeys := wpushMakeBrowserKeys(3, "http://127.0.0.1:9999/local")
	resHTTP, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushSignPushOn(wpushBrowserOne, 1, wpushGroupID, httpKeys, []string{"mention", "done"}), wpushTestNow)
	if err != nil || !resHTTP.Answer.OK {
		t.Fatalf("http endpoint should be accepted: %+v, %v", resHTTP, err)
	}

	cases := map[string]func(*proto.PushOn){
		"unparseable endpoint": func(p *proto.PushOn) { p.Sub.Endpoint = "://bad" },
		"unsupported scheme":   func(p *proto.PushOn) { p.Sub.Endpoint = "wss://push.example/send" },
		"empty endpoint host":  func(p *proto.PushOn) { p.Sub.Endpoint = "https:///send" },
		"bad base64 p256dh":    func(p *proto.PushOn) { p.Sub.P256DH = "not+valid/==" },
		"non-canonical p256dh": func(p *proto.PushOn) { p.Sub.P256DH = wpushNonCanonicalB64(valid.sub.P256DH) },
		"non-curve p256dh": func(p *proto.PushOn) {
			p.Sub.P256DH = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 65))
		},
		"bad base64 auth":    func(p *proto.PushOn) { p.Sub.Auth = "not+valid/==" },
		"non-canonical auth": func(p *proto.PushOn) { p.Sub.Auth = wpushNonCanonicalB64(valid.sub.Auth) },
		"short auth": func(p *proto.PushOn) {
			p.Sub.Auth = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 15))
		},
		"long auth": func(p *proto.PushOn) {
			p.Sub.Auth = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 17))
		},
		"non-canonical vapid pub":  func(p *proto.PushOn) { p.VAPID.Public = wpushNonCanonicalB64(valid.vapid.Public) },
		"non-canonical vapid priv": func(p *proto.PushOn) { p.VAPID.Private = wpushNonCanonicalB64(valid.vapid.Private) },
		"zero vapid priv": func(p *proto.PushOn) {
			p.VAPID.Private = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		},
		"mismatched vapid pub": func(p *proto.PushOn) { p.VAPID.Public = otherPub },
		"unknown event":        func(p *proto.PushOn) { p.On = []string{"mention", "task"} },
		"duplicate event":      func(p *proto.PushOn) { p.On = []string{"brief", "done", "brief"} },
	}

	var seq byte = 10
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			seq++
			cmd := proto.PushOn{
				HomeCommand: proto.HomeCommand{
					Type:      "push",
					Nonce:     proto.ID(wpushTestID(seq)),
					Timestamp: wpushTestClock,
					Group:     proto.ID(wpushGroupID),
				},
				Sub:   valid.sub,
				VAPID: valid.vapid,
				On:    []string{"mention", "approval", "decision", "brief", "done"},
			}
			mutate(&cmd)
			res, applyErr := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushBrowserOne.Sign(string(wpushMarshal(cmd))), wpushTestNow)
			if applyErr != nil || res.Answer.Code != "bad_request" || len(res.Writes) != 0 {
				t.Fatalf("%s: got %+v, err %v", name, res, applyErr)
			}
		})
	}

	// off: false without subscription fields fails PushOn decoding with bad_request.
	badOff := fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"off":false}`, wpushTestID(99), wpushTestClock, wpushGroupID)
	resBadOff, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushBrowserOne.Sign(badOff), wpushTestNow)
	if err != nil || resBadOff.Answer.Code != "bad_request" {
		t.Fatalf("off:false alone: got %+v, err %v", resBadOff, err)
	}
}

func TestWebPushReminderKindsTitlesAndNoLeak(t *testing.T) {
	rec := wpushInstallRecorder(t)
	s := wpushTestState(t)
	k := wpushMakeBrowserKeys(4, "https://push.example/send/kinds")

	resSub, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushSignPushOn(wpushBrowserOne, 1, wpushGroupID, k, []string{"mention", "approval", "decision", "brief", "done"}), wpushTestNow)
	if err != nil || !resSub.Answer.OK {
		t.Fatalf("subscribe: %+v, %v", resSub, err)
	}
	wpushAfter(s, resSub)

	// Non-message rows, updated cards (Pos != Seq), and non-pending cards send nothing.
	updatedCard := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(10), Seq: 15},
		Pos:       11,
		Group:     wpushGroupID,
		By:        wpushAIMember,
		Kind:      "approval",
		Body:      wpushMarshal(proto.ApprovalBody{Task: wpushTaskID, AI: wpushAIMember, Tool: "Bash", Command: "rm -rf /", Category: "command", Approvers: []string{wpushBrowserOne.ID()}}),
		State:     &proto.CardState{Status: "pending"},
	}
	nilStateApproval := updatedCard
	nilStateApproval.Pos = 15
	nilStateApproval.State = nil
	nilStateApproval.Mentions = []string{wpushOwnerMember}

	approvedCard := nilStateApproval
	approvedCard.State = &proto.CardState{Status: "approved"}

	decidedCard := approvedCard
	decidedCard.Kind = "decision"

	confirmedBrief := approvedCard
	confirmedBrief.Kind = "brief"

	aiBrief := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(11), Seq: 16},
		Pos:       16,
		Group:     wpushGroupID,
		By:        wpushSecMember,
		Kind:      "brief",
		Body:      wpushMarshal(proto.BriefBody{Title: "Internal brief", Criteria: []string{"c"}, Tasks: []proto.BriefTask{}, Requester: wpushAIMember}),
		State:     &proto.CardState{Status: "pending"},
	}

	aiOnlyMention := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(12), Seq: 17},
		Pos:       17,
		Group:     wpushGroupID,
		By:        wpushOwnerMember,
		Kind:      "text",
		Text:      "Hello AI",
		Mentions:  []string{wpushAIMember},
		Body:      json.RawMessage(`{}`),
	}

	wpushAfter(s, Result{
		Writes: []Write{wpushTestNoopWrite{}},
		Rows: []Row{
			RowOf(s.Tasks[wpushTaskID]),
			RowOf(updatedCard),
			RowOf(nilStateApproval),
			RowOf(approvedCard),
			RowOf(decidedCard),
			RowOf(confirmedBrief),
			RowOf(aiBrief),
			RowOf(aiOnlyMention),
		},
	})
	if got := rec.waitCalls(); len(got) != 0 {
		t.Fatalf("expected 0 pushes for ignored rows, got %d", len(got))
	}

	// 1. Approval with multi-line 81-rune Unicode why, mentions set to owner (must NOT double-send mention), and secret command/text.
	longWhy := strings.Repeat("界", 81) + "\r\nsecond line"
	secretCmd := "curl -H 'Authorization: Bearer topsecret' https://internal.example"
	secretText := "Private approval explanation text"
	approvalMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(20), Seq: 20},
		Pos:       20,
		Group:     wpushGroupID,
		By:        wpushAIMember,
		Kind:      "approval",
		Text:      secretText,
		Mentions:  []string{wpushOwnerMember},
		Body: wpushMarshal(proto.ApprovalBody{
			Task:      wpushTaskID,
			AI:        wpushAIMember,
			Tool:      "Bash",
			Command:   secretCmd,
			Why:       &longWhy,
			Category:  "command",
			Approvers: []string{wpushBrowserOne.ID(), wpushBrowserOne.ID()},
		}),
		State: &proto.CardState{Status: "pending"},
	}

	// 2. Approval without why (title omitted).
	approvalNoWhy := approvalMsg
	approvalNoWhy.ID = wpushTestID(21)
	approvalNoWhy.Body = wpushMarshal(proto.ApprovalBody{
		Task:      wpushTaskID,
		AI:        wpushAIMember,
		Tool:      "Write",
		Command:   secretCmd,
		Category:  "file",
		Approvers: []string{wpushBrowserOne.ID()},
	})

	// 3. Decision with exactly 80 Unicode runes on first line.
	exact80 := strings.Repeat("é", 80) + "\nextra line"
	decisionMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(22), Seq: 22},
		Pos:       22,
		Group:     wpushGroupID,
		By:        wpushAIMember,
		Kind:      "decision",
		Text:      secretText,
		Body: wpushMarshal(proto.DecisionBody{
			Question: exact80,
			Options:  []proto.DecisionOption{},
			To:       []string{wpushBrowserOne.ID()},
		}),
		State: &proto.CardState{Status: "pending"},
	}

	// 4. Brief for human requester.
	briefMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(23), Seq: 23},
		Pos:       23,
		Group:     wpushGroupID,
		By:        wpushSecMember,
		Kind:      "brief",
		Text:      secretText,
		Body: wpushMarshal(proto.BriefBody{
			Title:     "Plan <v1> & rollout\nMore lines",
			Criteria:  []string{"fast"},
			Tasks:     []proto.BriefTask{},
			Requester: wpushOwnerMember,
		}),
		State: &proto.CardState{Status: "pending"},
	}

	// 5. Summary (done) card.
	summaryMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(24), Seq: 24},
		Pos:       24,
		Group:     wpushGroupID,
		By:        wpushSecMember,
		Kind:      "summary",
		Text:      secretText,
		Body:      wpushMarshal(proto.SummaryBody{Task: wpushTaskID, Score: 95, Rounds: 1, Minutes: 5}),
	}

	// 6. Mention by human member (and leading newline title check on wpushTitle).
	if got := wpushTitle("\nsecond"); got != "" {
		t.Fatalf("leading newline title: got %q, want empty", got)
	}
	mentionMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(25), Seq: 25},
		Pos:       25,
		Group:     wpushGroupID,
		By:        wpushOwnerMember,
		Kind:      "text",
		Text:      secretText,
		Mentions:  []string{wpushOwnerMember, wpushOwnerMember},
		Body:      json.RawMessage(`{}`),
	}

	wpushAfter(s, Result{
		Rows: []Row{
			RowOf(approvalMsg),
			RowOf(approvalNoWhy),
			RowOf(decisionMsg),
			RowOf(briefMsg),
			RowOf(summaryMsg),
			RowOf(mentionMsg),
		},
	})

	calls := rec.waitCalls()
	if len(calls) != 6 {
		t.Fatalf("expected 6 pushes, got %d", len(calls))
	}

	want := []proto.Reminder{
		{Group: proto.ID(wpushGroupID), Msg: proto.ID(wpushTestID(20)), GroupName: "Website <Alpha> & Co", Kind: "approval", Who: "Codex", Title: strings.Repeat("界", 80)},
		{Group: proto.ID(wpushGroupID), Msg: proto.ID(wpushTestID(21)), GroupName: "Website <Alpha> & Co", Kind: "approval", Who: "Codex"},
		{Group: proto.ID(wpushGroupID), Msg: proto.ID(wpushTestID(22)), GroupName: "Website <Alpha> & Co", Kind: "decision", Who: "Codex", Title: strings.Repeat("é", 80)},
		{Group: proto.ID(wpushGroupID), Msg: proto.ID(wpushTestID(23)), GroupName: "Website <Alpha> & Co", Kind: "brief", Who: "Dawn", Title: "Plan <v1> & rollout"},
		{Group: proto.ID(wpushGroupID), Msg: proto.ID(wpushTestID(24)), GroupName: "Website <Alpha> & Co", Kind: "done", Who: "Dawn", Title: "Ship encrypted Web Push reminders"},
		{Group: proto.ID(wpushGroupID), Msg: proto.ID(wpushTestID(25)), GroupName: "Website <Alpha> & Co", Kind: "mention", Who: "Owner"},
	}

	for i, call := range calls {
		if call.encoding != "aes128gcm" || call.ttl != "86400" || !strings.Contains(call.authHdr, "k="+k.vapid.Public) {
			t.Fatalf("call %d headers: encoding=%q ttl=%q auth=%q", i, call.encoding, call.ttl, call.authHdr)
		}
		if bytes.Contains(call.wire, []byte(secretCmd)) || bytes.Contains(call.wire, []byte(secretText)) || bytes.Contains(call.wire, []byte("Codex")) {
			t.Fatalf("call %d leaked cleartext on wire", i)
		}
		plain := wpushDecryptRFC8291(t, k, call.wire)
		if bytes.Contains(plain, []byte(`\u003c`)) || bytes.Contains(plain, []byte(`\u0026`)) {
			t.Fatalf("call %d escaped HTML in JSON: %s", i, plain)
		}
		if bytes.Contains(plain, []byte(secretCmd)) || bytes.Contains(plain, []byte(secretText)) {
			t.Fatalf("call %d leaked command or text in plaintext: %s", i, plain)
		}
		if nulls := proto.UnexpectedNulls(plain); len(nulls) != 0 {
			t.Fatalf("call %d unexpected nulls %v in %s", i, nulls, plain)
		}
		var got proto.Reminder
		if err := proto.Decode(plain, &got); err != nil {
			t.Fatalf("call %d decode reminder %s: %v", i, plain, err)
		}
		if got != want[i] {
			t.Fatalf("call %d got %+v, want %+v", i, got, want[i])
		}
		if want[i].Title == "" && bytes.Contains(plain, []byte(`"title"`)) {
			t.Fatalf("call %d must omit absent title: %s", i, plain)
		}
	}
}

type wpushTestNoopWrite struct{}

func (wpushTestNoopWrite) Put(*store.Tx) error { return nil }

func TestWebPushEligibilityPerBrowserMemberAndRevocation(t *testing.T) {
	rec := wpushInstallRecorder(t)
	s := wpushTestState(t)
	s.People[wpushGuestMember] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: wpushGuestMember},
			Group:     wpushGroupID,
			Kind:      "human",
			User:      wpushGuestDev.ID(),
			Name:      "Guest",
		},
		Role: "member",
	}

	kOne := wpushMakeBrowserKeys(11, "https://push.example/send/b1")
	kTwo := wpushMakeBrowserKeys(12, "https://push.example/send/b2")
	kGuest := wpushMakeBrowserKeys(13, "https://push.example/send/guest")

	// Subscribe browserOne (approval only), browserTwo (decision + approval), and guest (approval).
	for _, item := range []struct {
		by     identity.Key
		keys   wpushTestBrowserKeys
		events []string
	}{
		{wpushBrowserOne, kOne, []string{"approval"}},
		{wpushBrowserTwo, kTwo, []string{"decision", "approval"}},
		{wpushGuestDev, kGuest, []string{"approval"}},
	} {
		res, err := Commands.Apply(s, nil, wpushTestReceipts{}, item.by.ID(), wpushSignPushOn(item.by, 1, wpushGroupID, item.keys, item.events), wpushTestNow)
		if err != nil || !res.Answer.OK {
			t.Fatalf("subscribe: %+v, %v", res, err)
		}
		wpushAfter(s, res)
	}

	// Owner's browserOne and hostNode are online; browserTwo and guest are offline.
	// Approval concerns ONLY owner (not guest, even though guest is a member with approval enabled).
	s.Online[wpushBrowserOne.ID()] = true
	s.Online[wpushHostNode.ID()] = true

	approvalForOwner := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(30), Seq: 30},
		Pos:       30,
		Group:     wpushGroupID,
		By:        wpushAIMember,
		Kind:      "approval",
		Mentions:  []string{wpushOwnerMember, wpushGuestMember},
		Body: wpushMarshal(proto.ApprovalBody{
			Task:      wpushTaskID,
			AI:        wpushAIMember,
			Tool:      "Bash",
			Command:   "npm install",
			Category:  "command",
			Approvers: []string{wpushBrowserOne.ID()},
		}),
		State: &proto.CardState{Status: "pending"},
	}
	wpushAfter(s, Result{Rows: []Row{RowOf(approvalForOwner)}})
	calls := rec.waitCalls()
	if len(calls) != 1 || calls[0].endpoint != kTwo.sub.Endpoint {
		t.Fatalf("expected only offline browserTwo to receive push, got %+v", calls)
	}

	// BrowserOne goes offline too, and a decision arrives: browserOne subscribed only to approval, so only browserTwo gets it.
	delete(s.Online, wpushBrowserOne.ID())
	decisionForOwner := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(31), Seq: 31},
		Pos:       31,
		Group:     wpushGroupID,
		By:        wpushAIMember,
		Kind:      "decision",
		Body: wpushMarshal(proto.DecisionBody{
			Question: "Which port?",
			Options:  []proto.DecisionOption{},
			To:       []string{wpushBrowserOne.ID()},
		}),
		State: &proto.CardState{Status: "pending"},
	}
	wpushAfter(s, Result{Rows: []Row{RowOf(decisionForOwner)}})
	calls = rec.waitCalls()
	if len(calls) != 2 || calls[1].endpoint != kTwo.sub.Endpoint {
		t.Fatalf("expected only browserTwo to receive decision, got %+v", calls)
	}

	// Remove guest from s.People (while guest remains in s.Logs and subscribed on this home):
	// an approval naming guest in Approvers must NOT notify a non-member.
	delete(s.People, wpushGuestMember)
	approvalNamingRemovedGuest := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(32), Seq: 32},
		Pos:       32,
		Group:     wpushGroupID,
		By:        wpushAIMember,
		Kind:      "approval",
		Body: wpushMarshal(proto.ApprovalBody{
			Task:      wpushTaskID,
			AI:        wpushAIMember,
			Tool:      "Bash",
			Command:   "npm test",
			Category:  "command",
			Approvers: []string{wpushGuestDev.ID()},
		}),
		State: &proto.CardState{Status: "pending"},
	}
	wpushAfter(s, Result{Rows: []Row{RowOf(approvalNamingRemovedGuest)}})
	if calls = rec.waitCalls(); len(calls) != 2 {
		t.Fatalf("removed member should receive nothing, got %d calls", len(calls))
	}

	// Revoke browserTwo in owner's device log: browserTwo must no longer receive pushes, while browserOne still does.
	revokePayload := fmt.Sprintf(
		`{"t":"device","user":%q,"ver":5,"op":"revoke","dev":%q}`,
		wpushBrowserOne.ID(), wpushBrowserTwo.ID(),
	)
	revokeRow := proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: wpushBrowserOne.ID() + ":5", Seq: 33},
		DeviceChange: proto.DeviceChange{User: wpushBrowserOne.ID(), Version: 5, Op: "revoke", Device: wpushBrowserTwo.ID()},
		Env:          wpushBrowserOne.Sign(revokePayload),
	}
	s.Commit(Result{Rows: []Row{RowOf(revokeRow)}})

	approvalAfterRevoke := approvalForOwner
	approvalAfterRevoke.ID = wpushTestID(34)
	approvalAfterRevoke.Seq = 34
	approvalAfterRevoke.Pos = 34
	wpushAfter(s, Result{Rows: []Row{RowOf(approvalAfterRevoke)}})
	calls = rec.waitCalls()
	if len(calls) != 3 || calls[2].endpoint != kOne.sub.Endpoint {
		t.Fatalf("expected only unrevoked browserOne to receive push, got %+v", calls)
	}
}

func TestWebPushEvictionErrorsQueueBoundsAndConcurrency(t *testing.T) {
	rec := wpushInstallRecorder(t)
	s := wpushTestState(t)

	k404 := wpushMakeBrowserKeys(21, "https://push.example/send/404")
	k410 := wpushMakeBrowserKeys(22, "https://push.example/send/410")
	s.Logs[wpushBrowserOne.ID()] = wpushMakeLog(
		t,
		wpushBrowserOne, "browser",
		wpushLogItem{dev: wpushBrowserTwo, kind: "browser"},
		wpushLogItem{dev: wpushHomeNode, kind: "node"},
		wpushLogItem{dev: wpushHostNode, kind: "node"},
	)

	for _, item := range []struct {
		by   identity.Key
		keys wpushTestBrowserKeys
	}{
		{wpushBrowserOne, k404},
		{wpushBrowserTwo, k410},
	} {
		res, err := Commands.Apply(s, nil, wpushTestReceipts{}, item.by.ID(), wpushSignPushOn(item.by, 1, wpushGroupID, item.keys, []string{"mention"}), wpushTestNow)
		if err != nil || !res.Answer.OK {
			t.Fatalf("subscribe: %+v, %v", res, err)
		}
		wpushAfter(s, res)
	}

	mention := func(n byte) Result {
		return Result{Rows: []Row{RowOf(proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: wpushTestID(n), Seq: int64(n)},
			Pos:       int64(n),
			Group:     wpushGroupID,
			By:        wpushAIMember,
			Kind:      "text",
			Mentions:  []string{wpushOwnerMember},
			Body:      json.RawMessage(`{}`),
		})}}
	}

	// Network error and 500 leave subscriptions intact.
	rec.errFor[k404.sub.Endpoint] = errors.New("network unreachable")
	rec.status[k410.sub.Endpoint] = http.StatusInternalServerError
	wpushAfter(s, mention(40))
	if got := rec.waitCalls(); len(got) != 1 {
		t.Fatalf("expected 1 completed HTTP response and 1 network failure, got %d", len(got))
	}
	delete(rec.errFor, k404.sub.Endpoint)

	// 404 evicts k404; for k410, re-subscribe browserTwo to a fresh endpoint while 410 is in flight so the newer sub stays.
	kFresh := wpushMakeBrowserKeys(23, "https://push.example/send/fresh")
	rec.status[k404.sub.Endpoint] = http.StatusNotFound
	rec.status[k410.sub.Endpoint] = http.StatusGone
	rec.onCall = func(req *http.Request) {
		if req.URL.String() == k410.sub.Endpoint {
			resUpdate, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserTwo.ID(), wpushSignPushOn(wpushBrowserTwo, 2, wpushGroupID, kFresh, []string{"mention"}), wpushTestNow)
			if err == nil && resUpdate.Answer.OK {
				wpushAfter(s, resUpdate)
			}
		}
	}
	wpushAfter(s, mention(41))
	rec.waitCalls()
	rec.onCall = nil

	// Next mention: k404 was evicted, while kFresh on browserTwo was preserved and now gets 410 to evict it too.
	rec.status[kFresh.sub.Endpoint] = http.StatusGone
	wpushAfter(s, mention(42))
	calls := rec.waitCalls()
	if calls[len(calls)-1].endpoint != kFresh.sub.Endpoint || len(calls) != 4 {
		t.Fatalf("expected k404 evicted and kFresh called once, got %d calls: %+v", len(calls), calls)
	}
	wpushAfter(s, mention(43))
	if calls = rec.waitCalls(); len(calls) != 4 {
		t.Fatalf("expected kFresh evicted after 410, got %d calls", len(calls))
	}

	// Bounded worker pool (wpushWorkers = 4) and bounded queue (wpushQueueCap = 64), plus context deadline.
	kPool := wpushMakeBrowserKeys(24, "https://push.example/send/pool")
	resPool, err := Commands.Apply(s, nil, wpushTestReceipts{}, wpushBrowserOne.ID(), wpushSignPushOn(wpushBrowserOne, 3, wpushGroupID, kPool, []string{"mention"}), wpushTestNow)
	if err != nil || !resPool.Answer.OK {
		t.Fatalf("subscribe pool: %+v, %v", resPool, err)
	}
	wpushAfter(s, resPool)

	wpushTimeout = 40 * time.Millisecond
	entered := make(chan struct{}, wpushWorkers)
	release := make(chan struct{})
	var sawDeadline sync.Once
	deadlineOK := false
	rec.onCall = func(req *http.Request) {
		sawDeadline.Do(func() {
			_, deadlineOK = req.Context().Deadline()
		})
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}

	// Start 4 workers and wait until all 4 are inside Do, then fill all 64 queue slots and drop the 65th.
	for i := range wpushWorkers {
		wpushAfter(s, mention(byte(50+i)))
	}
	for range wpushWorkers {
		<-entered
	}
	for i := range wpushQueueCap + 1 {
		wpushAfter(s, mention(byte(60+i)))
	}
	close(release)
	calls = rec.waitCalls()
	rec.onCall = nil

	if !deadlineOK {
		t.Fatal("expected request context to carry a timeout deadline")
	}
	if rec.maxSeen != wpushWorkers {
		t.Fatalf("expected max %d concurrent workers, got %d", wpushWorkers, rec.maxSeen)
	}
	if got := len(calls) - 4; got != wpushQueueCap+wpushWorkers {
		t.Fatalf("expected %d accepted pushes (%d active + %d queued), got %d", wpushQueueCap+wpushWorkers, wpushWorkers, wpushQueueCap, got)
	}

	// After draining to 0 running workers, a subsequent push starts a worker again and delivers.
	wpushAfter(s, mention(150))
	if got := len(rec.waitCalls()) - 4; got != wpushQueueCap+wpushWorkers+1 {
		t.Fatalf("expected post-drain push to deliver, got %d", got)
	}
}

type wpushLoopPeer struct {
	frames chan any
}

func (p *wpushLoopPeer) Send(frame any) error  { p.frames <- frame; return nil }
func (p *wpushLoopPeer) Done() <-chan struct{} { return nil }

func (p *wpushLoopPeer) next(t *testing.T) any {
	t.Helper()
	select {
	case f := <-p.frames:
		return f
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for frame")
		return nil
	}
}

func TestWebPushLoopPersistenceAcrossGroupsRestartsAndRollbacks(t *testing.T) {
	rec := wpushInstallRecorder(t)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tables := Tables{
		Commands: Table{
			"group/create": func(s *State, c Command) (Result, *Refusal) {
				var p struct {
					Name string `json:"name"`
				}
				if refusal := c.Decode(&p); refusal != nil {
					return Result{}, refusal
				}
				grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Kind: "project", Home: s.Home, Checks: []string{}, Created: c.Now}
				own := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: wpushOwnerMember}, Group: s.ID, Kind: "human", User: c.Who.User, Name: "Owner"}, Role: "owner"}
				aiRow := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: wpushAIMember}, Group: s.ID, Kind: "ai", User: c.Who.User, Name: "Codex"}, Host: wpushHostNode.ID()}
				return Result{Rows: []Row{RowOf(grp), RowOf(own), RowOf(aiRow)}, GroupsChanged: true}, nil
			},
			"post/text": func(s *State, c Command) (Result, *Refusal) {
				var p struct {
					Text     string     `json:"text"`
					Mentions []proto.ID `json:"mentions"`
				}
				if refusal := c.Decode(&p); refusal != nil {
					return Result{}, refusal
				}
				mentions := make([]string, len(p.Mentions))
				for i, m := range p.Mentions {
					mentions[i] = string(m)
				}
				msg := proto.MsgRow{
					RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)},
					Group:     s.ID,
					By:        c.Who.Member(),
					Kind:      "text",
					Timestamp: c.Now,
					Text:      p.Text,
					Mentions:  mentions,
					Body:      json.RawMessage(`{}`),
					Env:       &c.Env,
				}
				return Result{Rows: []Row{RowOf(msg)}}, nil
			},
			"push": Commands["push"],
		},
		After: Rules[Hook]{"push": After["push"]},
	}

	makeLogs := func(user string) *identity.Log {
		if user != wpushBrowserOne.ID() {
			return nil
		}
		return wpushMakeLog(
			t,
			wpushBrowserOne, "browser",
			wpushLogItem{dev: wpushBrowserTwo, kind: "browser"},
			wpushLogItem{dev: wpushHomeNode, kind: "node"},
			wpushLogItem{dev: wpushHostNode, kind: "node"},
		)
	}

	start := func() *Loop {
		loop, startErr := StartLoop(LoopConfig{
			Store:  db,
			Home:   wpushHomeNode.ID(),
			Owner:  wpushBrowserOne.ID(),
			Logs:   makeLogs,
			Tables: tables,
			Now:    func() time.Time { return wpushTestNow },
			Report: func(error) {},
		})
		if startErr != nil {
			t.Fatal(startErr)
		}
		return loop
	}

	loop := start()
	bPeer := &wpushLoopPeer{frames: make(chan any, 16)}
	bConn := loop.Attach(wpushBrowserOne.ID(), bPeer)
	hPeer := &wpushLoopPeer{frames: make(chan any, 16)}
	hConn := loop.Attach(wpushHostNode.ID(), hPeer)

	// Create group 1 and group 2, and subscribe browserOne on group 1 only.
	createG1 := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"One"}`, wpushGroupID, wpushTestClock, wpushGroupID)
	loop.Cmd(bConn, 1, wpushBrowserOne.Sign(createG1))
	bPeer.next(t)

	createG2 := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Two"}`, wpushGroupTwoID, wpushTestClock, wpushGroupTwoID)
	loop.Cmd(bConn, 2, wpushBrowserOne.Sign(createG2))
	bPeer.next(t)

	k := wpushMakeBrowserKeys(31, "https://push.example/send/persisted")
	loop.Cmd(bConn, 3, wpushSignPushOn(wpushBrowserOne, 90, wpushGroupID, k, []string{"mention"}))
	if ans, ok := bPeer.next(t).(proto.CmdResult); !ok || !bool(ans.OK) {
		t.Fatalf("push subscribe failed: %+v", ans)
	}

	// Simulate restart: wipe in-memory map, restart loop, and call LoadPushSubscriptions.
	loop.Close()
	wpushMu.Lock()
	wpushSubs = map[string]map[string]wpushSub{}
	wpushMu.Unlock()

	loop = start()
	if err := loop.LoadPushSubscriptions(); err != nil {
		t.Fatalf("LoadPushSubscriptions: %v", err)
	}

	// Only hostNode connects after restart; browserOne stays offline. Mentioning owner in group 2 triggers push.
	hPeer = &wpushLoopPeer{frames: make(chan any, 16)}
	hConn = loop.Attach(wpushHostNode.ID(), hPeer)
	postMention := fmt.Sprintf(
		`{"t":"post","kind":"text","n":%q,"ts":%d,"g":%q,"as":%q,"text":"Check group two","mentions":[%q]}`,
		wpushTestID(91), wpushTestClock, wpushGroupTwoID, wpushAIMember, wpushOwnerMember,
	)
	loop.Cmd(hConn, 1, wpushHostNode.Sign(postMention))
	hPeer.next(t)

	calls := rec.waitCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 push after restart in second group, got %d", len(calls))
	}
	var rem proto.Reminder
	if err := proto.Decode(wpushDecryptRFC8291(t, k, calls[0].wire), &rem); err != nil {
		t.Fatal(err)
	}
	if rem.Group != proto.ID(wpushGroupTwoID) || rem.GroupName != "Two" || rem.Kind != "mention" || rem.Who != "Codex" {
		t.Fatalf("unexpected reminder %+v", rem)
	}

	// Reconnect browserOne and send off: true -> deletes from store and memory.
	bPeer = &wpushLoopPeer{frames: make(chan any, 16)}
	bConn = loop.Attach(wpushBrowserOne.ID(), bPeer)
	offCmd := fmt.Sprintf(`{"t":"push","n":%q,"ts":%d,"g":%q,"off":true}`, wpushTestID(92), wpushTestClock, wpushGroupID)
	loop.Cmd(bConn, 1, wpushBrowserOne.Sign(offCmd))
	if ans, ok := bPeer.next(t).(proto.CmdResult); !ok || !bool(ans.OK) {
		t.Fatalf("push off failed: %+v", ans)
	}
	loop.Detach(bConn)
	<-bConn.Gone()

	postAgain := fmt.Sprintf(
		`{"t":"post","kind":"text","n":%q,"ts":%d,"g":%q,"as":%q,"text":"After off","mentions":[%q]}`,
		wpushTestID(93), wpushTestClock, wpushGroupTwoID, wpushAIMember, wpushOwnerMember,
	)
	loop.Cmd(hConn, 2, wpushHostNode.Sign(postAgain))
	hPeer.next(t)
	if calls = rec.waitCalls(); len(calls) != 1 {
		t.Fatalf("expected no push after off:true, got %d", len(calls))
	}

	// Error paths in LoadPushSubscriptions and transaction rollback on failed Put:
	// corrupt/nullable push_sub fails scan, and dropping push_sub rolls back PushOn without touching memory.
	badRow := func(tx *store.Tx) error {
		_, execErr := tx.Exec("DROP TABLE push_sub; CREATE TABLE push_sub(device TEXT, user TEXT, sub TEXT, vapid TEXT, events TEXT); INSERT INTO push_sub VALUES('d', NULL, '', '', '')")
		return execErr
	}
	if err := db.Write(badRow); err != nil {
		t.Fatal(err)
	}
	if err := loop.LoadPushSubscriptions(); err == nil {
		t.Fatal("expected scan error for NULL column in push_sub")
	}

	badJSONRow := func(tx *store.Tx) error {
		_, execErr := tx.Exec("DELETE FROM push_sub; INSERT INTO push_sub VALUES('d', 'u', 'bad', '{}', '[]')")
		return execErr
	}
	if err := db.Write(badJSONRow); err != nil {
		t.Fatal(err)
	}
	if err := loop.LoadPushSubscriptions(); err == nil {
		t.Fatal("expected decode error for corrupt JSON in push_sub")
	}

	dropTable := func(tx *store.Tx) error {
		_, execErr := tx.Exec("DROP TABLE push_sub")
		return execErr
	}
	if err := db.Write(dropTable); err != nil {
		t.Fatal(err)
	}
	if err := loop.LoadPushSubscriptions(); err == nil {
		t.Fatal("expected query error when push_sub table is missing")
	}

	// A PushOn whose transaction fails in Put must NOT update wpushSubs in memory.
	bPeer2 := &wpushLoopPeer{frames: make(chan any, 16)}
	bConn2 := loop.Attach(wpushBrowserTwo.ID(), bPeer2)
	kRollback := wpushMakeBrowserKeys(32, "https://push.example/send/rollback")
	loop.Cmd(bConn2, 1, wpushSignPushOn(wpushBrowserTwo, 94, wpushGroupID, kRollback, []string{"mention"}))
	if ans, ok := bPeer2.next(t).(proto.ErrorResponse); !ok || ans.Err.Code != "internal" {
		t.Fatalf("expected internal error on failed Put, got %+v", ans)
	}
	wpushMu.Lock()
	_, leaked := wpushSubs[wpushHomeNode.ID()][wpushBrowserTwo.ID()]
	wpushMu.Unlock()
	if leaked {
		t.Fatal("rolled-back PushOn must not populate in-memory subscription map")
	}

	loop.Close()
}
