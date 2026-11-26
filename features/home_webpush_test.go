//go:build acceptance

package features

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
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_webpush"] = func(sc *godog.ScenarioContext, _ fixtures) { homeWebPushSteps(sc) }
}

type homeWebPushBrowser struct {
	key      identity.Key
	uaPriv   *ecdh.PrivateKey
	auth     []byte
	sub      proto.PushSubscription
	vapid    proto.VAPIDKeys
	conn     *home.Conn
	peer     *homeRunPeer
	mu       sync.Mutex
	received [][]byte
}

func (b *homeWebPushBrowser) count() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([][]byte(nil), b.received...)
}

func (b *homeWebPushBrowser) take(req *http.Request) int {
	if req.Method != http.MethodPost || req.Header.Get("Content-Encoding") != "aes128gcm" {
		return http.StatusBadRequest
	}
	if !strings.Contains(req.Header.Get("Authorization"), "k="+b.vapid.Public) {
		return http.StatusUnauthorized
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return http.StatusInternalServerError
	}
	b.mu.Lock()
	b.received = append(b.received, body)
	b.mu.Unlock()
	return http.StatusCreated
}

func (b *homeWebPushBrowser) waitCount(want int) ([][]byte, error) {
	if want == 0 {
		time.Sleep(50 * time.Millisecond)
		got := b.count()
		if len(got) != 0 {
			return got, fmt.Errorf("push endpoint got %d notifications; want 0", len(got))
		}
		return got, nil
	}
	err := waitUntil(3*time.Second, fmt.Sprintf("%d push notifications", want), func() error {
		if n := len(b.count()); n != want {
			return fmt.Errorf("%d notifications", n)
		}
		return nil
	})
	return b.count(), err
}

type homeWebPushWorld struct {
	now       time.Time
	db        *store.Store
	loop      *home.Loop
	server    *httptest.Server
	homeKey   identity.Key
	hostKey   identity.Key
	hostConn  *home.Conn
	hostPeer  *homeRunPeer
	browsers  map[string]*homeWebPushBrowser
	groups    map[string]string
	aiID      string
	lastMsgID string
	nonce     int
}

func homeWebPushSteps(sc *godog.ScenarioContext) {
	w := &homeWebPushWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a home with the group "([^"]*)", two browsers of the owner, and the owner's node hosting "([^"]*)"$`, func(group, ai string) error {
		return w.setup(cleanup, group, ai)
	})
	sc.Step(`^(the first browser|the second browser) subscribes to Web Push for "([^"]*)"$`, w.subscribe)
	sc.Step(`^the first browser disconnects while the second browser and the owner's node stay connected$`, w.disconnectFirst)
	sc.Step(`^"([^"]*)" posts an approval card in "([^"]*)" for command "([^"]*)" with why "([^"]*)" and text "([^"]*)"$`, w.postApproval)
	sc.Step(`^the push endpoint of (the first browser|the second browser) receives (\d+) (?:encrypted )?notifications?$`, w.checkCount)
	sc.Step(`^the push endpoint of (the first browser|the second browser) receives no further notifications$`, w.checkNoMore)
	sc.Step(`^the wire bytes received by the push endpoint contain none of "([^"]*)", "([^"]*)", "([^"]*)" or "([^"]*)"$`, w.checkWireOpaque)
	sc.Step(`^decrypted by the first browser, the reminder is for group "([^"]*)", kind "([^"]*)", who "([^"]*)" and title "([^"]*)"(?: and code "([^"]*)")?, with none of "([^"]*)"(?: or "([^"]*)")?$`, w.checkDecrypted)
	sc.Step(`^the second browser creates the group "([^"]*)" on the same home with "([^"]*)"$`, w.createGroup)
	sc.Step(`^the home restarts and loads its push subscriptions while only the owner's node reconnects$`, func() error {
		return w.restart(cleanup)
	})
	sc.Step(`^"([^"]*)" posts a text message "([^"]*)" mentioning the owner in "([^"]*)"$`, w.postMention)
	sc.Step(`^the owner's node reports "([^"]*)" in "([^"]*)" with "login_invalid" and detail "([^"]*)"$`, w.reportLogin)
	sc.Step(`^the first browser reconnects, turns Web Push off, and disconnects$`, w.turnOffFirst)
}

func homeWebPushP256(seed byte) (*ecdh.PrivateKey, string, string, error) {
	priv, err := ecdh.P256().NewPrivateKey(bytes.Repeat([]byte{seed}, 32))
	if err != nil {
		return nil, "", "", err
	}
	return priv, base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(priv.Bytes()), nil
}

