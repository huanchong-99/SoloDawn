//go:build acceptance

package features

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"golang.org/x/term"
)

const (
	s2WatchReady = "watch terminal ready"
	s2WatchSize  = "91x27"
	s2WatchStop  = "watch record end"
	s2WatchInput = "watcher input must stay out"
)

type s2TerminalWorld struct {
	*s2WatchWorld
	group          proto.ID
	ai, other      string
	card           string
	owner, watcher *s2TerminalPeer
	rtcKey         identity.Key
	handBackAt     time.Time
	handBackClose  chan time.Time
}

func s2RegisterTerminal(sc *godog.ScenarioContext, w *s2WatchWorld) {
	t := &s2TerminalWorld{s2WatchWorld: w}
	sc.Step("^Q-team is in a takeover on Q1$", t.takeover)
	sc.Step("^P asks to watch the takeover$", t.ask)
	sc.Step("^Q sees a pending watch confirmation and Q1 takes no connection of P$", t.beforeConsent)
	sc.Step("^Q confirms the watch request$", t.confirm)
	sc.Step("^Q1 takes P's connection and refuses a watch of Q-personal$", t.connectGranted)
	sc.Step("^P receives the owner's terminal bytes in order$", t.compareTerminalBytes)
	sc.Step("^P's input and resize do not reach the terminal$", t.receiveOnly)
	sc.Step("^Q hands back the takeover$", t.handBack)
	sc.Step("^P's watch channel closes within 2 seconds and Q1 ends P's connection$", t.closedOnHandBack)
	sc.Step("^Q has granted P access to watch$", t.grant)
	sc.Step("^P opens a direct watch on Q1$", t.directWatch)
	sc.Step("^the direct path slows down and is cut, so the same connection selects the cast TURN relay$", t.moveToRelay)
	sc.Step(`^the watch closes and the original ctl answers, refusing watch\.start with relay$`, t.refusedOnRelay)
}

func (t *s2TerminalWorld) takeover() error {
	if err := t.start(3 * time.Minute); err != nil {
		return err
	}
	c := t.c
	var err error
	t.group, err = c.CreateProjectGroup(c.P, c.P1, "Watch project", "none")
	if err != nil {
		return err
	}
	if err := t.admitQ(); err != nil {
		return err
	}
	if err := t.joinQAIs(); err != nil {
		return err
	}
	t.rtcKey = newRandomPeerKey()
	if err := c.EnrollBrowser(c.P, t.rtcKey, "Watch terminal"); err != nil {
		return err
	}
	return t.openOwnerTerminal()
}

func (t *s2TerminalWorld) admitQ() error {
	c := t.c
	invite := proto.HomeCommand{Type: "invite", Nonce: c.NextID(), Timestamp: time.Now().UnixMilli(), Group: t.group}
	res, err := c.SendCmd(c.P, c.P.Conns[c.P1.ID()].conn, invite)
	if err != nil {
		return err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	if err != nil {
		return err
	}
	_, _, err = c.JoinGroup(c.Q, link, c.Q.Name)
	return err
}

func (t *s2TerminalWorld) joinQAIs() error {
	c := t.c
	team, err := c.JoinAI(c.Q, c.Q1, c.P1, t.group, s2QTeamCLI, s2QTeamName, false)
	if err != nil {
		return err
	}
	personal, err := c.JoinAI(c.Q, c.Q1, c.P1, t.group, s2QPersonalCLI, s2QPersonalName, false)
	t.ai, t.other = team.ID, personal.ID
	return err
}

func (t *s2TerminalWorld) openOwnerTerminal() error {
	c := t.c
	key := newRandomPeerKey()
	if err := c.EnrollBrowser(c.Q, key, "Owner terminal"); err != nil {
		return err
	}
	if _, err := requestHostInfo(c.Ctx, c.Q.Conns[c.Q1.ID()].conn); err != nil {
		return err
	}
	return t.attachOwner(key)
}

func (t *s2TerminalWorld) attachOwner(key identity.Key) error {
	c := t.c
	var err error
	t.owner, err = openS2TerminalPeer(c, c.Q, key, c.Q1, false)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s2WatchInteractiveFlag(c.Q1.Root), nil, 0600); err != nil {
		return err
	}
	return t.startOwnerTerminal()
}

