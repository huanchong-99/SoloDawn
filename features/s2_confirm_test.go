//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["s2_confirm"] = s2ConfirmSteps }

type s2ConfirmWorld struct {
	cast      *s2Cast
	gid       proto.ID
	ids       map[string]string
	ais       map[string]proto.AIMemberRow
	browsers  map[string]*peerBrowserConn
	actors    map[string]*s2Person
	last      json.RawMessage
	lastActor string
}

func s2ConfirmSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2ConfirmWorld{}
	cleanup := afterScenario(sc)
	sc.Step(`^the confirmation cast (with Q|without Q)$`, func(cast string) error {
		return w.setup(f, cleanup, cast == "with Q")
	})
	sc.Step(`^P has another AI "([^"]+)" in the confirmation cast$`, w.joinAnother)
	sc.Step(`^Q opens a second confirmation browser$`, w.secondBrowser)
	sc.Step(`^confirmation browser (P|Q|Q2) sends as "([^"]+)":$`, w.command)
	sc.Step(`^(P|Q) asks "([^"]+)" in "([^"]+)" with "([^"]+)" as "([^"]+)" for confirmation$`, w.mention)
	sc.Step(`^(P|Q|Q2) sends confirmation action "([^"]+)" on "([^"]+)"$`, w.act)
	sc.Step(`^confirmation browser (P|Q|Q2) repeats that command$`, w.repeat)
	sc.Step(`^the confirmation command answers "([^"]+)"$`, w.answer)
	s2ConfirmObservationSteps(sc, w)
	s2ConfirmScriptSteps(sc, w)
}

func (w *s2ConfirmWorld) setup(f fixtures, cleanup func(func()), withQ bool) error {
	c, err := startS2Cast(f.t, cleanup, f, s2CastConfig{Timeout: 5 * time.Minute})
	if err != nil {
		return err
	}
	w.cast = c
	// The AIs are on different nodes, so the project has a remote (contracts/rows.md section 1).
	if err := c.SetForgeReplies("watch-open", s2ForgeReply{Stdout: s2ForgeURL + "1\n"}); err != nil {
		return err
	}
	w.gid, err = c.CreateProjectGroup(c.P, c.P1, "Confirmation", "custom")
	if err != nil {
		return err
	}
	w.browsers = map[string]*peerBrowserConn{"P": c.P.Conns[c.P1.ID()]}
	w.actors = map[string]*s2Person{"P": c.P, "Q": c.Q, "Q2": c.Q}
	w.ids = map[string]string{"P": c.P.ID(), "Q": c.Q.ID()}
	w.ais = map[string]proto.AIMemberRow{}
	if err := w.awaitRow("member", "P-member", &godog.DocString{Content: `{"kind":"human","user":"${P}"}`}); err != nil {
		return err
	}
	return w.joinCast(withQ)
}

func (w *s2ConfirmWorld) joinCast(withQ bool) error {
	c := w.cast
	if err := w.joinAI(c.P1, s2SecretaryCLI, s2SecretaryName, true); err != nil {
		return err
	}
	if err := w.joinAI(c.P2, s2PWorkCLI, s2PWorkName, false); err != nil {
		return err
	}
	if !withQ {
		return w.joinLocalReviewer()
	}
	if err := w.admitQ(); err != nil {
		return err
	}
	if err := w.joinAI(c.Q1, s2QPersonalCLI, s2QPersonalName, false); err != nil {
		return err
	}
	return w.joinAI(c.Q1, s2QTeamCLI, s2QTeamName, false)
}

func (w *s2ConfirmWorld) joinLocalReviewer() error { return w.joinCodex(w.cast.P2, "P-review") }

func (w *s2ConfirmWorld) joinAnother(name string) error { return w.joinCodex(w.cast.P1, name) }

func (w *s2ConfirmWorld) joinCodex(n *s2Node, name string) error {
	if err := w.cast.configureNodeAgent(w.cast.P, n, "codex", "team"); err != nil {
		return err
	}
	return w.joinAI(n, "codex", name, false)
}

func (w *s2ConfirmWorld) joinAI(n *s2Node, cli, name string, secretary bool) error {
	ai, err := w.cast.JoinAI(n.Owner, n, w.cast.P1, w.gid, cli, name, secretary)
	if err != nil {
		return fmt.Errorf("join %s: %w", name, err)
	}
	w.ais[name], w.ids[name] = ai, ai.ID
	return nil
}

