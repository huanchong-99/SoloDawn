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
	deadline := time.Now().Add(3 * time.Second)
	for {
		b.mu.Lock()
		got := append([][]byte(nil), b.received...)
		b.mu.Unlock()
		if len(got) == want {
			if want == 0 {
				time.Sleep(50 * time.Millisecond)
				b.mu.Lock()
				got = append([][]byte(nil), b.received...)
				b.mu.Unlock()
			}
			if len(got) == want {
				return got, nil
			}
		}
		if time.Now().After(deadline) {
			return got, fmt.Errorf("push endpoint got %d notifications; want %d", len(got), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
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
	ownerID   string
	aiID      string
	taskID    string
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
	sc.Step(`^decrypted by the first browser, the reminder is for group "([^"]*)", kind "([^"]*)", who "([^"]*)" and title "([^"]*)", with none of "([^"]*)"(?: or "([^"]*)")?$`, w.checkDecrypted)
	sc.Step(`^the second browser creates the group "([^"]*)" on the same home with "([^"]*)"$`, w.createGroup)
	sc.Step(`^the home restarts and loads its push subscriptions while only the owner's node reconnects$`, func() error {
		return w.restart(cleanup)
	})
	sc.Step(`^"([^"]*)" posts a text message "([^"]*)" mentioning the owner in "([^"]*)"$`, w.postMention)
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

func (w *homeWebPushWorld) tables() home.Tables {
	return home.Tables{
		Commands: home.Table{
			"group/create": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
				var p struct {
					Name string `json:"name"`
					AI   string `json:"ai"`
				}
				if refusal := c.Decode(&p); refusal != nil {
					return home.Result{}, refusal
				}
				grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Kind: "project", Home: s.Home, Checks: []string{}, Created: c.Now}
				own := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.ownerID}, Group: s.ID, Kind: "human", User: c.Who.User, Name: "Owner"}, Role: "owner"}
				ai := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.aiID}, Group: s.ID, Kind: "ai", User: c.Who.User, Name: p.AI}, Host: w.hostKey.ID()}
				task := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: w.taskID}, Group: s.ID, Num: 1, Title: "Task", Slug: "task", State: "active", Step: "local_check", Assignee: w.aiID}
				return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(ai), home.RowOf(task)}, GroupsChanged: true}, nil
			},
			"post/approval": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
				var p proto.Post[proto.ApprovalBody]
				if refusal := c.Decode(&p); refusal != nil {
					return home.Result{}, refusal
				}
				mentions := make([]string, len(p.Mentions))
				for i, id := range p.Mentions {
					mentions[i] = string(id)
				}
				body, _ := json.Marshal(p.Body)
				thread := string(*p.Thread)
				row := proto.MsgRow{
					RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)},
					Group:     s.ID,
					Thread:    &thread,
					Main:      true,
					By:        c.Who.Member(),
					Kind:      "approval",
					Timestamp: c.Timestamp,
					Text:      p.Text,
					Mentions:  mentions,
					Body:      body,
					State:     &proto.CardState{Status: "pending"},
					Env:       &c.Env,
				}
				return home.Result{Rows: []home.Row{home.RowOf(row)}}, nil
			},
			"post/text": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
				var p proto.Post[proto.EmptyBody]
				if refusal := c.Decode(&p); refusal != nil {
					return home.Result{}, refusal
				}
				mentions := make([]string, len(p.Mentions))
				for i, id := range p.Mentions {
					mentions[i] = string(id)
				}
				row := proto.MsgRow{
					RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)},
					Group:     s.ID,
					By:        c.Who.Member(),
					Kind:      "text",
					Timestamp: c.Timestamp,
					Text:      p.Text,
					Mentions:  mentions,
					Body:      json.RawMessage(`{}`),
					Env:       &c.Env,
				}
				return home.Result{Rows: []home.Row{home.RowOf(row)}}, nil
			},
			"push": home.Commands["push"],
		},
		After: home.Rules[home.Hook]{"push": home.After["push"]},
	}
}