func (t *s2TerminalWorld) startOwnerTerminal() error {
	if err := t.owner.command(t.hostCommand("takeover.start"), ""); err != nil {
		return err
	}
	if err := t.owner.send(proto.PTYResize{Type: "pty.resize", Group: t.group, AI: proto.ID(t.ai), Cols: 91, Rows: 27}); err != nil {
		return err
	}
	label := t.label("pty")
	if err := t.owner.channel(label); err != nil {
		return err
	}
	if err := t.owner.waitOpened(label, "the owner's terminal channel to open"); err != nil {
		return err
	}
	_, err := t.owner.terminalBytes(s2WatchReady + "\r\n")
	return err
}

func (t *s2TerminalWorld) label(kind string) string {
	return kind + ":" + string(t.group) + ":" + t.ai
}

func (t *s2TerminalWorld) hostCommand(kind string) map[string]any {
	return map[string]any{"t": kind, "g": t.group, "member": t.ai, "to": t.c.Q1.ID()}
}

func (t *s2TerminalWorld) ask() error {
	body := map[string]any{"what": "watch", "ai": t.ai}
	fields := s2WatchPost(t.group, "confirm", "", body)
	return s2WatchCmd(t.c, t.c.P, t.c.P.Conns[t.c.P1.ID()].conn, fields, "")
}

func s2WatchPost(group proto.ID, kind, text string, body any) map[string]any {
	return map[string]any{"t": "post", "g": group, "kind": kind, "text": text, "main": true,
		"thread": nil, "re": nil, "mentions": []string{}, "body": body}
}

func (t *s2TerminalWorld) findCard() error {
	return t.c.readGroupRowsUntil(t.c.Q.Conns[t.c.P1.ID()], t.group, t.matchCard)
}

func (t *s2TerminalWorld) matchCard(raw json.RawMessage) (bool, error) {
	var row proto.MsgRow
	if proto.Decode(raw, &row) != nil || row.Kind != "confirm" {
		return false, nil
	}
	var body struct {
		What, AI string
		To       []string
	}
	if err := json.Unmarshal(row.Body, &body); err != nil {
		return false, err
	}
	if body.What != "watch" || body.AI != t.ai {
		return false, nil
	}
	t.card = row.ID
	return true, t.checkCard(row, body.To)
}

func (t *s2TerminalWorld) checkCard(row proto.MsgRow, to []string) error {
	if !slices.Equal(to, []string{t.c.Q.ID()}) || row.State == nil || row.State.Status != "pending" {
		return fmt.Errorf("watch card %s: status %+v, recipients %v", row.ID, row.State, to)
	}
	return nil
}

func (t *s2TerminalWorld) beforeConsent() error {
	if err := t.findCard(); err != nil {
		return err
	}
	if err := expectNeverOpens(t.c.Ctx, t.c.P.Browser, t.c.Q1.ID()); err != nil {
		return err
	}
	return t.noWatcher()
}

// noWatcher: an offer made before a grant arrives is dropped either way, so the grant itself is looked for.
func (t *s2TerminalWorld) noWatcher() error {
	raw, err := t.owner.request(map[string]any{"t": "host.info"})
	if err != nil {
		return err
	}
	if err := s2WatchReply(raw, ""); err != nil {
		return err
	}
	if t.hasWatcher(raw) {
		return errors.New("P is a watcher before the consent")
	}
	return nil
}

func (t *s2TerminalWorld) confirm() error {
	fields := map[string]any{"t": "act", "g": t.group, "msg": t.card, "action": "confirm"}
	if err := s2WatchCmd(t.c, t.c.Q, t.c.Q.Conns[t.c.P1.ID()].conn, fields, ""); err != nil {
		return err
	}
	_, err := t.owner.nextCtl("Q1 to push host.info with P as a watcher", t.hasWatcher)
	return err
}

