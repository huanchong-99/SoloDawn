//go:build acceptance

package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The invitation-code scenarios of the step 2 cast (A9.47 to A9.52): a real rendezvous, the real
// nodes P1, P2 and Q1 as processes, and the browsers of P and Q as peers. node_pair.feature proves
// the node's side of a code in process; these prove what only the process can: that the code and
// its room survive a restart, that nothing the home writes holds the code, and that the person who
// typed it brings their own computer into the group.

// s2MadeCode is a code that `code new` made: the command, its answer and the secret.
type s2MadeCode struct {
	cmd    proto.Code
	res    proto.CmdResult
	secret identity.Secret
}

type s2Typed struct {
	card proto.Pair
	err  error
}

// s2CodeState is what a scenario keeps of its codes: the one open and the one before it, the
// answer to a repeated command, the person who typed while P1 was off and the one who waits.
type s2CodeState struct {
	open, ended s2MadeCode
	again       proto.CmdResult
	typed       chan s2Typed
	waiting     *peerBrowserConn
	rows        []any
}

// MakeCode sends `code new` to the home and keeps what it answers.
func (c *s2Cast) MakeCode(p *s2Person, home *s2Node, g proto.ID, hours int) (s2MadeCode, error) {
	cmd := proto.Code{
		HomeCommand: proto.HomeCommand{Type: "code", Nonce: c.NextID(), Timestamp: time.Now().UnixMilli(), Group: g},
		Op:          "new", Hours: &hours,
	}
	res, err := c.SendCmd(p, p.Conns[home.ID()].conn, cmd)
	if err != nil {
		return s2MadeCode{}, err
	}
	secret, err := identity.ParseSecret(res.Code)
	return s2MadeCode{cmd, res, secret}, err
}

// TypeCode is what the web client does with a code (joinGroup in web/src/lib/join.ts): it joins
// the room of the secret and goes on from the card of the room's host.
func (c *s2Cast) TypeCode(p *s2Person, secret identity.Secret, name string) (s2JoinLink, error) {
	card, err := p.Browser.exchangeRoom(c.Ctx, secret, "b", "a")
	if err != nil {
		return s2JoinLink{}, err
	}
	return c.JoinFromCard(p, secret, card, name)
}

// JoinFromCard takes the home from the card and the group from the welcome, and sends join. The
// link it returns names both; the answer to join is ok, or an error.
func (c *s2Cast) JoinFromCard(p *s2Person, secret identity.Secret, card proto.Pair, name string) (s2JoinLink, error) {
	bc, err := openS2PeopleCtl(c.Ctx, c.addUndo, p.Browser, card.Key)
	if err != nil {
		return s2JoinLink{}, err
	}
	welcome, err := requestBrowserHello(c.Ctx, bc.conn)
	link := s2JoinLink{Secret: secret, Group: welcome.Join, Home: card.Key}
	if err != nil || welcome.Owner || link.Group == "" {
		return link, fmt.Errorf("welcome of %s = %+v (%v), want a joining one that names the group", card.Key, welcome, err)
	}
	if p.Conns[card.Key] == nil {
		p.Conns[card.Key] = bc
	}
	raw, err := sendJoinOn(c.Ctx, bc, p.Key, p.Entries, link, name, c.NextID(), time.Now())
	if err != nil {
		return link, err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return link, fmt.Errorf("join answered %s: %w", raw, err)
	}
	return link, nil
}

// Count reads one number from the database of a node, which keeps running meanwhile.
func (c *s2Cast) Count(n *s2Node, query string, args ...any) (int, error) {
	rows, err := readS2SQLiteRows(c.Ctx, s2ExportDB(n), query, args...)
	if err != nil || len(rows) != 1 {
		return 0, fmt.Errorf("%s on %s: %v (%v)", query, n.Name, rows, err)
	}
	count, ok := rows[0][0].(int64)
	if !ok {
		return 0, fmt.Errorf("%s on %s: %v is a %T, not a count", query, n.Name, rows[0][0], rows[0][0])
	}
	return int(count), nil
}

