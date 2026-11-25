//go:build acceptance

package features

import (
	"bufio"
	"encoding/json"
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
	ai, card       string
	owner, watcher *s2TerminalPeer
	browser        *peerBrowser
	rtcKey         identity.Key
	bc             *peerBrowserConn
	handBackAt     time.Time
	handBackClose  chan time.Time
}

func s2RegisterTerminal(sc *godog.ScenarioContext, w *s2WatchWorld) {
	t := &s2TerminalWorld{s2WatchWorld: w}
	sc.Step("^Q-team is in a takeover on Q1$", t.takeover)
	sc.Step("^P asks to watch the takeover$", t.ask)
	sc.Step("^Q sees a pending watch confirmation and P cannot start watching$", t.beforeConsent)
	sc.Step("^Q confirms the watch request$", t.confirm)
	sc.Step("^P receives the owner's terminal bytes in order$", t.compareTerminalBytes)
	sc.Step("^P's input and resize do not reach the terminal$", t.receiveOnly)
	sc.Step("^Q hands back the takeover$", t.handBack)
	sc.Step("^P's watch channel closes within 2 seconds$", t.closedOnHandBack)
	sc.Step("^Q has granted P access to watch$", t.grant)
	sc.Step("^P opens a direct watch on Q1$", t.directWatch)
	sc.Step("^the same connection selects the cast TURN relay$", t.waitForRelay)
	sc.Step(`^the watch closes, the original ctl syncs and watch\.start is refused with relay$`, t.refusedOnRelay)
}

func (t *s2TerminalWorld) takeover() error {
	if err := t.start(3 * time.Minute); err != nil {
		return err
	}
	c := t.c
	var err error
	t.group, err = c.CreateProjectGroup(c.Q, c.Q1, "Watch project", "none")
	if err != nil {
		return err
	}
	ai, err := c.JoinAI(c.Q, c.Q1, c.Q1, t.group, s2QTeamCLI, s2QTeamName, false)
	if err != nil {
		return err
	}
	t.ai = ai.ID
	if err := t.joinWatcher(); err != nil {
		return err
	}
	return t.openOwnerTerminal()
}