type s2WatcherGroup struct {
	Group    proto.ID `json:"g"`
	Member   string   `json:"member"`
	Watchers []string
}

func (t *s2TerminalWorld) hasWatcher(raw json.RawMessage) bool {
	var info struct {
		Type   string `json:"t"`
		Agents []struct {
			Groups []s2WatcherGroup
		}
	}
	if json.Unmarshal(raw, &info) != nil || info.Type != "host.info" {
		return false
	}
	for _, agent := range info.Agents {
		if slices.ContainsFunc(agent.Groups, t.watchGroupHasP) {
			return true
		}
	}
	return false
}

func (t *s2TerminalWorld) watchGroupHasP(group s2WatcherGroup) bool {
	return group.Group == t.group && group.Member == t.ai && slices.Contains(group.Watchers, t.c.P.ID())
}

func (t *s2TerminalWorld) grant() error {
	if err := t.ask(); err != nil {
		return err
	}
	if err := t.findCard(); err != nil {
		return err
	}
	return t.confirm()
}

func (t *s2TerminalWorld) connectWatcher(withTURN bool) error {
	var err error
	t.watcher, err = openS2TerminalPeer(t.c, t.c.P, t.rtcKey, t.c.Q1, withTURN)
	return err
}

func (t *s2TerminalWorld) connectGranted() error {
	if err := t.connectWatcher(false); err != nil {
		return err
	}
	return t.watchStart(t.other, "forbidden")
}

func (t *s2TerminalWorld) watchStart(ai, want string) error {
	raw, err := t.watcher.request(map[string]any{"t": "watch.start", "g": t.group, "member": ai})
	if err != nil {
		return err
	}
	return s2WatchReply(raw, want)
}

func (t *s2TerminalWorld) openWatch() error {
	if err := t.watchStart(t.ai, ""); err != nil {
		return err
	}
	label := t.label("watch")
	if err := t.watcher.channel(label); err != nil {
		return err
	}
	return t.waitAttached()
}

// waitAttached reads the size that Q1 pushes once the watch is open on its side: the peer's own side opens before that,
// and a watcher gets only the bytes from then on.
func (t *s2TerminalWorld) waitAttached() error {
	var got proto.PTYResize
	raw, err := t.watcher.nextCtl("the size of the watched terminal", func(raw json.RawMessage) bool {
		return proto.Decode(raw, &got) == nil && got.Type == "pty.resize"
	})
	if err != nil {
		return err
	}
	if fmt.Sprintf("%dx%d", got.Cols, got.Rows) != s2WatchSize || got.Group != t.group || got.AI != proto.ID(t.ai) {
		return fmt.Errorf("size push %s, want %s of %s", raw, s2WatchSize, t.ai)
	}
	return nil
}

func (t *s2TerminalWorld) compareTerminalBytes() error {
	if err := t.openWatch(); err != nil {
		return err
	}
	owner, err := t.ownerRoundTrip("watch first\rwatch second\r")
	if err != nil {
		return err
	}
	watcher, err := t.watcher.terminalBytes(s2WatchStop + "\r\n")
	if err != nil {
		return err
	}
	return compareS2TerminalBytes(owner, watcher)
}

func (t *s2TerminalWorld) ownerRoundTrip(input string) (string, error) {
	dc := t.owner.channels[t.label("pty")]
	if err := dc.Send([]byte(input)); err != nil {
		return "", err
	}
	return t.owner.terminalBytes(s2WatchStop + "\r\n")
}