// awaitCodeHost waits until the room of a code on a rendezvous is hosted by a device, or by
// nobody when host is empty.
func awaitCodeHost(ctx context.Context, url string, secret identity.Secret, host string) error {
	return waitUntil(10*time.Second, fmt.Sprintf("host %q of the code's room", host), func() error {
		card, _, err := codeAnswer(ctx, url, secret)
		if err != nil {
			return abort(err)
		}
		if card.Key != host {
			return fmt.Errorf("host %q", card.Key)
		}
		return nil
	})
}

// checkShape says whether the answer to `code new` is a code of 16 characters in four groups
// that ends `hours` from now, as far as the two clocks of one machine agree.
func (m s2MadeCode) checkShape(hours int) error {
	groups := strings.Split(m.res.Code, "-")
	if len(groups) != 4 || len(strings.Join(groups, "")) != 16 || m.res.Code != m.secret.String() {
		return fmt.Errorf("the code %q is not 16 characters in four groups", m.res.Code)
	}
	want := time.Now().Add(time.Duration(hours) * time.Hour)
	if off := time.UnixMilli(m.res.Expires).Sub(want).Abs(); off > 2*time.Minute {
		return fmt.Errorf("the code ends at %d, %s from %d hours on", m.res.Expires, off, hours)
	}
	return nil
}

// noStored says that nothing a node wrote but its open code holds the code, in either of its
// forms: not a row of a group, and not the receipt of a command.
func (c *s2Cast) noStored(n *s2Node, m s2MadeCode) error {
	for _, form := range []string{m.res.Code, strings.ReplaceAll(m.res.Code, "-", "")} {
		for _, query := range []string{
			"SELECT count(*) FROM row WHERE instr(data, ?) > 0",
			"SELECT count(*) FROM receipt WHERE instr(answer, ?) > 0",
		} {
			if held, err := c.Count(n, query, form); err != nil || held != 0 {
				return fmt.Errorf("%s holds the code %q: %d (%v)", query, form, held, err)
			}
		}
	}
	return nil
}

func registerS2PeopleCodeSteps(sc *godog.ScenarioContext, w *s2PeopleWorld) {
	sc.Step(`^the people cast with P's group "([^"]*)" on P1$`, w.initCastGroup)
	sc.Step(`^the people cast where Q joined P's group "([^"]*)" on P1 by an invitation code$`, w.givenCastQByCode)
	sc.Step(`^P makes an invitation code for (\d+) hours? on P1$`, w.pMakesCode)
	sc.Step(`^the code has 16 characters in four groups, ends (\d+) hours on and the rendezvous shows P1 as the host of its room$`, w.checkCode)
	sc.Step(`^Q types the code and joins as "([^"]*)"$`, w.qTypesCode)
	sc.Step(`^P1 holds Q as a member, let in by P$`, w.checkQLetIn)
	sc.Step(`^P sends that code command again$`, w.pSendsAgain)
	sc.Step(`^P1 answers it ok with no code and makes no second code$`, w.checkAgain)
	sc.Step(`^no row and no receipt of P1 holds the code, and the receipt of the command says only that it succeeded$`, w.checkNothingStored)
	sc.Step(`^Q, a member, types the code again$`, w.qTypesAgain)
	sc.Step(`^P1 answers join ok and Q has no second member row$`, w.checkQOnce)
	sc.Step(`^P1 is stopped, Q types the code and waits, and nothing answers a wrong code$`, w.stopAndQTypes)
	sc.Step(`^P1 is started again$`, w.p1Restarts)
	sc.Step(`^Q is shown P1 and joins without typing again$`, w.qJoinsAfterWaiting)
	sc.Step(`^the rendezvous gives a wrong code no answer$`, w.wrongCodeUnanswered)
	sc.Step(`^a person who typed the code waits on a connection to P1$`, w.rWaits)
	sc.Step(`^the rendezvous shows P1 as the host of the room of the new code and of no other$`, w.checkReplaced)
	sc.Step(`^the connection that waited on the first code is closed$`, w.checkWaitingClosed)
	sc.Step(`^P asks P1 for the code with show$`, w.pShows)
	sc.Step(`^P1 answers ok with no code, and nothing answers the code that was open$`, w.checkShownNone)
}

