//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
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
	mu            sync.Mutex
	done          chan struct{}
	approvalPosts []proto.Post[proto.ApprovalBody]
	decisionPosts []proto.Post[proto.DecisionBody]
	cards         map[string]proto.MsgRow
	onPost        func(card proto.ID)
	sentAck       []proto.Ack
}

func newApproveAccHome() *approveAccHome {
	return &approveAccHome{
		done:  make(chan struct{}),
		cards: map[string]proto.MsgRow{},
	}
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
	id := int64(len(f.approvalPosts) + len(f.decisionPosts) + 1)
	req := build(id)
	if fetch, ok := req.(proto.FetchRow); ok {
		rows := []json.RawMessage{}
		if row, exists := f.cards[fetch.RowID]; exists {
			rows = []json.RawMessage{json.RawMessage(accMarshal(row))}
		}
		f.mu.Unlock()
		resp := proto.Fetched{
			Type:    "fetched",
			Success: proto.Success{Re: id, OK: true},
			Rows:    rows,
			More:    false,
		}
		return json.RawMessage(accMarshal(resp)), nil
	}
	var postedID proto.ID
	if cmd, ok := req.(proto.Cmd); ok {
		var hdr struct {
			Kind string   `json:"kind"`
			ID   proto.ID `json:"n"`
		}
		json.Unmarshal([]byte(cmd.Env.Payload), &hdr)
		postedID = hdr.ID
		if hdr.Kind == "approval" {
			var post proto.Post[proto.ApprovalBody]
			if proto.Decode([]byte(cmd.Env.Payload), &post) == nil {
				f.approvalPosts = append(f.approvalPosts, post)
			}
		} else if hdr.Kind == "decision" {
			var post proto.Post[proto.DecisionBody]
			if proto.Decode([]byte(cmd.Env.Payload), &post) == nil {
				f.decisionPosts = append(f.decisionPosts, post)
			}
		}
	}
	hook := f.onPost
	f.mu.Unlock()
	if hook != nil && postedID != "" {
		hook(postedID)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (f *approveAccHome) totalPostCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.approvalPosts) + len(f.decisionPosts)
}

func (f *approveAccHome) lastApprovalPost() (proto.Post[proto.ApprovalBody], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.approvalPosts) == 0 {
		return proto.Post[proto.ApprovalBody]{}, false
	}
	return f.approvalPosts[len(f.approvalPosts)-1], true
}

func (f *approveAccHome) lastDecisionPost() (proto.Post[proto.DecisionBody], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.decisionPosts) == 0 {
		return proto.Post[proto.DecisionBody]{}, false
	}
	return f.decisionPosts[len(f.decisionPosts)-1], true
}