func (w *homeWebPushWorld) newBrowser(label string, seed byte, path string) (*homeWebPushBrowser, error) {
	uaPriv, p256dh, _, err := homeWebPushP256(seed)
	if err != nil {
		return nil, err
	}
	_, vapidPub, vapidPriv, err := homeWebPushP256(seed + 10)
	if err != nil {
		return nil, err
	}
	auth := bytes.Repeat([]byte{seed + 20}, 16)
	return &homeWebPushBrowser{
		key:    exampleKey("wpush " + label),
		uaPriv: uaPriv,
		auth:   auth,
		sub: proto.PushSubscription{
			Endpoint: w.server.URL + path,
			P256DH:   p256dh,
			Auth:     base64.RawURLEncoding.EncodeToString(auth),
		},
		vapid: proto.VAPIDKeys{Public: vapidPub, Private: vapidPriv},
	}, nil
}

func (w *homeWebPushWorld) ownerLog() (*identity.Log, error) {
	b1 := w.browsers["the first browser"].key
	b2 := w.browsers["the second browser"].key
	log := &identity.Log{User: b1.ID()}
	items := []struct {
		dev  identity.Key
		kind string
	}{
		{b1, "browser"},
		{b2, "browser"},
		{w.homeKey, "node"},
		{w.hostKey, "node"},
	}
	for i, it := range items {
		p := fmt.Sprintf(`{"t":"device","user":%q,"ver":%d,"op":"add","dev":%q,"kind":%q,"name":%q}`, b1.ID(), i+1, it.dev.ID(), it.kind, it.kind)
		if _, err := log.Apply(b1.Sign(p)); err != nil {
			return nil, err
		}
	}
	return log, nil
}

func (w *homeWebPushWorld) setup(cleanup func(func()), groupName, aiName string) error {
	mux := http.NewServeMux()
	w.server = httptest.NewServer(mux)
	cleanup(w.server.Close)

	w.homeKey = exampleKey("wpush home")
	w.hostKey = exampleKey("wpush host")
	w.aiID = string(exampleID("wpush ai member"))
	w.groups = map[string]string{}
	w.browsers = map[string]*homeWebPushBrowser{}

	b1, err := w.newBrowser("first", 41, "/push/first")
	if err != nil {
		return err
	}
	b2, err := w.newBrowser("second", 51, "/push/second")
	if err != nil {
		return err
	}
	w.browsers["the first browser"] = b1
	w.browsers["the second browser"] = b2
	mux.HandleFunc("/push/first", func(rw http.ResponseWriter, req *http.Request) { rw.WriteHeader(b1.take(req)) })
	mux.HandleFunc("/push/second", func(rw http.ResponseWriter, req *http.Request) { rw.WriteHeader(b2.take(req)) })

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db

	if err := w.startLoop(cleanup, true); err != nil {
		return err
	}
	return w.createWith("the first browser", groupName, aiName)
}

func (w *homeWebPushWorld) startLoop(cleanup func(func()), attachBrowsers bool) error {
	owner := w.browsers["the first browser"].key.ID()
	seedLog, err := w.ownerLog()
	if err != nil {
		return err
	}
	entries := append([]proto.Envelope(nil), seedLog.Entries...)
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db,
		Home:  w.homeKey.ID(),
		Owner: owner,
		Logs: func(user string) *identity.Log {
			if user != owner {
				return nil
			}
			replayed, err := identity.Replay(owner, entries)
			if err != nil {
				return nil
			}
			return replayed
		},
		Tables: home.NodeTables(),
		Now:    func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop

	w.hostPeer = &homeRunPeer{frames: make(chan any, 32)}
	w.hostConn = loop.Attach(w.hostKey.ID(), w.hostPeer)
	if !attachBrowsers {
		return nil
	}
	for _, b := range w.browsers {
		b.peer = &homeRunPeer{frames: make(chan any, 32)}
		b.conn = loop.Attach(b.key.ID(), b.peer)
	}
	return nil
}

func (w *homeWebPushWorld) nextID() proto.ID {
	w.nonce++
	return exampleID(fmt.Sprintf("wpush nonce %d", w.nonce))
}

func (w *homeWebPushWorld) execCmd(conn *home.Conn, peer *homeRunPeer, signer identity.Key, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	peer.re++
	w.loop.Cmd(conn, peer.re, signer.Sign(string(raw)))
	got, err := peer.next()
	if err != nil {
		return err
	}
	want := fmt.Sprintf(`{"re":%d,"ok":true}`, peer.re)
	if got != want {
		return fmt.Errorf("command answered %s; want %s", got, want)
	}
	return nil
}

