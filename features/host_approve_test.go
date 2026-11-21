//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_approve"] = hostApproveSteps
}

type approveAccHome struct {
	mu      sync.Mutex
	done    chan struct{}
	posts   []proto.Post[proto.ApprovalBody]
	onPost  func(card proto.ID)
	sentAck []proto.Ack
}

func newApproveAccHome() *approveAccHome {
	return &approveAccHome{done: make(chan struct{})}
}

func (f *approveAccHome) Send(frame any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ack, ok := frame.(proto.Ack); ok {
		f.sentAck = append(f.sentAck, ack)
	}
	return nil
}

func (f *approveAccHome) Path() string                 { return "local" }
func (f *approveAccHome) PathChanged() <-chan struct{} { return nil }
func (f *approveAccHome) Done() <-chan struct{}        { return f.done }

func (f *approveAccHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.posts) + 1)
	req := build(id)
	cmd, ok := req.(proto.Cmd)
	var post proto.Post[proto.ApprovalBody]
	if ok && proto.Decode([]byte(cmd.Env.Payload), &post) == nil {
		f.posts = append(f.posts, post)
	}
	hook := f.onPost
	f.mu.Unlock()
	if hook != nil && post.Nonce != "" {
		hook(post.Nonce)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (f *approveAccHome) postCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.posts)
}

func (f *approveAccHome) lastPost() (proto.Post[proto.ApprovalBody], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.posts) == 0 {
		return proto.Post[proto.ApprovalBody]{}, false
	}
	return f.posts[len(f.posts)-1], true
}

type approveAccRunner struct {
	ch chan bool
}

func (r *approveAccRunner) Turn(context.Context, *host.Host, host.SessionKey, string, host.Turn) error {
	return nil
}

func (r *approveAccRunner) Answer(_ *host.Host, _ host.SessionKey, _ string, allow bool) error {
	r.ch <- allow
	return nil
}

func accSeedKey(b byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize)))
}

func accMakeID(b byte) proto.ID {
	return proto.ID(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{b}, 16)))
}

func accMarshal(v any) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