func (f *approveAccHome) putCard(card proto.MsgRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cards[card.ID] = card
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

func parseColonPairs(a, b string) []map[string]string {
	var out []map[string]string
	for _, raw := range []string{a, b} {
		label, desc, _ := strings.Cut(raw, ":")
		out = append(out, map[string]string{"label": label, "description": desc})
	}
	return out
}

func hostApproveSteps(sc *godog.ScenarioContext, _ fixtures) {
	cleanup := afterScenario(sc)

	owner1 := accSeedKey(11)
	owner2 := accSeedKey(12)
	homeDev := accSeedKey(13)
	hostDev := accSeedKey(14)

	groupID := string(accMakeID(21))
	memberID := string(accMakeID(22))
	taskID := string(accMakeID(23))
	humanID := string(accMakeID(24))

	var (
		db            *store.Store
		h             *host.Host
		fakeHome      *approveAccHome
		runner        *approveAccRunner
		nowMS         int64 = 1_763_600_000_000
		nextItemID    int64 = 500
		sessKey       host.SessionKey
		cliName       string
		waitOutcome   chan host.ApprovalResult
		codexReplyCh  chan map[string][]string
		lastResult    host.ApprovalResult
		lastCodexAns  map[string][]string
		postsAtBefore int
	)

	clock := func() time.Time {
		nowMS += 500
		return time.UnixMilli(nowMS)
	}

	seedGroupRows := func(target *host.Host, waitS int, secretary bool) {
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
			Secretary: secretary,
			Account:   "personal",
			Allow:     []string{},
			Approvers: []string{owner1.ID()},
			State:     "working",
		}
		target.Rows.Put(groupID, "group", groupID, json.RawMessage(accMarshal(grp)))
		target.Rows.Put(groupID, "member", humanID, json.RawMessage(accMarshal(hum)))
		target.Rows.Put(groupID, "member", memberID, json.RawMessage(accMarshal(ai)))
	}

	buildCardRow := func(card proto.ID, kind string, state proto.CardState) proto.MsgRow {
		var thread *string
		if sessKey.Task != "" {
			t := sessKey.Task
			thread = &t
		}
		return proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: string(card), Seq: 10},
			Pos:       10,
			Group:     groupID,
			Thread:    thread,
			Main:      true,
			By:        memberID,
			Kind:      kind,
			Timestamp: nowMS,
			Text:      "",
			Mentions:  []string{},
			Re:        nil,
			Body:      json.RawMessage(`{}`),
			State:     &state,
		}
	}

	buildApprovalDecidedRaw := func(signer identity.Key, card proto.ID, status string) json.RawMessage {
		nextItemID++
		actEnv := signer.Sign(`{"t":"act"}`)
		by := humanID
		at := nowMS
		msgRow := buildCardRow(card, "approval", proto.CardState{
			Status: status,
			By:     &by,
			At:     &at,
			Act:    &actEnv,
		})
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
		return json.RawMessage(accMarshal(proto.InboxItem{
			Env: itemEnv,
			Msg: json.RawMessage(accMarshal(msgRow)),
		}))
	}

	buildDecisionDecidedRaw := func(signer identity.Key, card proto.ID, choice, text *string) json.RawMessage {
		nextItemID++
		actEnv := signer.Sign(`{"t":"act"}`)
		by := humanID
		at := nowMS
		msgRow := buildCardRow(card, "decision", proto.CardState{
			Status: "decided",
			By:     &by,
			At:     &at,
			Choice: choice,
			Text:   text,
			Act:    &actEnv,
		})
		var itemTask proto.ID
		if sessKey.Task != "" {
			itemTask = proto.ID(sessKey.Task)
		}
		itemEnv := homeDev.Sign(accMarshal(proto.Item{
			Type:      "item",
			Nonce:     accMakeID(byte(40 + nextItemID%50)),
			Timestamp: nowMS,
			Group:     proto.ID(groupID),
			To:        hostDev.ID(),
			Item:      nextItemID,
			AI:        proto.ID(memberID),
			Why:       "decided",
			Task:      itemTask,
			Msg:       card,
		}))
		return json.RawMessage(accMarshal(proto.InboxItem{
			Env: itemEnv,
			Msg: json.RawMessage(accMarshal(msgRow)),
		}))
	}

	setupHost := func(cli string, waitS int, kind host.Kind, secretary bool) error {
		var err error
		cliName = cli
		db, err = store.Open(":memory:")
		if err != nil {
			return err
		}
		opened := db
		cleanup(func() { opened.Close() })
		seedOwner := func(tx *store.Tx) error {
			_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", accMarshal(owner1.ID()))
			return err
		}
		if err := db.Write(seedOwner); err != nil {
			return err
		}
		prevRunner := host.Runner
		// Single reader: startAsyncAsk goroutine awaiting Runner.Answer.
		runner = &approveAccRunner{ch: make(chan bool, 4)}
		host.Runner = runner
		cleanup(func() { host.Runner = prevRunner })

		h, _, err = host.New(db, hostDev, clock)
		if err != nil {
			return err
		}
		fakeHome = newApproveAccHome()
		h.Attach(homeDev.ID(), fakeHome)
		seedGroupRows(h, waitS, secretary)
		task := taskID
		folder := "wt/1-test"
		if kind == host.GroupSession {
			task = ""
			folder = "repo"
		}
		sessKey = host.SessionKey{Group: groupID, Member: memberID, Task: task}
		return h.Sessions.Create(host.Session{
			SessionKey: sessKey,
			Kind:       kind,
			Folder:     folder,
			Agent:      cli,
			Home:       homeDev.ID(),
			Name:       "Worker",
			Secretary:  secretary,
		})
	}

	awaitPostedCard := func(posted <-chan proto.ID) error {
		// Single reader: step function waiting for Home.Request card post notification.
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-posted:
			return nil
		case <-timer.C:
			return fmt.Errorf("timed out waiting for card post")
		}
	}

	armPostSignal := func() <-chan proto.ID {
		// Single reader: awaitPostedCard above.
		posted := make(chan proto.ID, 1)
		fakeHome.mu.Lock()
		fakeHome.onPost = func(card proto.ID) {
			select {
			case posted <- card:
			default:
			}
		}
		fakeHome.mu.Unlock()
		return posted
	}

	startAsyncAsk := func(cli, cmd, why string, level int) error {
		posted := armPostSignal()
		// Single reader: collectResult step helper.
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
		return awaitPostedCard(posted)
	}

	collectResult := func() error {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case lastResult = <-waitOutcome:
			return nil
		case <-timer.C:
			return fmt.Errorf("timed out waiting for approval result")
		}
	}

	sc.Step(`^a paired host with a second browser device and a default level 3 "([^"]+)" task session$`, func(cli string) error {
		return setupHost(cli, 60, host.TaskSession, false)
	})
	sc.Step(`^a paired host with a second browser device and a default level 3 "([^"]+)" secretary group session$`, func(cli string) error {
		return setupHost(cli, 60, host.GroupSession, true)
	})
	sc.Step(`^a paired host with a second browser device and a default level 3 "([^"]+)" task session with a 1-second approval wait$`, func(cli string) error {
		return setupHost(cli, 1, host.TaskSession, false)
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)"$`, func(cli, cmd, why string) error {
		if strings.Contains(cmd, "ghp_") {
			s, _ := h.Sessions.Get(sessKey)
			args := accMarshal(map[string]any{
				"tool_name":   "Bash",
				"input":       map[string]string{"command": cmd, "description": why},
				"tool_use_id": "toolu_sec",
			})
			res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 3}, json.RawMessage(args))
			if err != nil {
				return err
			}
			lastResult = res.(host.ApprovalResult)
			return nil
		}
		return startAsyncAsk(cli, cmd, why, 3)
	})
	sc.Step(`^the second browser device "(approves|denies)" the posted approval card while the session waits$`, func(verb string) error {
		post, ok := fakeHome.lastApprovalPost()
		if !ok {
			return fmt.Errorf("no posted approval card")
		}
		status := "approved"
		if verb == "denies" {
			status = "denied"
		}
		if _, err := h.RecordApprovalDecision(homeDev.ID(), buildApprovalDecidedRaw(owner2, post.Nonce, status)); err != nil {
			return err
		}
		return collectResult()
	})
	sc.Step(`^the "([^"]+)" session receives "(allow|deny)"$`, func(_, want string) error {
		if want == "allow" && (lastResult.Behavior != "allow" || lastResult.Message != "" || lastResult.UpdatedInput != nil) {
			return fmt.Errorf("got %+v, want allow", lastResult)
		}
		if want == "deny" && (lastResult.Behavior != "deny" || lastResult.Message != "Denied by the owner.") {
			return fmt.Errorf("got %+v, want owner deny", lastResult)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session receives deny "([^"]+)"$`, func(_, want string) error {
		if lastResult.Behavior != "deny" || lastResult.Message != want {
			return fmt.Errorf("got %+v, want deny %q", lastResult, want)
		}
		return nil
	})
	sc.Step(`^the posted approval card has command "([^"]+)" and why "([^"]+)"$`, func(wantCmd, wantWhy string) error {
		post, ok := fakeHome.lastApprovalPost()
		if !ok {
			return fmt.Errorf("no posted approval card")
		}
		if post.Body.Command != wantCmd || post.Body.Why == nil || *post.Body.Why != wantWhy {
			return fmt.Errorf("got body %+v", post.Body)
		}
		return nil
	})
	sc.Step(`^Claude Code calls AskUserQuestion with header "([^"]*)" and question "([^"]+)" and options "([^"]+)" and "([^"]+)"$`, func(header, question, optA, optB string) error {
		posted := armPostSignal()
		// Single reader: collectResult step helper.
		waitOutcome = make(chan host.ApprovalResult, 1)
		s, _ := h.Sessions.Get(sessKey)
		args := accMarshal(map[string]any{
			"tool_name": "AskUserQuestion",
			"input": map[string]any{
				"questions": []map[string]any{
					{
						"header":      header,
						"question":    question,
						"options":     parseColonPairs(optA, optB),
						"multiSelect": false,
					},
				},
			},
			"tool_use_id": "toolu_ask",
		})
		go func() {
			res, _ := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 3}, json.RawMessage(args))
			waitOutcome <- res.(host.ApprovalResult)
		}()
		return awaitPostedCard(posted)
	})
	sc.Step(`^the Codex session asks requestUserInput id "([^"]+)" header "([^"]*)" question "([^"]+)" with options "([^"]+)" and "([^"]+)"$`, func(qID, header, question, optA, optB string) error {
		posted := armPostSignal()
		// Single reader: step asserting Codex Reply output.
		codexReplyCh = make(chan map[string][]string, 1)
		input := accMarshal(map[string]any{
			"questions": []map[string]any{
				{
					"id":       qID,
					"header":   header,
					"question": question,
					"isSecret": false,
					"options":  parseColonPairs(optA, optB),
				},
			},
		})
		replyFn := func(ans map[string][]string) error {
			codexReplyCh <- ans
			return nil
		}
		h.Report(sessKey, host.Event{
			Kind: host.Asked,
			Ask:  &host.Ask{ID: "req-" + qID, Tool: "requestUserInput", Input: json.RawMessage(input), Reply: replyFn},
		})
		return awaitPostedCard(posted)
	})
	sc.Step(`^the second browser device chooses option "([^"]+)" with note "([^"]*)" on the posted decision card$`, func(optID, note string) error {
		post, ok := fakeHome.lastDecisionPost()
		if !ok {
			return fmt.Errorf("no posted decision card")
		}
		choice := optID
		var textPtr *string
		if note != "" {
			n := note
			textPtr = &n
		}
		waited, err := h.Hand(string(post.Nonce), buildDecisionDecidedRaw(owner2, post.Nonce, &choice, textPtr))
		if !waited || err != nil {
			return fmt.Errorf("Hand waited=%v err=%v", waited, err)
		}
		if cliName == "claude" {
			return collectResult()
		}
		lastCodexAns = <-codexReplyCh
		return nil
	})
	sc.Step(`^the second browser device answers the posted decision card with custom text "([^"]+)"$`, func(custom string) error {
		post, ok := fakeHome.lastDecisionPost()
		if !ok {
			return fmt.Errorf("no posted decision card")
		}
		txt := custom
		waited, err := h.Hand(string(post.Nonce), buildDecisionDecidedRaw(owner2, post.Nonce, nil, &txt))
		if !waited || err != nil {
			return fmt.Errorf("Hand waited=%v err=%v", waited, err)
		}
		if cliName == "claude" {
			return collectResult()
		}
		lastCodexAns = <-codexReplyCh
		return nil
	})
	sc.Step(`^the posted decision card has options "([^"]+)" and "([^"]+)" and context "([^"]+)"$`, func(wantA, wantB, wantCtx string) error {
		post, ok := fakeHome.lastDecisionPost()
		if !ok {
			return fmt.Errorf("no posted decision card")
		}
		if len(post.Body.Options) != 2 || post.Body.Context == nil || *post.Body.Context != wantCtx {
			return fmt.Errorf("unexpected decision body %+v", post.Body)
		}
		gotA := post.Body.Options[0].ID + ":" + post.Body.Options[0].Label
		gotB := post.Body.Options[1].ID + ":" + post.Body.Options[1].Label
		if gotA != wantA || gotB != wantB {
			return fmt.Errorf("got options %q and %q, want %q and %q", gotA, gotB, wantA, wantB)
		}
		return nil
	})
	sc.Step(`^the posted decision card has no task thread and options "([^"]+)" and "([^"]+)"$`, func(wantA, wantB string) error {
		post, ok := fakeHome.lastDecisionPost()
		if !ok {
			return fmt.Errorf("no posted decision card")
		}
		if post.Thread != nil || post.Body.Task != nil || len(post.Body.Options) != 2 {
			return fmt.Errorf("expected nil thread/task and 2 options, got thread=%v body=%+v", post.Thread, post.Body)
		}
		gotA := post.Body.Options[0].ID + ":" + post.Body.Options[0].Label
		gotB := post.Body.Options[1].ID + ":" + post.Body.Options[1].Label
		if gotA != wantA || gotB != wantB {
			return fmt.Errorf("got options %q and %q, want %q and %q", gotA, gotB, wantA, wantB)
		}
		return nil
	})
	sc.Step(`^Claude Code receives allow with answer "([^"]+)" for "([^"]+)" and note "([^"]*)"$`, func(wantAns, question, wantNote string) error {
		if lastResult.Behavior != "allow" || lastResult.UpdatedInput == nil {
			return fmt.Errorf("got %+v, want allow with updatedInput", lastResult)
		}
		ansMap, _ := lastResult.UpdatedInput["answers"].(map[string]string)
		if ansMap[question] != wantAns {
			return fmt.Errorf("answers[%q] = %q, want %q", question, ansMap[question], wantAns)
		}
		ann, hasAnn := lastResult.UpdatedInput["annotations"].(map[string]map[string]string)
		if wantNote == "" && hasAnn {
			return fmt.Errorf("expected no annotations, got %+v", ann)
		}
		if wantNote != "" && (!hasAnn || ann[question]["notes"] != wantNote) {
			return fmt.Errorf("annotations = %+v, want notes=%q", ann, wantNote)
		}
		return nil
	})
	sc.Step(`^Codex Reply receives "([^"]+)" for question "([^"]+)"$`, func(wantVal, qID string) error {
		want := []string{wantVal}
		if !reflect.DeepEqual(lastCodexAns[qID], want) {
			return fmt.Errorf("Codex answers[%q] = %v, want %v", qID, lastCodexAns[qID], want)
		}
		return nil
	})
	sc.Step(`^the approval table holds (\d+) rows$`, func(want int) error {
		var got int
		read := func(tx *store.Tx) error {
			return tx.QueryRow("SELECT count(*) FROM approval").Scan(&got)
		}
		err := db.Write(read)
		if err != nil || got != want {
			return fmt.Errorf("approval count = %d (%v), want %d", got, err, want)
		}
		return nil
	})
	sc.Step(`^the approval table holds 1 row in state "([^"]+)"$`, func(want string) error {
		var state string
		read := func(tx *store.Tx) error {
			return tx.QueryRow("SELECT state FROM approval").Scan(&state)
		}
		err := db.Write(read)
		if err != nil || state != want {
			return fmt.Errorf("approval state = %q (%v), want %q", state, err, want)
		}
		return nil
	})
	sc.Step(`^the host audit records "requested" and "approval" with result "([^"]+)" by the second browser device$`, func(wantResult string) error {
		var reqCount, appCount int
		read := func(tx *store.Tx) error {
			if err := tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'requested'").Scan(&reqCount); err != nil {
				return err
			}
			return tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'approval' AND by = ? AND result = ?", owner2.ID(), wantResult).Scan(&appCount)
		}
		err := db.Write(read)
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
		res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 3}, json.RawMessage(args))
		if err != nil {
			return err
		}
		lastResult = res.(host.ApprovalResult)
		return nil
	})
	sc.Step(`^the host restarts on the same store and the second browser device approves the pending card via RecordApprovalDecision$`, func() error {
		post, ok := fakeHome.lastApprovalPost()
		if !ok {
			return fmt.Errorf("no posted card")
		}
		var err error
		h, _, err = host.New(db, hostDev, clock)
		if err != nil {
			return err
		}
		h.Attach(homeDev.ID(), fakeHome)
		seedGroupRows(h, 1, false)
		status, err := h.RecordApprovalDecision(homeDev.ID(), buildApprovalDecidedRaw(owner2, post.Nonce, "approved"))
		if err != nil || status != "approved" {
			return fmt.Errorf("late decision = %q (%v)", status, err)
		}
		return nil
	})
	sc.Step(`^the home marks the pending card approved by the second browser device without calling RecordApprovalDecision$`, func() error {
		post, ok := fakeHome.lastApprovalPost()
		if !ok {
			return fmt.Errorf("no posted card")
		}
		actEnv := owner2.Sign(`{"t":"act"}`)
		by := humanID
		at := nowMS
		fakeHome.putCard(buildCardRow(post.Nonce, "approval", proto.CardState{
			Status: "approved",
			By:     &by,
			At:     &at,
			Act:    &actEnv,
		}))
		return nil
	})
	sc.Step(`^the "([^"]+)" session retries "([^"]+)" with why "([^"]+)"$`, func(_, cmd, why string) error {
		postsAtBefore = fakeHome.totalPostCount()
		s, _ := h.Sessions.Get(sessKey)
		args := accMarshal(map[string]any{
			"tool_name":   "Bash",
			"input":       map[string]string{"command": cmd, "description": why},
			"tool_use_id": "toolu_retry",
		})
		res, err := host.Tools["approve"].Call(context.Background(), h, host.Caller{Session: s, Level: 3}, json.RawMessage(args))
		if err != nil {
			return err
		}
		lastResult = res.(host.ApprovalResult)
		return nil
	})
	sc.Step(`^the "([^"]+)" session receives allow without posting another card$`, func(_ string) error {
		if lastResult.Behavior != "allow" || fakeHome.totalPostCount() != postsAtBefore {
			return fmt.Errorf("result=%+v posts=%d (was %d)", lastResult, fakeHome.totalPostCount(), postsAtBefore)
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
	sc.Step(`^no card was posted to the home$`, func() error {
		if fakeHome.totalPostCount() != 0 {
			return fmt.Errorf("posted %d cards, want 0", fakeHome.totalPostCount())
		}
		return nil
	})
}
