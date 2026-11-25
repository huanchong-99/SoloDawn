//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_run"] = func(sc *godog.ScenarioContext, _ fixtures) { homeRunSteps(sc) }
}

// homeRunWorld is a home's run loop over a store in memory, with two
// connected devices of its owner. Its commands are its own, because no
// command file exists yet: a group create and a text message.
type homeRunWorld struct {
	now   time.Time
	db    *store.Store
	loop  *home.Loop
	keys  map[string]identity.Key
	peers map[string]*homeRunPeer // by the device as a step names it
	group string
	sent  int // the commands signed so far, for their nonces
}

// homeRunPeer is a connection as the loop sees it: a home.Client that keeps
// the frames it is sent.
type homeRunPeer struct {
	conn    *home.Conn
	frames  chan any
	saw     trail
	re      int64
	welcome string // the answer to its hello
}

func (p *homeRunPeer) Send(frame any) error  { p.frames <- frame; return nil }
func (p *homeRunPeer) Done() <-chan struct{} { return nil }

// next returns the next frame the connection was sent, as a peer connection
// would encode it.
func (p *homeRunPeer) next() (string, error) {
	frame, err := recvFrame(p.frames, 10*time.Second, &p.saw, briefAny)
	if err != nil {
		return "", err
	}
	text, err := json.Marshal(frame)
	return string(text), err
}

const (
	homeRunBrowser = "the owner's browser"
	homeRunLaptop  = "the owner's laptop"
)

func homeRunSteps(sc *godog.ScenarioContext) {
	w := &homeRunWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a home that the owner's browser and the owner's laptop are connected to$`, func() error { return w.open(cleanup) })
	sc.Step(`^(the owner's browser|the owner's laptop) creates the group "([^"]*)"$`, w.create)
	sc.Step(`^the group "([^"]*)", which both connections read$`, w.founded)
	sc.Step(`^(the owner's browser|the owner's laptop) posts "([^"]*)"$`, w.post)
	sc.Step(`^(the owner's browser|the owner's laptop) is answered with success$`, w.answered)
	sc.Step(`^both connections are pushed a list of groups that holds "([^"]*)"$`, w.listed)
	sc.Step(`^both connections are pushed the message "([^"]*)" with the sequence number (\d+)$`, w.pushed)
	sc.Step(`^the home stops and starts again$`, func() error {
		w.loop.Close()
		return w.start(cleanup)
	})
	sc.Step(`^the home lists the group "([^"]*)" to both connections that connect again$`, w.again)
}

// open makes the store and starts the home on it.
func (w *homeRunWorld) open(cleanup func(func())) error {
	w.keys = map[string]identity.Key{homeRunBrowser: exampleKey("browser"), homeRunLaptop: exampleKey("laptop")}
	w.group, w.sent = string(exampleID("group")), 0
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db
	return w.start(cleanup)
}

// start runs the loop on the store and connects both devices.
func (w *homeRunWorld) start(cleanup func(func())) error {
	browser, node, laptop := w.keys[homeRunBrowser], exampleKey("node"), w.keys[homeRunLaptop]
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: node.ID(), Owner: browser.ID(),
		Logs: func(user string) *identity.Log {
			log, _ := homeLog([]identity.Key{browser, node, laptop}) // nil for an error, and for another person
			if log == nil || log.User != user {
				return nil
			}
			return log
		},
		Tables: home.Tables{Commands: home.Table{"group/create": homeRunCreate, "post/text": homeRunPost}},
		Now:    func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop, w.peers = loop, map[string]*homeRunPeer{}
	for name, key := range w.keys {
		p := &homeRunPeer{frames: make(chan any, 64)}
		p.conn = loop.Attach(key.ID(), p)
		loop.Welcome(p.conn, func(groups []proto.GroupSummary) any {
			return proto.Welcome{Type: "welcome", Success: proto.Success{Re: 1, OK: true}, V: 1, Device: node.ID(), Time: w.now.UnixMilli(), Addrs: []string{}, Groups: groups}
		})
		if p.welcome, err = p.next(); err != nil {
			return err
		}
		w.peers[name] = p
	}
	return nil
}

// homeRunCreate writes the group row and the member row of its sender, and
// asks for the groups push.
func homeRunCreate(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Name string `json:"name"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	group := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Kind: "project", Home: s.Home, Checks: []string{}, Created: c.Now}
	owner := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: string(c.Nonce)}, Group: s.ID, Kind: "human", User: c.Who.User, Name: "owner"}, Role: "owner"}
	return home.Result{Rows: []home.Row{home.RowOf(group), home.RowOf(owner)}, GroupsChanged: true}, nil
}

// homeRunPost writes a text message into the main line.
func homeRunPost(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Text string `json:"text"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	msg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, By: c.Who.Member(), Kind: "text",
		Timestamp: c.Now, Text: p.Text, Mentions: []string{}, Body: json.RawMessage(`{}`), Env: &c.Env}
	return home.Result{Rows: []home.Row{home.RowOf(msg)}}, nil
}