func (w *homeWebPushWorld) ownerMember(groupID string) proto.ID {
	sum := sha256.Sum256([]byte("SoloDawn group owner " + groupID))
	return proto.ID(base64.RawURLEncoding.EncodeToString(sum[:16]))
}

func (w *homeWebPushWorld) createWith(who, groupName, aiName string) error {
	gid := w.nextID()
	w.groups[groupName] = string(gid)
	b := w.browsers[who]
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: w.now.UnixMilli(), Group: gid},
			Op:          "create",
		},
		Name: groupName, MemberName: "Owner",
	}
	if err := w.execCmd(b.conn, b.peer, b.key, create); err != nil {
		return err
	}
	addAI := proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: gid},
			Op:          "add_ai", ID: proto.ID(w.aiID),
		},
		CLI: "codex", Name: aiName, Workspace: "/work/" + aiName, Account: "api_key", Approvers: []string{},
	}
	return w.execCmd(w.hostConn, w.hostPeer, w.hostKey, addAI)
}

func (w *homeWebPushWorld) createGroup(groupName, aiName string) error {
	return w.createWith("the second browser", groupName, aiName)
}

func (w *homeWebPushWorld) subscribe(who, rawEvents string) error {
	b := w.browsers[who]
	events := strings.Split(rawEvents, ", ")
	var firstGroup string
	for _, g := range w.groups {
		firstGroup = g
		break
	}
	return w.execCmd(b.conn, b.peer, b.key, proto.PushOn{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(firstGroup)},
		Sub:         b.sub,
		VAPID:       b.vapid,
		On:          events,
	})
}

func (w *homeWebPushWorld) disconnectFirst() error {
	b := w.browsers["the first browser"]
	w.loop.Detach(b.conn)
	<-b.conn.Gone()
	return nil
}

func (w *homeWebPushWorld) postApproval(_, groupName, command, why, text string) error {
	msgID := w.nextID()
	w.lastMsgID = string(msgID)
	whyLine := why + "\nAdditional CLI detail line"
	gid := w.groups[groupName]
	post := proto.Post[proto.ApprovalBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     msgID,
			Timestamp: w.now.UnixMilli(),
			Group:     proto.ID(gid),
			As:        proto.ID(w.aiID),
		},
		Kind:     "approval",
		Text:     text,
		Mentions: []proto.ID{w.ownerMember(gid)},
		Body: proto.ApprovalBody{
			AI:        w.aiID,
			Tool:      "Bash",
			Command:   command,
			Why:       &whyLine,
			Category:  "command",
			Approvers: []string{w.browsers["the first browser"].key.ID()},
		},
	}
	return w.execCmd(w.hostConn, w.hostPeer, w.hostKey, post)
}

func (w *homeWebPushWorld) postMention(_, text, groupName string) error {
	msgID := w.nextID()
	w.lastMsgID = string(msgID)
	gid := w.groups[groupName]
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     msgID,
			Timestamp: w.now.UnixMilli(),
			Group:     proto.ID(gid),
			As:        proto.ID(w.aiID),
		},
		Kind:     "text",
		Text:     text,
		Mentions: []proto.ID{w.ownerMember(gid)},
		Body:     proto.EmptyBody{},
	}
	return w.execCmd(w.hostConn, w.hostPeer, w.hostKey, post)
}

func (w *homeWebPushWorld) reportLogin(_, groupName, detail string) error {
	msgID := w.nextID()
	w.lastMsgID = string(msgID)
	return w.execCmd(w.hostConn, w.hostPeer, w.hostKey, map[string]any{
		"t": "member", "op": "status", "n": msgID, "ts": w.now.UnixMilli(), "g": w.groups[groupName],
		"id": w.aiID, "state": "limited", "path": "local", "usage": map[string]any{},
		"event": map[string]any{"code": "login_invalid", "detail": detail},
	})
}

func (w *homeWebPushWorld) restart(cleanup func(func())) error {
	w.loop.Close()
	return w.startLoop(cleanup, false)
}

func (w *homeWebPushWorld) turnOffFirst() error {
	b := w.browsers["the first browser"]
	b.peer = &homeRunPeer{frames: make(chan any, 32)}
	b.conn = w.loop.Attach(b.key.ID(), b.peer)
	var anyGroup string
	for _, g := range w.groups {
		anyGroup = g
		break
	}
	if err := w.execCmd(b.conn, b.peer, b.key, proto.PushOff{
		HomeCommand: proto.HomeCommand{Type: "push", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(anyGroup)},
		Off:         true,
	}); err != nil {
		return err
	}
	w.loop.Detach(b.conn)
	<-b.conn.Gone()
	return nil
}

func (w *homeWebPushWorld) checkCount(who string, want int) error {
	_, err := w.browsers[who].waitCount(want)
	return err
}

