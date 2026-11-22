//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type bulletinHomeBridge struct {
	w        *homeGroupWorld
	done     chan struct{}
	requests []proto.Envelope
}

func (b *bulletinHomeBridge) Send(any) error               { return nil }
func (b *bulletinHomeBridge) Done() <-chan struct{}        { return b.done }
func (b *bulletinHomeBridge) Path() string                 { return "local" }
func (b *bulletinHomeBridge) PathChanged() <-chan struct{} { return nil }
func (b *bulletinHomeBridge) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	cmd := build(1).(proto.Cmd)
	b.requests = append(b.requests, cmd.Env)
	res, err := home.Commands.Apply(b.w.state, b.w.store, b.w.store, cmd.Env, b.w.now)
	if err != nil {
		return nil, err
	}
	b.w.last = res
	b.w.state.Commit(res)
	return json.Marshal(res.Frame(1))
}

func init() { stepSets["host_bulletin"] = hostBulletinSteps }

func hostBulletinSteps(sc *godog.ScenarioContext, _ fixtures) {
	cleanup := afterScenario(sc)
	w := &homeGroupWorld{now: time.UnixMilli(1763600000000)}
	var h *host.Host
	var bridge *bulletinHomeBridge
	var caller host.Caller
	var callErr error
	var sourceID proto.ID
	call := func(group, text string, mentions []string) {
		raw, _ := json.Marshal(map[string]any{"group": group, "text": text, "mentions": mentions})
		_, callErr = host.Tools["post"].Call(context.Background(), h, caller, raw)
	}
	sc.Step(`^a secretary with an existing connection to a known team home$`, func() error {
		if err := initTeamWorld(w, "Team"); err != nil {
			return err
		}
		db, err := store.Open(":memory:")
		if err != nil {
			return err
		}
		cleanup(func() { db.Close() })
		h, _, err = host.New(db, w.laptop, func() time.Time { return w.now })
		if err != nil {
			return err
		}
		cleanup(h.Settle)
		bridge = &bulletinHomeBridge{w: w, done: make(chan struct{})}
		h.Attach(w.state.Home, bridge)
		if err := db.Write(func(tx *store.Tx) error { return store.Put(tx, w.state.ID, w.now.UnixMilli(), w.state.Group) }); err != nil {
			return err
		}
		put := func(group, typ, id string, row any) { raw, _ := json.Marshal(row); h.Rows.Put(group, typ, id, raw) }
		put(w.state.ID, "group", w.state.ID, w.state.Group)
		for id, p := range w.state.People {
			put(w.state.ID, "member", id, p)
		}
		sourceID = exampleID("bulletin source")
		source := w.state.Group
		source.ID, source.Kind, source.Name = string(sourceID), "project", "Website"
		put(source.ID, "group", source.ID, source)
		caller = host.Caller{Session: host.Session{SessionKey: host.SessionKey{Group: source.ID, Member: string(exampleID("bulletin secretary"))}, Kind: host.GroupSession, Secretary: true, Name: "Secretary"}}
		return nil
	})
	sc.Step(`^the team secretary calls post with text "([^"]*)" and mentions its owner$`, func(text string) { call(w.state.ID, text, []string{"owner"}) })
	sc.Step(`^the team home stores a signed person bulletin naming the project and secretary$`, func() error {
		if callErr != nil {
			return callErr
		}
		msg := w.last.Rows[0].Value().(proto.MsgRow)
		var body proto.BulletinBody
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			return err
		}
		known := func(device string) bool { return device == h.Key.ID() }
		if _, err := identity.OpenCommand(*msg.Env, "", known, w.now); err != nil {
			return err
		}
		if msg.Kind != "bulletin" || body.From != string(sourceID) || body.Group != "Website" || body.AI != "Secretary" || w.state.People[msg.By].Role != "owner" || strings.Contains(msg.Env.Payload, `"as"`) {
			return fmt.Errorf("unexpected bulletin %+v, body %+v", msg, body)
		}
		return nil
	})
	sc.Step(`^the team home queues no inbox item$`, func() error {
		if len(w.last.Items) != 0 {
			return fmt.Errorf("team queued items: %+v", w.last.Items)
		}
		return nil
	})
	sc.Step(`^the bulletin caller is "([^"]*)" and its destination is "([^"]*)"$`, func(who, destination string) {
		caller.Secretary = who == "secretary"
		group, text := w.state.ID, "Ready"
		switch destination {
		case "unknown":
			group = string(exampleID("unknown bulletin"))
		case "project":
			row := w.state.Group
			row.Kind = "project"
			raw, _ := json.Marshal(row)
			h.Rows.Put(row.ID, "group", row.ID, raw)
		case "disconnected":
			close(bridge.done)
		case "secret":
			text = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
		}
		call(group, text, []string{})
	})
	sc.Step(`^the bulletin tool refuses with "([^"]*)" without posting$`, func(code string) error {
		if e, ok := callErr.(*host.Error); !ok || e.Code != code {
			return fmt.Errorf("got %v, want %s", callErr, code)
		}
		if len(bridge.requests) != 0 {
			return fmt.Errorf("sent %d commands", len(bridge.requests))
		}
		return nil
	})
}
