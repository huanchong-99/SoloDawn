package home

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The two task transcripts of planning/flows/trace-task.md are what this home
// answers, pushes and delivers for their signed commands. With -update the
// test writes them; without, it fails when they differ from what the home
// produces now.
var traceUpdate = flag.Bool("update", false, "write ../proto/testdata/trace-*.json from the home")

// traceTS is the start of the transcripts, in Unix milliseconds.
const traceTS int64 = 1763600000000

const (
	traceCommit      = "9f2c1abf43f3224eb6826ef971a943dbab4795ce"
	traceMergeCommit = "c1d9e7a2f0b34455aa8f0e6d7c2b1a9988776655"
	traceTitle       = "Node: report path changes"
	tracePR          = 31
	tracePRURL       = "https://git.example/team/website/pull/31"
)

func traceID(label string) string {
	h := sha256.Sum256([]byte(label))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

func traceKey(label string) identity.Key {
	seed := sha256.Sum256([]byte("SoloDawn protocol example: " + label))
	return identity.Key(ed25519.NewKeyFromSeed(seed[:]))
}

// traceRun is one transcript as it is produced: the cast, the home's state
// and store, and what has been recorded so far.
type traceRun struct {
	t          *testing.T
	remote     bool
	ob, ha, hb identity.Key
	reviewer   identity.Key // the node that hosts mY
	g          string
	mO, mS     string
	mX, mY     string
	task       proto.TaskRow // as the last step left it
	db         *store.Store
	run        *runGroup
	tr         proto.TaskTrace
}

func newTraceRun(t *testing.T, remote bool) *traceRun {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &traceRun{t: t, remote: remote, ob: traceKey("trace browser"), ha: traceKey("trace host a"), hb: traceKey("trace host b"),
		g: traceID("trace group"), mS: traceID("trace secretary"), mX: traceID("trace ai x"), mY: traceID("trace ai y"), db: db}
	r.mO, r.reviewer = grpOwnerID(proto.ID(r.g)), r.ha
	cast := []identity.Key{r.ob, r.ha}
	names := []string{"OB", "HA"}
	if remote {
		r.reviewer, cast, names = r.hb, append(cast, r.hb), append(names, "HB")
	}
	r.start(cast, names)
	return r
}

// start makes the transcript's beginning and the home's state and store from it.
func (r *traceRun) start(cast []identity.Key, names []string) {
	kinds, devNames := []string{"browser", "node", "node"}, []string{"phone browser", "workstation", "laptop"}
	r.tr.Devlog = proto.Devlog{Type: "devlog", User: r.ob.ID()}
	for i, k := range cast {
		r.tr.Keys = append(r.tr.Keys, proto.TraceKey{Name: names[i], Device: k.ID()})
		entry := proto.DeviceEntry{
			PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: proto.ID(traceID(fmt.Sprintf("trace dev %d", i+1))), Timestamp: traceTS - 70000 + int64(i)*1000},
			DeviceChange:  proto.DeviceChange{User: r.ob.ID(), Version: i + 1, Op: "add", Device: k.ID(), Kind: kinds[i], Name: devNames[i]},
		}
		r.tr.Devlog.Entries = append(r.tr.Devlog.Entries, r.ob.Sign(r.json(entry)))
	}
	r.tr.Group, r.tr.Owner, r.tr.AIs = r.groupRow(), r.ownerRow(), r.aiRows()
	r.tr.Seq, r.tr.Num, r.tr.Steps = 800, 11, []proto.TraceStep{}

	log, err := identity.Replay(r.ob.ID(), r.tr.Devlog.Entries)
	if err != nil {
		r.t.Fatal(err)
	}
	s := NewState(r.g, r.ha.ID(), r.ob.ID())
	s.Logs[r.ob.ID()], s.Group, s.People[r.mO], s.Seq, s.Num = log, r.tr.Group, r.tr.Owner, r.tr.Seq, r.tr.Num
	for _, ai := range r.tr.AIs {
		s.AIs[ai.ID] = ai
	}
	r.run = &runGroup{loop: &Loop{cfg: LoopConfig{Store: r.db}}, state: s}
	r.write(func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO grp(g, seq, num) VALUES (?, ?, ?)", r.g, r.tr.Seq, r.tr.Num)
		return err
	})
}

func (r *traceRun) groupRow() proto.GroupRow {
	repo, forge := "/srv/repos/website", "none"
	if r.remote {
		repo, forge = "https://git.example/team/website.git", "github"
	}
	return proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: r.g, Seq: 796}, Name: "Website", Kind: "project",
		Repo: repo, Base: "main", Forge: forge, Home: r.ha.ID(), Secretary: r.mS,
		Checks: []string{"go vet ./...", "go test ./..."}, Pass: 90, PassFoundation: 70,
		Limits:  proto.Limits{Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3, ReviewRounds: 5, AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30},
		Created: traceTS - 60000,
	}
}