func (w *homeWebPushWorld) checkNoMore(who string) error {
	b := w.browsers[who]
	before := len(b.count())
	time.Sleep(50 * time.Millisecond)
	if after := len(b.count()); after != before {
		return fmt.Errorf("%s received %d further notifications", who, after-before)
	}
	return nil
}

func (w *homeWebPushWorld) checkWireOpaque(a, b, c, d string) error {
	bodies, err := w.browsers["the first browser"].waitCount(1)
	if err != nil {
		return err
	}
	wire := bodies[len(bodies)-1]
	for _, forbidden := range []string{a, b, c, d} {
		if bytes.Contains(wire, []byte(forbidden)) {
			return fmt.Errorf("wire push payload leaked %q", forbidden)
		}
	}
	return nil
}

func (w *homeWebPushWorld) checkDecrypted(groupName, kind, who, title, code, leakOne, leakTwo string) error {
	got := w.browsers["the first browser"].count()
	if len(got) == 0 {
		return fmt.Errorf("no notification received")
	}
	plain, err := homeWebPushDecrypt(w.browsers["the first browser"], got[len(got)-1])
	if err != nil {
		return err
	}
	if err := homeWebPushNoLeaks(plain, title, leakOne, leakTwo); err != nil {
		return err
	}
	var rem proto.Reminder
	if err := proto.Decode(plain, &rem); err != nil {
		return fmt.Errorf("decode reminder %s: %w", plain, err)
	}
	want := proto.Reminder{
		Group:     proto.ID(w.groups[groupName]),
		Msg:       proto.ID(w.lastMsgID),
		GroupName: groupName,
		Kind:      kind,
		Who:       who,
		Title:     title,
		Code:      code,
	}
	if rem != want {
		return fmt.Errorf("got reminder %+v; want %+v", rem, want)
	}
	return nil
}

func homeWebPushNoLeaks(plain []byte, title, leakOne, leakTwo string) error {
	for _, forbidden := range []string{leakOne, leakTwo} {
		if forbidden != "" && bytes.Contains(plain, []byte(forbidden)) {
			return fmt.Errorf("decrypted reminder leaked %q in %s", forbidden, plain)
		}
	}
	if title == "" && bytes.Contains(plain, []byte(`"title"`)) {
		return fmt.Errorf("decrypted reminder included empty title: %s", plain)
	}
	return nil
}

func homeWebPushDecrypt(b *homeWebPushBrowser, wire []byte) ([]byte, error) {
	if len(wire) < 21 {
		return nil, fmt.Errorf("push body too short: %d", len(wire))
	}
	rs := binary.BigEndian.Uint32(wire[16:20])
	idLen := int(wire[20])
	if rs != 4096 || len(wire) < 21+idLen+16 {
		return nil, fmt.Errorf("push header invalid: rs=%d len=%d", rs, len(wire))
	}
	return homeWebPushOpen(b, wire[:16], wire[21:21+idLen], wire[21+idLen:])
}

func homeWebPushAEAD(b *homeWebPushBrowser, salt, ephem []byte) (cipher.AEAD, []byte, error) {
	peerPub, err := ecdh.P256().NewPublicKey(ephem)
	if err != nil {
		return nil, nil, err
	}
	secret, err := b.uaPriv.ECDH(peerPub)
	if err != nil {
		return nil, nil, err
	}
	ctxInfo := append(append([]byte("WebPush: info\x00"), b.uaPriv.PublicKey().Bytes()...), ephem...)
	prk, err1 := hkdf.Key(sha256.New, secret, b.auth, string(ctxInfo), 32)
	keyBytes, err2 := hkdf.Key(sha256.New, prk, salt, "Content-Encoding: aes128gcm\x00", 16)
	ivBytes, err3 := hkdf.Key(sha256.New, prk, salt, "Content-Encoding: nonce\x00", 12)
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, nil, err
	}
	ciph, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(ciph)
	return aead, ivBytes, err
}

func homeWebPushOpen(b *homeWebPushBrowser, salt, ephem, cipherBytes []byte) ([]byte, error) {
	aead, ivBytes, err := homeWebPushAEAD(b, salt, ephem)
	if err != nil {
		return nil, err
	}
	raw, err := aead.Open(nil, ivBytes, cipherBytes, nil)
	if err != nil {
		return nil, err
	}
	cut := bytes.TrimRight(raw, "\x00")
	if len(cut) == 0 || cut[len(cut)-1] != 0x02 {
		return nil, fmt.Errorf("missing RFC 8188 delimiter")
	}
	return cut[:len(cut)-1], nil
}