// compareS2TerminalBytes: the watcher has every byte of the owner from the moment its watch opened, so from the echo of
// the first line, which the owner types after that. What the terminal echoes between the lines, and when, differs between
// systems.
func compareS2TerminalBytes(owner, watcher string) error {
	before, after, first := strings.Cut(watcher, s2WatchSize+" watch first")
	echo := strings.Contains(before, "watch first")
	second := strings.Contains(after, s2WatchSize+" watch second")
	if !strings.HasSuffix(owner, watcher) || !echo || !first || !second {
		return fmt.Errorf("owner terminal %q does not end with watch %q, or the watch lacks ordered output", owner, watcher)
	}
	return nil
}

func (t *s2TerminalWorld) receiveOnly() error {
	if err := t.watcher.send(proto.PTYResize{Type: "pty.resize", Group: t.group, AI: proto.ID(t.ai), Cols: 44, Rows: 12}); err != nil {
		return err
	}
	if err := t.hostInfoRefused(); err != nil {
		return err
	}
	label := t.label("watch")
	if err := t.watcher.channels[label].Send([]byte(s2WatchInput + "\r")); err != nil {
		return err
	}
	if err := t.watcher.waitClosed(label, "Q1 to close the watch that sent input"); err != nil {
		return err
	}
	return t.checkOwnerTerminal()
}

func (t *s2TerminalWorld) checkOwnerTerminal() error {
	out, err := t.ownerRoundTrip("watch probe\rwatch second\r")
	if err != nil {
		return err
	}
	if !strings.Contains(out, s2WatchSize+" watch probe") || strings.Contains(out, s2WatchInput) {
		return fmt.Errorf("owner terminal after watch input and resize %q", out)
	}
	return t.openWatch()
}

func (t *s2TerminalWorld) handBack() error {
	t.handBackClose = make(chan time.Time, 1)
	go func() {
		if t.watcher.waitClosed(t.label("watch"), "the watch to close on hand-back") == nil {
			t.handBackClose <- time.Now()
		}
	}()
	t.handBackAt = time.Now()
	return t.owner.command(t.hostCommand("takeover.end"), "")
}

func (t *s2TerminalWorld) closedOnHandBack() error {
	closed, err := recvCtx(t.c.Ctx, t.handBackClose, "the watch to close on hand-back", &t.watcher.saw)
	if err != nil {
		return err
	}
	if elapsed := closed.Sub(t.handBackAt); elapsed > 2*time.Second {
		return fmt.Errorf("watch took %s to close on hand-back", elapsed)
	}
	return t.watcher.waitClosed("ctl", "Q1 to end P's connection after hand-back")
}

func (t *s2TerminalWorld) directWatch() error {
	if err := t.connectWatcher(true); err != nil {
		return err
	}
	return t.openWatch()
}

func (t *s2TerminalWorld) moveToRelay() error { return t.watcher.moveToRelay() }

func (t *s2TerminalWorld) refusedOnRelay() error {
	if err := t.watcher.waitClosed(t.label("watch"), "Q1 to close the watch on a relayed path"); err != nil {
		return err
	}
	if err := t.hostInfoRefused(); err != nil {
		return err
	}
	return t.watchStart(t.ai, "relay")
}

// hostInfoRefused shows that ctl answers, and that the grant gives P no host surface.
func (t *s2TerminalWorld) hostInfoRefused() error {
	raw, err := t.watcher.request(map[string]any{"t": "host.info"})
	if err != nil {
		return err
	}
	return s2WatchReply(raw, "forbidden")
}

func s2WatchInteractiveFlag(root string) string {
	return root + string(os.PathSeparator) + "watch-terminal"
}

func s2WatchInteractive(args []string) bool {
	if slices.Contains(args, "-p") || os.Getenv("SOLODAWN_S2_NODE") != "Q1" {
		return false
	}
	_, err := os.Stat(s2WatchInteractiveFlag(os.Getenv("HOME")))
	return err == nil
}

func s2RunWatchInteractive() int {
	fmt.Println(s2WatchReady)
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("%dx%d %s\n", cols, rows, in.Text())
		if in.Text() == "watch second" {
			fmt.Println(s2WatchStop)
		}
	}
	return 0
}