func (w *homeWebPushWorld) setup(cleanup func(func()), groupName, aiName string) error {
	mux := http.NewServeMux()
	w.server = httptest.NewServer(mux)
	cleanup(w.server.Close)

	w.homeKey = exampleKey("wpush home")
	w.hostKey = exampleKey("wpush host")
	w.ownerID = string(exampleID("wpush owner member"))
	w.aiID = string(exampleID("wpush ai member"))
	w.taskID = string(exampleID("wpush task"))
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
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db,
		Home:  w.homeKey.ID(),
		Owner: owner,
		Logs: func(user string) *identity.Log {
			if user != owner {
				return nil
			}
			log, _ := w.ownerLog()
			return log
		},
		Tables: w.tables(),
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

func (w *homeWebPushWorld) createWith(who, groupName, aiName string) error {
	gid := string(w.nextID())
	w.groups[groupName] = gid
	b := w.browsers[who]
	return w.execCmd(b.conn, b.peer, b.key, map[string]any{
		"t": "group", "op": "create", "n": gid, "ts": w.now.UnixMilli(), "g": gid,
		"name": groupName, "ai": aiName,
	})
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
	thread := proto.ID(w.taskID)
	post := proto.Post[proto.ApprovalBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     msgID,
			Timestamp: w.now.UnixMilli(),
			Group:     proto.ID(w.groups[groupName]),
			As:        proto.ID(w.aiID),
		},
		Kind:     "approval",
		Thread:   &thread,
		Main:     true,
		Text:     text,
		Mentions: []proto.ID{proto.ID(w.ownerID)},
		Body: proto.ApprovalBody{
			Task:      w.taskID,
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
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     msgID,
			Timestamp: w.now.UnixMilli(),
			Group:     proto.ID(w.groups[groupName]),
			As:        proto.ID(w.aiID),
		},
		Kind:     "text",
		Text:     text,
		Mentions: []proto.ID{proto.ID(w.ownerID)},
		Body:     proto.EmptyBody{},
	}
	return w.execCmd(w.hostConn, w.hostPeer, w.hostKey, post)
}

func (w *homeWebPushWorld) restart(cleanup func(func())) error {
	w.loop.Close()
	if err := w.startLoop(cleanup, false); err != nil {
		return err
	}
	return w.loop.LoadPushSubscriptions()
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
	b.mu.Lock()
	before := len(b.received)
	b.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	b.mu.Lock()
	after := len(b.received)
	b.mu.Unlock()
	if after != before {
		return fmt.Errorf("%s received %d further notifications", who, after-before)
	}
	return nil
}

func (w *homeWebPushWorld) checkWireOpaque(a, b, c, d string) error {
	first := w.browsers["the first browser"]
	bodies, err := first.waitCount(1)
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

func (w *homeWebPushWorld) checkDecrypted(groupName, kind, who, title, leakOne, leakTwo string) error {
	first := w.browsers["the first browser"]
	first.mu.Lock()
	if len(first.received) == 0 {
		first.mu.Unlock()
		return fmt.Errorf("no notification received")
	}
	wire := first.received[len(first.received)-1]
	first.mu.Unlock()

	plain, err := homeWebPushDecrypt(first, wire)
	if err != nil {
		return err
	}
	for _, forbidden := range []string{leakOne, leakTwo} {
		if forbidden != "" && bytes.Contains(plain, []byte(forbidden)) {
			return fmt.Errorf("decrypted reminder leaked %q in %s", forbidden, plain)
		}
	}
	if title == "" && bytes.Contains(plain, []byte(`"title"`)) {
		return fmt.Errorf("decrypted reminder included empty title: %s", plain)
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
	}
	if rem != want {
		return fmt.Errorf("got reminder %+v; want %+v", rem, want)
	}
	return nil
}

func homeWebPushDecrypt(b *homeWebPushBrowser, wire []byte) ([]byte, error) {
	if len(wire) < 21 {
		return nil, fmt.Errorf("push body too short: %d", len(wire))
	}
	salt := wire[:16]
	rs := binary.BigEndian.Uint32(wire[16:20])
	idLen := int(wire[20])
	if rs != 4096 || len(wire) < 21+idLen+16 {
		return nil, fmt.Errorf("push header invalid: rs=%d len=%d", rs, len(wire))
	}
	asPubBytes := wire[21 : 21+idLen]
	ciphertext := wire[21+idLen:]
	asPub, err := ecdh.P256().NewPublicKey(asPubBytes)
	if err != nil {
		return nil, err
	}
	shared, err := b.uaPriv.ECDH(asPub)
	if err != nil {
		return nil, err
	}
	info := append(append([]byte("WebPush: info\x00"), b.uaPriv.PublicKey().Bytes()...), asPubBytes...)
	ikm, err := hkdf.Key(sha256.New, shared, b.auth, string(info), 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	padded, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimRight(padded, "\x00")
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != 0x02 {
		return nil, fmt.Errorf("missing RFC 8188 delimiter")
	}
	return trimmed[:len(trimmed)-1], nil
}