func (w *s2ConfirmWorld) homeCmd(typ string) proto.HomeCommand {
	return proto.HomeCommand{Type: typ, Group: w.gid, Nonce: w.cast.NextID(), Timestamp: time.Now().UnixMilli()}
}

func (w *s2ConfirmWorld) admitQ() error {
	c := w.cast
	res, err := c.SendCmd(c.P, w.browsers["P"].conn, w.homeCmd("invite"))
	if err != nil {
		return err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	if err != nil {
		return err
	}
	bc, member, err := c.JoinGroup(c.Q, link, c.Q.Name)
	if err != nil {
		return err
	}
	w.browsers["Q"], w.ids["Q-member"] = bc, string(member)
	return nil
}

func (w *s2ConfirmWorld) secondBrowser() error {
	c := w.cast
	b, err := c.AddBrowser(c.Q, "Q-browser-2")
	if err != nil {
		return err
	}
	if err := w.awaitSecondBrowser(b.key.ID()); err != nil {
		return err
	}
	bc, err := b.openCtl(c.Ctx, c.addUndo, c.P1.ID())
	if err != nil {
		return err
	}
	w.browsers["Q2"] = bc
	_, err = requestBrowserHello(c.Ctx, bc.conn)
	return err
}

func (w *s2ConfirmWorld) awaitSecondBrowser(id string) error {
	return w.cast.readGroupRowsUntil(w.browsers["P"], w.gid, func(raw json.RawMessage) (bool, error) {
		var row proto.DeviceRow
		return proto.Decode(raw, &row) == nil && row.Device == id, nil
	})
}

var s2ConfirmRef = regexp.MustCompile(`\$\{([^}]+)\}`)

func (w *s2ConfirmWorld) expand(s string) (string, error) {
	var missing string
	expanded := s2ConfirmRef.ReplaceAllStringFunc(s, func(ref string) string {
		value, ok := w.ids[ref[2:len(ref)-1]]
		if !ok {
			missing = ref
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("unknown confirmation reference %s", missing)
	}
	return expanded, nil
}

func (w *s2ConfirmWorld) command(actor, name string, doc *godog.DocString) error {
	body, err := w.expand(doc.Content)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return err
	}
	return w.send(actor, name, payload)
}

func (w *s2ConfirmWorld) send(actor, name string, payload map[string]any) error {
	cmd := w.homeCmd(payload["t"].(string))
	payload["n"], payload["ts"], payload["g"] = cmd.Nonce, cmd.Timestamp, w.gid
	w.ids[name] = string(cmd.Nonce)
	w.last = mustMarshalNoEscape(payload)
	return w.repeat(actor)
}

func (w *s2ConfirmWorld) mention(actor, ai, place, text, name string) error {
	var thread any
	if place != "main" {
		thread = w.ids[place]
	}
	err := w.send(actor, name, map[string]any{
		"t": "post", "kind": "text", "thread": thread, "main": place == "main",
		"text": text, "mentions": []string{w.ids[ai]}, "re": nil, "body": map[string]any{},
	})
	if err != nil {
		return err
	}
	return w.answer("ok")
}

func (w *s2ConfirmWorld) act(actor, action, card string) error {
	return w.send(actor, "", map[string]any{"t": "act", "action": action, "msg": w.ids[card]})
}

func (w *s2ConfirmWorld) repeat(actor string) error {
	c, bc := w.cast, w.browsers[actor]
	signer := w.actors[actor].Key
	if actor == "Q2" {
		signer = c.Q.Browsers[1].key
	}
	raw, err := sendSignedCmdRaw(c.Ctx, bc.conn, signer, w.last)
	w.lastActor = string(raw)
	return err
}

func (w *s2ConfirmWorld) answer(want string) error {
	if want != "ok" {
		return checkWireError([]byte(w.lastActor), want)
	}
	var ok proto.CmdResult
	if err := proto.Decode([]byte(w.lastActor), &ok); err != nil {
		return fmt.Errorf("the command answered %s: %w", w.lastActor, err)
	}
	return nil
}