// send hands in a command of a device on its own connection.
func (w *homeRunWorld) send(who, t string, more map[string]any) error {
	w.sent++
	payload := map[string]any{"t": t, "n": exampleID(fmt.Sprint("home run ", w.sent)), "ts": w.now.UnixMilli(), "g": w.group}
	for name, value := range more {
		payload[name] = value
	}
	text, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	p := w.peers[who]
	p.re++
	w.loop.Cmd(p.conn, p.re, w.keys[who].Sign(string(text)))
	return nil
}

func (w *homeRunWorld) create(who, name string) error {
	return w.send(who, "group", map[string]any{"op": "create", "name": name})
}

func (w *homeRunWorld) post(who, text string) error {
	return w.send(who, "post", map[string]any{"kind": "text", "text": text})
}

// answered checks that the next frame of a connection is the success of its
// last request.
func (w *homeRunWorld) answered(who string) error {
	p := w.peers[who]
	got, err := p.next()
	if want := fmt.Sprintf(`{"re":%d,"ok":true}`, p.re); err != nil || got != want {
		return fmt.Errorf("%s got %s (%v); want %s", who, got, err, want)
	}
	return nil
}

// listed checks that the next frame of both connections is the groups push
// with that one group.
func (w *homeRunWorld) listed(name string) error {
	want := fmt.Sprintf(`{"t":"groups","groups":[{"g":%q,"name":%q,"seq":2}]}`, w.group, name)
	for who, p := range w.peers {
		if got, err := p.next(); err != nil || got != want {
			return fmt.Errorf("%s got %s (%v); want %s", who, got, err, want)
		}
	}
	return nil
}

// founded creates the group and lets both connections read it from where it
// is, as a sync does.
func (w *homeRunWorld) founded(name string) error {
	if err := w.create(homeRunBrowser, name); err != nil {
		return err
	}
	if err := w.answered(homeRunBrowser); err != nil {
		return err
	}
	if err := w.listed(name); err != nil {
		return err
	}
	return w.read(2)
}

// read subscribes both connections to the group, which is at seq.
func (w *homeRunWorld) read(seq int64) error {
	for who, p := range w.peers {
		p.re++
		w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: p.re}, Group: w.group, Since: seq})
		got, err := p.next()
		if err != nil {
			return err
		}
		var synced proto.Synced
		want := proto.Synced{Type: "synced", Success: proto.Success{Re: p.re, OK: true}, Group: w.group, Seq: seq}
		if err := proto.Decode([]byte(got), &synced); err != nil || synced != want {
			return fmt.Errorf("%s synced %s (%v); want %+v", who, got, err, want)
		}
	}
	return nil
}

// pushed checks that the next frame of both connections is a rows push with
// that one message.
func (w *homeRunWorld) pushed(text string, seq int64) error {
	for who, p := range w.peers {
		got, err := p.next()
		if err != nil {
			return fmt.Errorf("%s: %w", who, err)
		}
		row, err := homeRunReadMessage(got, w.group)
		if err != nil {
			return fmt.Errorf("%s: %w", who, err)
		}
		if have, want := []any{row.Type, row.Text, row.Seq, row.Pos}, []any{"msg", text, seq, seq}; !reflect.DeepEqual(have, want) {
			return fmt.Errorf("%s got the row %v; want %v", who, have, want)
		}
	}
	return nil
}

func homeRunReadMessage(raw, group string) (proto.MsgRow, error) {
	var frame proto.Rows
	var row proto.MsgRow
	if err := json.Unmarshal([]byte(raw), &frame); err != nil || frame.Type != "rows" || frame.Group != group || len(frame.Rows) != 1 {
		return row, fmt.Errorf("got %s (%v); want one row of the group", raw, err)
	}
	err := json.Unmarshal(frame.Rows[0], &row)
	return row, err
}

// again checks what the connections of a home that started again are told
// of its groups in their welcome, and lets both read the group from where
// it is.
func (w *homeRunWorld) again(name string) error {
	for who, p := range w.peers {
		var welcome proto.Welcome
		want := []proto.GroupSummary{{Group: w.group, Name: name, Seq: 3}}
		if err := json.Unmarshal([]byte(p.welcome), &welcome); err != nil || !reflect.DeepEqual(welcome.Groups, want) {
			return fmt.Errorf("%s was welcomed with %s (%v); want the groups %v", who, p.welcome, err, want)
		}
	}
	return w.read(3)
}