func (r *traceRun) ownerRow() proto.HumanMemberRow {
	return proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: r.mO, Seq: 797}, Group: r.g, Kind: "human", User: r.ob.ID(), Name: "owner", LastSeen: traceTS - 10000},
		Role:      "owner", Online: true,
	}
}

func (r *traceRun) aiRows() []proto.AIMemberRow {
	ai := func(id, name, cli string, host identity.Key, hostName, path string, secretary bool, seq int64) proto.AIMemberRow {
		return proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: seq}, Group: r.g, Kind: "ai", User: r.ob.ID(), Name: name, LastSeen: traceTS - 10000},
			CLI:       cli, Host: host.ID(), HostName: hostName, Secretary: secretary, Account: "personal",
			Profile: proto.Profile{Tier: "standard"},
			Allow:   []string{}, Approvers: []string{r.ob.ID()}, State: "waiting", Since: traceTS - 10000, Path: path, Usage: proto.Usage{},
		}
	}
	revName, revPath := "workstation", "local"
	if r.remote {
		revName, revPath = "laptop", "direct"
	}
	return []proto.AIMemberRow{
		ai(r.mS, "Secretary", "claude", r.ha, "workstation", "local", true, 798),
		ai(r.mX, "Claude", "claude", r.ha, "workstation", "local", false, 799),
		ai(r.mY, "Codex", "codex", r.reviewer, revName, revPath, false, 800),
	}
}

func (r *traceRun) json(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *traceRun) write(run func(tx *store.Tx) error) {
	if err := r.db.Write(run); err != nil {
		r.t.Fatal(err)
	}
}

// send signs a command and gives it to the home at its own ts, as the
// goroutine of a group does: Apply in a transaction that writes the result,
// then Commit. It records the answer and the rows, then delivers the queued
// items and acknowledges them as their hosts do.
func (r *traceRun) send(by identity.Key, payload any) {
	re := int64(len(r.tr.Steps) + 1)
	env := by.Sign(r.json(payload))
	var hdr proto.PayloadHeader
	json.Unmarshal([]byte(env.Payload), &hdr) // it was just encoded
	now := time.UnixMilli(hdr.Timestamp)
	res, err := r.run.transact(now, func(d Disk, tx *store.Tx) (Result, error) {
		return Commands.Apply(r.run.state, d, tx, env, now)
	})
	if err != nil || res.Answer.Code != "" {
		r.t.Fatalf("command %d: %v %s %s", re, err, res.Answer.Code, res.Msg)
	}
	r.run.state.Commit(res)
	r.run.count(res.Items, hdr.Timestamp)
	for _, row := range res.Rows {
		if task, ok := row.Value().(proto.TaskRow); ok {
			r.task = task
		}
	}
	r.tr.Steps = append(r.tr.Steps, proto.TraceStep{
		From:    by.ID(),
		Cmd:     proto.Cmd{Request: proto.Request{Type: "cmd", ID: re}, Env: env},
		Answer:  res.Frame(re).(proto.CmdResult),
		Rows:    r.run.rowFrames(res.Rows, false)[0].(proto.Rows),
		Inboxes: r.deliver(hdr.Timestamp),
	})
}

// deliver sends every queued item to its host, which acknowledges each.
func (r *traceRun) deliver(now int64) []proto.TraceInbox {
	inboxes := []proto.TraceInbox{}
	r.write(func(tx *store.Tx) error {
		d := &runDisk{tx: tx, group: r.g}
		due, _ := runDue(d.Queued(), now)
		err := d.err
		for _, host := range []string{r.ha.ID(), r.hb.ID()} {
			lot, failed := runLotOf(r.run.state, d, true, nil, host, due)
			err = errors.Join(err, failed)
			for _, frame := range lot.frames {
				inbox, acks := frame.(proto.Inbox), []proto.Ack{}
				for _, entry := range inbox.Items {
					acks = append(acks, proto.Ack{Type: "ack", Group: proto.ID(r.g), Item: entry.Item.Item})
					err = errors.Join(err, tx.Ack(r.g, host, entry.Item.Item))
				}
				inboxes = append(inboxes, proto.TraceInbox{To: host, Inbox: inbox, Acks: acks})
			}
		}
		return err
	})
	return inboxes
}

// cmd is the start of a command of type t, sent at traceTS + after, with the
// nonce that label makes.
func (r *traceRun) cmd(t, label string, after int64, as string) proto.HomeCommand {
	return proto.HomeCommand{Type: t, Nonce: proto.ID(traceID(label)), Timestamp: traceTS + after, Group: proto.ID(r.g), As: proto.ID(as)}
}

func (r *traceRun) taskCmd(op string, after int64) proto.TaskCommand {
	return proto.TaskCommand{HomeCommand: r.cmd("task", "trace "+op, after, ""), Op: op, ID: proto.ID(r.task.ID)}
}

func (r *traceRun) thread() *proto.ID {
	id := proto.ID(r.task.ID)
	return &id
}