func (w *s2PeopleWorld) givenCastQByCode(ctx context.Context, groupName string) error {
	if err := w.initCastGroup(ctx, groupName); err != nil {
		return err
	}
	if err := w.pMakesCode(24); err != nil {
		return err
	}
	return w.qTypesCode("Q")
}

func (w *s2PeopleWorld) pMakesCode(hours int) error {
	made, err := w.cast.MakeCode(w.cast.P, w.cast.P1, w.gid, hours)
	w.code.ended, w.code.open = w.code.open, made
	return err
}

func (w *s2PeopleWorld) checkCode(hours int) error {
	if err := w.code.open.checkShape(hours); err != nil {
		return err
	}
	return awaitCodeHost(w.cast.Ctx, w.cast.RVURL, w.code.open.secret, w.cast.P1.ID())
}

// qTypesCode joins Q by the code and waits until both browsers see Q's row.
func (w *s2PeopleWorld) qTypesCode(name string) error {
	c := w.cast
	link, err := c.TypeCode(c.Q, w.code.open.secret, name)
	if err != nil {
		return err
	}
	w.qHomeBC = c.Q.Conns[link.Home]
	if err := c.followJoin(c.Q, link); err != nil {
		return err
	}
	return w.awaitQJoinedOnBothViews()
}

// checkQLetIn reads P1's database: one member row for Q, role member, and the message in the main
// line that says who let Q in.
func (w *s2PeopleWorld) checkQLetIn() error {
	rows, err := readS2SQLiteRows(w.cast.Ctx, s2ExportDB(w.cast.P1),
		`SELECT m.data->>'$.role', coalesce((SELECT j.data->>'$.body.by' FROM row j
			WHERE j.g = m.g AND j.r = 'msg' AND j.data->>'$.body.member' = m.data->>'$.id'), '')
		FROM row m WHERE m.g = ? AND m.r = 'member' AND m.data->>'$.kind' = 'human' AND m.data->>'$.user' = ? AND m.data->>'$.gone' IS NULL`,
		string(w.gid), w.cast.Q.ID())
	if err != nil || len(rows) != 1 || rows[0][0] != "member" || rows[0][1] != w.pMemID {
		return fmt.Errorf("P1 holds Q as %v (%v), want one member row, let in by %s", rows, err, w.pMemID)
	}
	return nil
}

func (w *s2PeopleWorld) pSendsAgain() error {
	c := w.cast
	again, err := c.SendCmd(c.P, c.P.Conns[c.P1.ID()].conn, w.code.open.cmd)
	w.code.again = again
	return err
}

func (w *s2PeopleWorld) checkAgain() error {
	c := w.cast
	if w.code.again.Code != "" {
		return fmt.Errorf("the repeated command answered the code %q", w.code.again.Code)
	}
	held, err := c.Count(c.P1, "SELECT count(*) FROM setting WHERE key='invites' AND instr(value, ?) > 0", strings.ReplaceAll(w.code.open.res.Code, "-", ""))
	if err != nil || held != 1 {
		return fmt.Errorf("P1 keeps the open code in %d entries (%v), want 1", held, err)
	}
	return nil
}

func (w *s2PeopleWorld) checkNothingStored() error {
	c := w.cast
	receipts, err := c.Count(c.P1, "SELECT count(*) FROM receipt WHERE n = ?", string(w.code.open.cmd.Nonce))
	if err != nil || receipts != 1 {
		return fmt.Errorf("P1 holds %d receipts of the command (%v), want 1", receipts, err)
	}
	return c.noStored(c.P1, w.code.open)
}