func (t *s2TerminalWorld) joinWatcher() error {
	c := t.c
	t.rtcKey = newRandomPeerKey()
	if err := c.EnrollBrowser(c.P, t.rtcKey, "Watch terminal"); err != nil {
		return err
	}
	t.browser = c.P.Browser
	conn := c.Q.Conns[c.Q1.ID()].conn
	invite := proto.HomeCommand{Type: "invite", Nonce: c.NextID(), Timestamp: time.Now().UnixMilli(), Group: t.group}
	res, err := c.SendCmd(c.Q, conn, invite)
	if err != nil {
		return err
	}
	link, err := parseS2JoinLink(res.Link, res.Expires)
	if err != nil {
		return err
	}
	bc, raw, err := joinGroupWithLink(c.Ctx, c.addUndo, t.browser, c.P.Key, c.P.Entries, link, c.P.Name, c.NextID(), time.Now())
	if err != nil {
		return err
	}
	t.bc = bc
	return s2WatchReply(raw, "")
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
	if err := t.owner.wait(t.owner.opened[label]); err != nil {
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
	return s2WatchCmd(t.c, t.c.P, t.bc.conn, fields, "")
}

func s2WatchPost(group proto.ID, kind, text string, body any) map[string]any {
	return map[string]any{"t": "post", "g": group, "kind": kind, "text": text, "main": true,
		"thread": nil, "re": nil, "mentions": []string{}, "body": body}
}

func (t *s2TerminalWorld) findCard() error {
	return t.c.readGroupRowsUntil(t.c.Q.Conns[t.c.Q1.ID()], t.group, t.matchCard)
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
	raw, err := t.bc.request(t.c.Ctx, func(id int64) any {
		return map[string]any{"t": "watch.start", "id": id, "g": t.group, "member": t.ai}
	})
	if err != nil {
		return err
	}
	return s2WatchReply(raw, "forbidden")
}

func (t *s2TerminalWorld) confirm() error {
	fields := map[string]any{"t": "act", "g": t.group, "msg": t.card, "action": "confirm"}
	if err := s2WatchCmd(t.c, t.c.Q, t.c.Q.Conns[t.c.Q1.ID()].conn, fields, ""); err != nil {
		return err
	}
	return t.waitGrant()
}

func (t *s2TerminalWorld) waitGrant() error {
	if err := t.owner.send(map[string]any{"t": "host.info", "id": 1 << 30}); err != nil {
		return err
	}
	_, err := t.owner.nextCtl(t.hasWatcher)
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

func (t *s2TerminalWorld) watchStart(want string) error {
	raw, err := t.watcher.request(map[string]any{"t": "watch.start", "g": t.group, "member": t.ai})
	if err != nil {
		return err
	}
	return s2WatchReply(raw, want)
}

func (t *s2TerminalWorld) openWatch() error {
	if err := t.watchStart(""); err != nil {
		return err
	}
	label := t.label("watch")
	if err := t.watcher.channel(label); err != nil {
		return err
	}
	return t.watcher.wait(t.watcher.opened[label])
}

func (t *s2TerminalWorld) compareTerminalBytes() error {
	if err := t.connectWatcher(false); err != nil {
		return err
	}
	if err := t.openWatch(); err != nil {
		return err
	}
	owner, err := t.ownerRoundTrip("watch first\nwatch second\n")
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

func compareS2TerminalBytes(owner, watcher string) error {
	const first = "91x27 watch first"
	const second = "91x27 watch second"
	if owner != watcher || !strings.Contains(owner, first+"\r\n"+second+"\r\n"+s2WatchStop+"\r\n") {
		return fmt.Errorf("owner terminal %q differs from watch %q or lacks ordered output", owner, watcher)
	}
	return nil
}

func (t *s2TerminalWorld) receiveOnly() error {
	if err := t.watcher.send(proto.PTYResize{Type: "pty.resize", Group: t.group, AI: proto.ID(t.ai), Cols: 44, Rows: 12}); err != nil {
		return err
	}
	if err := t.watcherSync(); err != nil {
		return err
	}
	label := t.label("watch")
	if err := t.watcher.channels[label].Send([]byte(s2WatchInput + "\n")); err != nil {
		return err
	}
	if err := t.watcher.wait(t.watcher.closed[label]); err != nil {
		return err
	}
	return t.checkOwnerTerminal()
}

func (t *s2TerminalWorld) checkOwnerTerminal() error {
	out, err := t.ownerRoundTrip("watch probe\nwatch second\n")
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
		if t.watcher.wait(t.watcher.closed[t.label("watch")]) == nil {
			t.handBackClose <- time.Now()
		}
	}()
	t.handBackAt = time.Now()
	return t.owner.command(t.hostCommand("takeover.end"), "")
}

func (t *s2TerminalWorld) closedOnHandBack() error {
	closed, err := recvCtx(t.c.Ctx, t.handBackClose, "the watch to close on hand-back", nil)
	if err != nil {
		return err
	}
	if elapsed := closed.Sub(t.handBackAt); elapsed > 2*time.Second {
		return fmt.Errorf("watch took %s to close on hand-back", elapsed)
	}
	return nil
}

func (t *s2TerminalWorld) directWatch() error {
	if err := t.connectWatcher(true); err != nil {
		return err
	}
	return t.openWatch()
}

func (t *s2TerminalWorld) waitForRelay() error {
	return waitUntilCtx(t.c.Ctx, 100*time.Millisecond, "the watch connection to move to TURN", func() error {
		return t.watcher.checkPath(true)
	})
}

func (t *s2TerminalWorld) refusedOnRelay() error {
	if err := t.watcher.wait(t.watcher.closed[t.label("watch")]); err != nil {
		return err
	}
	if err := t.watcherSync(); err != nil {
		return err
	}
	return t.watchStart("relay")
}

func (t *s2TerminalWorld) watcherSync() error {
	raw, err := t.watcher.request(map[string]any{"t": "sync", "g": t.group, "since": 0, "state_only": true})
	if err != nil {
		return err
	}
	return proto.Decode(raw, new(proto.Synced))
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
		cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
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