func hostApproveSteps(sc *godog.ScenarioContext, _ fixtures) {
	cleanup := afterScenario(sc)

	owner1 := accSeedKey(11)
	owner2 := accSeedKey(12)
	homeDev := accSeedKey(13)
	hostDev := accSeedKey(14)
	stranger := accSeedKey(15)

	groupID := string(accMakeID(21))
	memberID := string(accMakeID(22))
	taskID := string(accMakeID(23))
	humanID := string(accMakeID(24))

	var (
		db          *store.Store
		h           *host.Host
		fakeHome    *approveAccHome
		runner      *approveAccRunner
		nowMS       int64 = 1_763_600_000_000
		nextItemID  int64 = 500
		sessKey     host.SessionKey
		cliName     string
		waitOutcome chan host.ApprovalResult
		lastResult  host.ApprovalResult
		lateErr     error
	)

	clock := func() time.Time {
		nowMS += 500
		return time.UnixMilli(nowMS)
	}

	seedGroupRows := func(target *host.Host, waitS int) {
		grp := proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: groupID, Seq: 1},
			Name:      "Website",
			Kind:      "project",
			Base:      "main",
			Forge:     "none",
			Home:      homeDev.ID(),
			Checks:    []string{},
			Limits:    proto.Limits{ApprovalWaitS: waitS},
		}
		hum := proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: humanID, Seq: 2}, Group: groupID, Kind: "human", User: owner1.ID(), Name: "Owner"},
			Role:      "owner",
		}
		ai := proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: memberID, Seq: 3}, Group: groupID, Kind: "ai", User: owner1.ID(), Name: "Worker"},
			CLI:       cliName,
			Host:      hostDev.ID(),
			Account:   "personal",
			Allow:     []string{},
			Approvers: []string{owner1.ID()},
			State:     "working",
		}
		target.Rows.Put(groupID, "group", groupID, json.RawMessage(accMarshal(grp)))
		target.Rows.Put(groupID, "member", humanID, json.RawMessage(accMarshal(hum)))
		target.Rows.Put(groupID, "member", memberID, json.RawMessage(accMarshal(ai)))
	}

	seedOwnerLogs := func() error {
		entries := []proto.Envelope{
			owner1.Sign(accMarshal(proto.DeviceEntry{
				PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: accMakeID(31), Timestamp: nowMS},
				DeviceChange:  proto.DeviceChange{User: owner1.ID(), Version: 1, Op: "add", Device: owner1.ID(), Kind: "browser", Name: "first"},
			})),
			owner1.Sign(accMarshal(proto.DeviceEntry{
				PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: accMakeID(32), Timestamp: nowMS},
				DeviceChange:  proto.DeviceChange{User: owner1.ID(), Version: 2, Op: "add", Device: owner2.ID(), Kind: "browser", Name: "second"},
			})),
			owner1.Sign(accMarshal(proto.DeviceEntry{
				PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: accMakeID(33), Timestamp: nowMS},
				DeviceChange:  proto.DeviceChange{User: owner1.ID(), Version: 3, Op: "add", Device: homeDev.ID(), Kind: "node", Name: "home"},
			})),
			owner1.Sign(accMarshal(proto.DeviceEntry{
				PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: accMakeID(34), Timestamp: nowMS},
				DeviceChange:  proto.DeviceChange{User: owner1.ID(), Version: 4, Op: "add", Device: hostDev.ID(), Kind: "node", Name: "host"},
			})),
		}
		return db.Write(func(tx *store.Tx) error {
			if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", accMarshal(owner1.ID())); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO perm(agent, g, level, repo_config) VALUES(?, ?, 2, 0)", cliName, groupID); err != nil {
				return err
			}
			for i, env := range entries {
				if _, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", owner1.ID(), i+1, accMarshal(env)); err != nil {
					return err
				}
			}
			return nil
		})
	}

	buildDecidedRaw := func(signer identity.Key, card proto.ID, action string) json.RawMessage {
		nextItemID++
		actEnv := signer.Sign(accMarshal(proto.Act{
			HomeCommand: proto.HomeCommand{Type: "act", Nonce: accMakeID(byte(40 + nextItemID%50)), Timestamp: nowMS, Group: proto.ID(groupID)},
			Msg:         card,
			Action:      action,
		}))
		itemEnv := homeDev.Sign(accMarshal(proto.Item{
			Type:      "item",
			Nonce:     accMakeID(byte(90 + nextItemID%50)),
			Timestamp: nowMS,
			Group:     proto.ID(groupID),
			To:        hostDev.ID(),
			Item:      nextItemID,
			AI:        proto.ID(memberID),
			Why:       "decided",
			Task:      proto.ID(taskID),
			Msg:       card,
		}))
		return json.RawMessage(accMarshal(proto.InboxItem{Env: itemEnv, Cause: &actEnv}))
	}

	setupHost := func(cli string, waitS int) error {
		var err error
		cliName = cli
		db, err = store.Open(":memory:")
		if err != nil {
			return err
		}
		cleanup(func() { db.Close() })
		if err := seedOwnerLogs(); err != nil {
			return err
		}
		prevRunner := host.Runner
		runner = &approveAccRunner{ch: make(chan bool, 4)}
		host.Runner = runner
		cleanup(func() { host.Runner = prevRunner })

		h, _, err = host.New(db, hostDev, clock)
		if err != nil {
			return err
		}
		fakeHome = newApproveAccHome()
		h.Attach(homeDev.ID(), fakeHome)
		seedGroupRows(h, waitS)
		sessKey = host.SessionKey{Group: groupID, Member: memberID, Task: taskID}
		return h.Sessions.Create(host.Session{
			SessionKey: sessKey,
			Kind:       host.TaskSession,
			Folder:     "wt/1-test",
			Agent:      cli,
			Home:       homeDev.ID(),
			Name:       "Worker",
		})
	}

	startAsyncAsk := func(cli, cmd, why string, level int) error {
		posted := make(chan proto.ID, 1)
		fakeHome.mu.Lock()
		fakeHome.onPost = func(card proto.ID) {
			select {
			case posted <- card:
			default:
			}
		}
		fakeHome.mu.Unlock()

		waitOutcome = make(chan host.ApprovalResult, 1)
		if cli == "claude" {
			s, _ := h.Sessions.Get(sessKey)
			caller := host.Caller{Session: s, Level: level}
			args := accMarshal(map[string]any{
				"tool_name":   "Bash",
				"input":       map[string]string{"command": cmd, "description": why},
				"tool_use_id": "toolu_acc",
			})
			go func() {
				res, _ := host.Tools["approve"].Call(context.Background(), h, caller, json.RawMessage(args))
				waitOutcome <- res.(host.ApprovalResult)
			}()
		} else {
			input := accMarshal(map[string]string{"command": cmd, "cwd": "/repo"})
			h.Report(sessKey, host.Event{
				Kind: host.Asked,
				Ask:  &host.Ask{ID: "ask-1", Tool: "commandExecution", Category: "command", Command: cmd, Why: why, Input: json.RawMessage(input)},
			})
			go func() {
				allow := <-runner.ch
				if allow {
					waitOutcome <- host.ApprovalResult{Behavior: "allow"}
					return
				}
				waitOutcome <- host.ApprovalResult{Behavior: "deny", Message: "Denied by the owner."}
			}()
		}
		select {
		case <-posted:
			return nil
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timed out waiting for approval card post")
		}
	}

	collectResult := func() error {
		select {
		case lastResult = <-waitOutcome:
			return nil
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timed out waiting for approval result")
		}
	}

	sc.Step(`^a paired host with a second browser device and a level 2 "([^"]+)" task session$`, func(cli string) error {
		return setupHost(cli, 60)
	})
	sc.Step(`^a paired host with a second browser device and a level 2 "([^"]+)" task session with a 1-second approval wait$`, func(cli string) error {
		return setupHost(cli, 1)
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)"$`, func(cli, cmd, why string) error {
		if cmd == "export TOKEN=ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD" {
			s, _ := h.Sessions.Get(sessKey)
			args := accMarshal(map[string]any{
				"tool_name":   "Bash",
				"input":       map[string]string{"command": cmd, "description": why},
				"tool_use_id": "toolu_sec",
			})
			res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 2}, json.RawMessage(args))
			if err != nil {
				return err
			}
			lastResult = res.(host.ApprovalResult)
			return nil
		}
		return startAsyncAsk(cli, cmd, why, 2)
	})
	sc.Step(`^the (second browser|unknown) device (approves|denies) the posted approval card while the session waits$`, func(who, verb string) error {
		post, ok := fakeHome.lastPost()
		if !ok {
			return fmt.Errorf("no posted approval card")
		}
		signer := owner2
		if who == "unknown" {
			signer = stranger
		}
		action := "approve"
		if verb == "denies" {
			action = "deny"
		}
		if _, err := h.RecordApprovalDecision(homeDev.ID(), buildDecidedRaw(signer, post.Nonce, action)); err != nil {
			return err
		}
		return collectResult()
	})
	sc.Step(`^an unknown device injects an approval decision while the session waits$`, func() error {
		post, ok := fakeHome.lastPost()
		if !ok {
			return fmt.Errorf("no posted approval card")
		}
		_, err := h.RecordApprovalDecision(homeDev.ID(), buildDecidedRaw(stranger, post.Nonce, "approve"))
		if err == nil {
			return fmt.Errorf("expected unknown device decision to be refused")
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session receives allow$`, func(_ string) error {
		if lastResult.Behavior != "allow" || lastResult.Message != "" {
			return fmt.Errorf("got %+v, want allow", lastResult)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session receives deny "([^"]+)"$`, func(_, want string) error {
		if lastResult.Behavior != "deny" || lastResult.Message != want {
			return fmt.Errorf("got %+v, want deny %q", lastResult, want)
		}
		return nil
	})
	sc.Step(`^the posted card has command "([^"]+)" and why "([^"]+)"$`, func(wantCmd, wantWhy string) error {
		post, ok := fakeHome.lastPost()
		if !ok {
			return fmt.Errorf("no posted card")
		}
		if post.Body.Command != wantCmd || post.Body.Why == nil || *post.Body.Why != wantWhy {
			return fmt.Errorf("got body %+v", post.Body)
		}
		return nil
	})
	sc.Step(`^the approval table holds (\d+) rows$`, func(want int) error {
		var got int
		err := db.Write(func(tx *store.Tx) error {
			return tx.QueryRow("SELECT count(*) FROM approval").Scan(&got)
		})
		if err != nil || got != want {
			return fmt.Errorf("approval count = %d (%v), want %d", got, err, want)
		}
		return nil
	})
	sc.Step(`^the approval table holds 1 row in state "([^"]+)"$`, func(want string) error {
		var state string
		err := db.Write(func(tx *store.Tx) error {
			return tx.QueryRow("SELECT state FROM approval").Scan(&state)
		})
		if err != nil || state != want {
			return fmt.Errorf("approval state = %q (%v), want %q", state, err, want)
		}
		return nil
	})
	sc.Step(`^the host audit records "requested" and "approval" with result "([^"]+)" by the second browser device$`, func(wantResult string) error {
		var reqCount, appCount int
		err := db.Write(func(tx *store.Tx) error {
			if err := tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'requested'").Scan(&reqCount); err != nil {
				return err
			}
			return tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'approval' AND by = ? AND result = ?", owner2.ID(), wantResult).Scan(&appCount)
		})
		if err != nil || reqCount != 1 || appCount != 1 {
			return fmt.Errorf("audit counts requested=%d approval=%d (%v)", reqCount, appCount, err)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)" and no decision arrives$`, func(_, cmd, why string) error {
		s, _ := h.Sessions.Get(sessKey)
		args := accMarshal(map[string]any{
			"tool_name":   "Bash",
			"input":       map[string]string{"command": cmd, "description": why},
			"tool_use_id": "toolu_wait",
		})
		res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 2}, json.RawMessage(args))
		if err != nil {
			return err
		}
		lastResult = res.(host.ApprovalResult)
		return nil
	})
	sc.Step(`^the host restarts on the same store and the second browser device approves the pending card$`, func() error {
		post, ok := fakeHome.lastPost()
		if !ok {
			return fmt.Errorf("no posted card")
		}
		var err error
		h, _, err = host.New(db, hostDev, clock)
		if err != nil {
			return err
		}
		h.Attach(homeDev.ID(), fakeHome)
		seedGroupRows(h, 1)
		action, err := h.RecordApprovalDecision(homeDev.ID(), buildDecidedRaw(owner2, post.Nonce, "approve"))
		if err != nil || action != "approve" {
			return fmt.Errorf("late decision = %q (%v)", action, err)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session retries "([^"]+)" with why "([^"]+)"$`, func(_, cmd, why string) error {
		s, _ := h.Sessions.Get(sessKey)
		args := accMarshal(map[string]any{
			"tool_name":   "Bash",
			"input":       map[string]string{"command": cmd, "description": why},
			"tool_use_id": "toolu_retry",
		})
		res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 2}, json.RawMessage(args))
		if err != nil {
			return err
		}
		lastResult = res.(host.ApprovalResult)
		return nil
	})
	sc.Step(`^the "([^"]+)" session receives allow without posting another card$`, func(_ string) error {
		if lastResult.Behavior != "allow" || fakeHome.postCount() != 1 {
			return fmt.Errorf("result=%+v posts=%d, want allow and 1 post", lastResult, fakeHome.postCount())
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)" and the second browser device approves while waiting$`, func(cli, cmd, why string) error {
		if err := startAsyncAsk(cli, cmd, why, 2); err != nil {
			return err
		}
		post, _ := fakeHome.lastPost()
		_, err := h.RecordApprovalDecision(homeDev.ID(), buildDecidedRaw(owner2, post.Nonce, "approve"))
		if err != nil {
			return err
		}
		return collectResult()
	})
	sc.Step(`^a second approval card was posted and the "([^"]+)" session receives allow$`, func(_ string) error {
		if fakeHome.postCount() != 2 || lastResult.Behavior != "allow" {
			return fmt.Errorf("posts=%d result=%+v, want 2 and allow", fakeHome.postCount(), lastResult)
		}
		return nil
	})
	sc.Step(`^the host audit contains a "refused" row naming the unknown device$`, func() error {
		var n int
		err := db.Write(func(tx *store.Tx) error {
			return tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'refused' AND by = ?", stranger.ID()).Scan(&n)
		})
		if err != nil || n == 0 {
			return fmt.Errorf("refused count = %d (%v)", n, err)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)" under a 1-second wait and no decision arrives$`, func(_, cmd, why string) error {
		seedGroupRows(h, 1)
		s, _ := h.Sessions.Get(sessKey)
		args := accMarshal(map[string]any{
			"tool_name":   "Bash",
			"input":       map[string]string{"command": cmd, "description": why},
			"tool_use_id": "toolu_late_forged",
		})
		res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 2}, json.RawMessage(args))
		if err != nil {
			return err
		}
		lastResult = res.(host.ApprovalResult)
		return nil
	})
	sc.Step(`^an unknown device injects a late approval decision after the wait ended$`, func() error {
		post, ok := fakeHome.lastPost()
		if !ok {
			return fmt.Errorf("no posted card")
		}
		_, lateErr = h.RecordApprovalDecision(homeDev.ID(), buildDecidedRaw(stranger, post.Nonce, "approve"))
		return nil
	})
	sc.Step(`^the late forged decision is refused and the approval table holds 1 row in state "([^"]+)"$`, func(wantState string) error {
		if lateErr == nil {
			return fmt.Errorf("expected late forged decision to fail")
		}
		var state string
		err := db.Write(func(tx *store.Tx) error {
			return tx.QueryRow("SELECT state FROM approval").Scan(&state)
		})
		if err != nil || state != wantState {
			return fmt.Errorf("state = %q (%v), want %q", state, err, wantState)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session asks at level (\d+) to run "([^"]+)"$`, func(_ string, level int, cmd string) error {
		s, _ := h.Sessions.Get(sessKey)
		args := accMarshal(map[string]any{
			"tool_name":   "Bash",
			"input":       map[string]string{"command": cmd},
			"tool_use_id": "toolu_lvl",
		})
		res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: level}, json.RawMessage(args))
		if err != nil {
			return err
		}
		lastResult = res.(host.ApprovalResult)
		return nil
	})
	sc.Step(`^no approval card was posted to the home$`, func() error {
		if fakeHome.postCount() != 0 {
			return fmt.Errorf("posted %d cards, want 0", fakeHome.postCount())
		}
		return nil
	})
}