// steps sends the commands of trace-task.md, rows 1 to 11.
func (r *traceRun) steps() {
	r.send(r.ob, proto.Post[proto.EmptyBody]{HomeCommand: r.cmd("post", "trace m1", 10000, ""),
		Kind: "text", Text: "Show the connection path and warn when relayed.", Mentions: []proto.ID{}})
	r.send(r.ha, proto.Post[proto.BriefBody]{HomeCommand: r.cmd("post", "trace m2", 20000, r.mS), Kind: "brief", Mentions: []proto.ID{},
		Body: proto.BriefBody{Title: "Show the connection path", Goal: "People can see how a peer is connected.",
			Criteria: []string{"Report every connection path change within 1 s."},
			Scope:    proto.BriefScope{In: []string{"node path events"}, Out: []string{"relay server"}},
			Tasks:    []proto.BriefTask{{Title: traceTitle, Assignee: r.mX, Reviewer: r.mY, Depends: []int{}}}, Pass: 90, Requester: r.mO}})
	r.send(r.ob, proto.Act{HomeCommand: r.cmd("act", "trace c1", 30000, ""), Msg: proto.ID(traceID("trace m2")), Action: "confirm"})
	r.send(r.ha, proto.TaskBranch{TaskCommand: r.taskCmd("branch", 40000), Branch: fmt.Sprintf("task/%d-%s", r.task.Num, r.task.Slug)})
	r.send(r.ha, proto.Post[proto.EmptyBody]{HomeCommand: r.cmd("post", "trace progress", 90000, r.mX), Kind: "progress",
		Thread: r.thread(), Text: "Branch created. Reporting path changes from the peer loop.", Mentions: []proto.ID{}})
	r.send(r.ha, proto.TaskChecked{TaskCommand: r.taskCmd("checked", 140000), OK: true, Commit: traceCommit,
		Items: []proto.CheckItem{{Name: "go vet ./...", OK: true}, {Name: "go test ./...", OK: true, Detail: new("18 tests")}}})
	r.submit()
	r.send(r.reviewer, proto.Post[proto.ReviewSigned]{HomeCommand: r.cmd("post", "trace review", 230000, r.mY), Kind: "review",
		Thread: r.thread(), Text: "Path transitions and round-trip fields are covered cleanly.", Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: proto.ID(r.task.ID), Commit: traceCommit, Score: 93, Items: []proto.ReviewItem{
			{Text: "Covers direct-to-relay and relay-to-direct transitions.", File: new("internal/peer/path.go"), Line: new(42)}}}})
	r.send(r.ha, proto.TaskMerged{TaskCommand: r.taskCmd("merged", 270000), Commit: traceCommit, MergeCommit: traceMergeCommit,
		Stats: proto.DiffStats{Added: 84, Removed: 12}})
	r.send(r.ha, proto.Post[proto.EmptyBody]{HomeCommand: r.cmd("post", "trace summary", 280000, r.mS), Kind: "summary",
		Thread: r.thread(), Main: true, Text: "Path changes are reported with the active transport kind and round-trip time.", Mentions: []proto.ID{}})
}

// submit is rows 7 and 8: the report, the submission and, with a remote, the
// hosted checks.
func (r *traceRun) submit() {
	report := proto.ReportBody{Commit: traceCommit, Files: 3, Added: 84, Removed: 12}
	submitted := proto.TaskSubmitted{TaskCommand: r.taskCmd("submitted", 160000), Commit: traceCommit, Stats: proto.DiffStats{Added: 84, Removed: 12}}
	if r.remote {
		report.PR, submitted.PR, submitted.PRURL = new(tracePR), tracePR, tracePRURL
	}
	r.send(r.ha, proto.Post[proto.ReportBody]{HomeCommand: r.cmd("post", "trace report", 150000, r.mX), Kind: "report",
		Thread: r.thread(), Text: "Path changes are emitted with transport kind and round-trip time.", Mentions: []proto.ID{}, Body: report})
	r.send(r.ha, submitted)
	if r.remote {
		r.send(r.ha, proto.TaskChecked{TaskCommand: r.taskCmd("cloud", 190000), OK: true, Commit: traceCommit,
			Items: []proto.CheckItem{{Name: "ci / test", OK: true}}})
	}
}

// traceGolden writes the transcript under -update, and holds the file to it.
func traceGolden(t *testing.T, name string, tr proto.TaskTrace) {
	t.Helper()
	encoded, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	var pretty bytes.Buffer
	json.Indent(&pretty, encoded, "", "  ") // it was just encoded
	pretty.WriteByte('\n')
	path := filepath.Join("..", "proto", "testdata", name+".json")
	if *traceUpdate {
		if err := os.WriteFile(path, pretty.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, pretty.Bytes()) {
		t.Errorf("%s is not what the home produces; run go test ./internal/home -run TestTaskTraces -update", path)
	}
}

func TestTaskTraces(t *testing.T) {
	for name, remote := range map[string]bool{"trace-remote": true, "trace-local": false} {
		t.Run(name, func(t *testing.T) {
			r := newTraceRun(t, remote)
			r.steps()
			traceGolden(t, name, r.tr)
		})
	}
}