// groupRows reads how many rows the group has and the last one's sequence number: a join that
// writes any row at all changes one of them.
func (w *s2PeopleWorld) groupRows() ([]any, error) {
	rows, err := readS2SQLiteRows(w.cast.Ctx, s2ExportDB(w.cast.P1), "SELECT count(*), max(seq) FROM row WHERE g = ?", string(w.gid))
	if err != nil || len(rows) != 1 {
		return nil, fmt.Errorf("the rows of the group: %v (%v)", rows, err)
	}
	return rows[0], nil
}

func (w *s2PeopleWorld) qTypesAgain() error {
	before, err := w.groupRows()
	if err != nil {
		return err
	}
	w.code.rows = before
	_, err = w.cast.TypeCode(w.cast.Q, w.code.open.secret, "Q")
	return err
}

func (w *s2PeopleWorld) checkQOnce() error {
	c := w.cast
	if after, err := w.groupRows(); err != nil || !reflect.DeepEqual(after, w.code.rows) {
		return fmt.Errorf("the group has %v rows and last sequence after the join, %v before (%v)", after, w.code.rows, err)
	}
	rows, err := c.Count(c.P1, "SELECT count(*) FROM row WHERE g = ? AND r = 'member' AND data->>'$.kind' = 'human' AND data->>'$.user' = ?", string(w.gid), c.Q.ID())
	if err != nil || rows != 1 {
		return fmt.Errorf("P1 holds %d member rows of Q (%v), want 1", rows, err)
	}
	logged, err := c.Count(c.P1, "SELECT count(*) FROM device_log WHERE user = ?", c.Q.ID())
	if err != nil || logged != len(c.Q.Entries) {
		return fmt.Errorf("P1 holds %d entries of Q's device log (%v), want %d", logged, err, len(c.Q.Entries))
	}
	return nil
}

func (w *s2PeopleWorld) wrongCodeUnanswered() error {
	if card, ok, err := codeAnswer(w.cast.Ctx, w.cast.RVURL, identity.NewSecret()); err != nil || ok {
		return fmt.Errorf("a wrong code was answered by %q (%v)", card.Key, err)
	}
	return nil
}

func (w *s2PeopleWorld) stopAndQTypes() error {
	c := w.cast
	c.P1.Stop()
	if err := codeAwaitOffline(c.Ctx, c.RVURL, c.P1.ID()); err != nil {
		return err
	}
	typed := make(chan s2Typed, 1)
	w.code.typed = typed
	// The join is sent here, so that it is on the way before P1 starts; only the wait for the home
	// runs on its own.
	b, secret := c.Q.Browser, w.code.open.secret
	if err := b.path.Write(c.Ctx, proto.RoomID{Type: "join", Room: secret.Room()}); err != nil {
		return err
	}
	go func() {
		card, err := b.awaitRoomPeer(c.Ctx, secret, secret.Room(), "a", b.pairSelf(secret, "b"))
		typed <- s2Typed{card, err}
	}()
	if err := w.wrongCodeUnanswered(); err != nil {
		return err
	}
	if len(typed) != 0 {
		return errors.New("the code was answered while P1 was off")
	}
	return nil
}

func (w *s2PeopleWorld) p1Restarts() error {
	return w.cast.startNode(w.f.t, w.cast.P1)
}

func (w *s2PeopleWorld) qJoinsAfterWaiting() error {
	c := w.cast
	typed, err := recvCtx(c.Ctx, w.code.typed, "the room to answer Q, who typed while P1 was off", nil)
	if err = errors.Join(err, typed.err); err != nil {
		return err
	}
	if typed.card.Key != c.P1.ID() {
		return fmt.Errorf("the room showed %q, want P1", typed.card.Key)
	}
	link, err := c.JoinFromCard(c.Q, w.code.open.secret, typed.card, "Q")
	if err != nil {
		return err
	}
	if err := c.followJoin(c.Q, link); err != nil {
		return err
	}
	return w.checkQLetIn()
}

// rWaits is a person who typed the code and has not joined: a connection to P1 with no member.
func (w *s2PeopleWorld) rWaits() error {
	c := w.cast
	b, err := newPeerBrowser(c.Ctx, c.addUndo, newRandomPeerKey(), c.RVURL, nil)
	if err != nil {
		return err
	}
	card, err := b.exchangeRoom(c.Ctx, w.code.open.secret, "b", "a")
	if err != nil {
		return err
	}
	if w.code.waiting, err = openS2PeopleCtl(c.Ctx, c.addUndo, b, card.Key); err != nil {
		return err
	}
	return verifyJoiningWelcome(c.Ctx, w.code.waiting.conn)
}

func (w *s2PeopleWorld) checkReplaced() error {
	c := w.cast
	if err := awaitCodeHost(c.Ctx, c.RVURL, w.code.ended.secret, ""); err != nil {
		return err
	}
	return awaitCodeHost(c.Ctx, c.RVURL, w.code.open.secret, c.P1.ID())
}

func (w *s2PeopleWorld) checkWaitingClosed() error {
	select {
	case <-w.code.waiting.conn.Done():
		return nil
	case <-time.After(10 * time.Second):
		return new(trail).timedOut(10*time.Second, "the connection that waited on the first code to be closed")
	}
}

func (w *s2PeopleWorld) pShows() error {
	c := w.cast
	show := proto.Code{HomeCommand: proto.HomeCommand{Type: "code", Nonce: c.NextID(), Timestamp: time.Now().UnixMilli(), Group: w.gid}, Op: "show"}
	shown, err := c.SendCmd(c.P, c.P.Conns[c.P1.ID()].conn, show)
	w.code.again = shown
	return err
}

func (w *s2PeopleWorld) checkShownNone() error {
	if w.code.again.Code != "" || w.code.again.Expires != 0 {
		return fmt.Errorf("code show answered %+v, want ok with no field", w.code.again)
	}
	return awaitCodeHost(w.cast.Ctx, w.cast.RVURL, w.code.open.secret, "")
}

// The move of a group (s2_export.feature): the code ends with it, the file holds none, and the new
// home makes a code that a member types.

func registerS2ExportCodeSteps(sc *godog.ScenarioContext, w *s2ExportWorld) {
	sc.Step(`^P has made an invitation code for (\d+) hours on P1$`, w.codeOn(func(c *s2Cast) *s2Node { return c.P1 }))
	sc.Step(`^P1 has ended the code, nothing answers it on the rendezvous and the file holds no invitation code$`, w.checkMovedCode)
	sc.Step(`^P makes an invitation code for (\d+) hours on P2$`, w.codeOn(func(c *s2Cast) *s2Node { return c.P2 }))
	sc.Step(`^Q, a member, types the code and join succeeds without adding a member row$`, w.qTypesCode)
}

func (w *s2ExportWorld) codeOn(home func(*s2Cast) *s2Node) func(int) error {
	return func(hours int) error {
		made, err := w.c.MakeCode(w.c.P, home(w.c), w.g, hours)
		w.code.open = made
		return err
	}
}

func (w *s2ExportWorld) checkMovedCode() error {
	c, plain := w.c, strings.ReplaceAll(w.code.open.res.Code, "-", "")
	const stored = "SELECT count(*) FROM setting WHERE key='invites' AND instr(value, ?) > 0"
	if held, err := c.Count(c.P1, stored, plain); err != nil || held != 0 {
		return fmt.Errorf("P1 keeps the code after the move: %d (%v)", held, err)
	}
	file, err := os.ReadFile(w.file)
	if err != nil || bytes.Contains(file, []byte(plain)) || bytes.Contains(file, []byte(w.code.open.res.Code)) {
		return fmt.Errorf("the file of the move holds the code (%v)", err)
	}
	return awaitCodeHost(c.Ctx, c.RVURL, w.code.open.secret, "")
}

func (w *s2ExportWorld) qTypesCode() error {
	c := w.c
	link, err := c.TypeCode(c.Q, w.code.open.secret, c.Q.Name)
	if err != nil {
		return err
	}
	if link.Home != c.P2.ID() || link.Group != string(w.g) {
		return fmt.Errorf("the code led to %+v, want group %s on P2", link, w.g)
	}
	w.link = link
	return w.compareRows(s2ExportDB(c.P2))
}
