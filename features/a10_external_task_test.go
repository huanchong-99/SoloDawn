//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const externalCriteria = "External criteria"

func externalTaskSteps(sc *godog.ScenarioContext, w *externalWorld) {
	sc.Step(`^the external home rejects a non-owner tool edit$`, w.nonOwnerEdit)
	sc.Step(`^a project with external X and forge "([^"]+)"$`, w.setupForge)
	sc.Step(`^the external brief runs with X as "([^"]+)"$`, w.runBrief)
	sc.Step(`^X takes the task of an external brief$`, w.takeBrief)
	sc.Step(`^X's "([^"]+)" post "([^"]+)" appears in the external task's thread$`, w.inTaskThread)
	sc.Step(`^the external task merges and its worktree is removed$`, w.merged)
	sc.Step(`^X starts an external task from the group run$`, w.startTask)
	sc.Step(`^an external task run$`, w.takeTask)
	sc.Step(`^an external task run and a managed secretary$`, w.takeTaskWithSecretary)
	sc.Step(`^the external task run ends by "([^"]+)"$`, w.interruptRun)
	sc.Step(`^the external task has one failure and the secretary receives the "([^"]+)" notice$`, w.interrupted)
	sc.Step(`^late done for the interrupted external run is harmless$`, w.lateDone)
	sc.Step(`^the external owner sets (group|host) to "([^"]+)"$`, w.setHold)
	sc.Step(`^a queued, paused external task$`, w.pausedTask)
	sc.Step(`^the external owner resumes the task$`, func() error { return w.taskOp("resume") })
	sc.Step(`^the external owner queues and jumps a newer item$`, w.queueJump)
	sc.Step(`^the jump heads the next external handout$`, w.jumpFirst)
	sc.Step(`^the external task has no approval card$`, w.noApproval)
	sc.Step(`^X requests a (Bash|second Bash) approval in the external task$`, w.askApproval)
	sc.Step(`^the owner can approve it and X receives allow$`, func() error { return w.decideApproval("approve") })
	sc.Step(`^the owner can deny it and X receives the denial message$`, func() error { return w.decideApproval("deny") })
	sc.Step(`^a connected external runner stays waiting until its 180 second presence interval expires$`, w.expirePresence)
	sc.Step(`^the external owner tries to remove X from an unfinished task$`, w.removeConflict)
	sc.Step(`^removal is refused until the external task is reassigned$`, w.reassign)
	sc.Step(`^the external member is gone and its MCP connection closes$`, w.revoked)
	externalManagedSteps(sc, w)
}

func (w *externalWorld) setupForge(forge string) error {
	return s3Run(func() error { return w.setupProject(forge) }, func() error { return w.join("X", "api_key", "ZCode") })
}

func (w *externalWorld) prepareForge(forge string) error {
	if forge == "none" {
		return nil
	}
	m := &m5World{f: w.f, cleanup: w.cleanup}
	if err := m.prepareHelperBinaries(true); err != nil {
		return err
	}
	if err := m.initBareRemote(w.repoDir); err != nil {
		return err
	}
	w.repoDir = m.repoDir
	return setScenarioEnv(w.cleanup, "PATH", m.binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func (w *externalWorld) nonOwnerEdit() error {
	var reply proto.CmdResult
	raw, err := sendSignedCmdRaw(w.ctx, w.homeLink.conn, w.ownerKey, w.homeHeader("invite"))
	if err != nil {
		return err
	}
	if err := proto.Decode(raw, &reply); err != nil {
		return err
	}
	return w.editAsGuest(reply)
}

func (w *externalWorld) editAsGuest(reply proto.CmdResult) error {
	link, err := parseS2JoinLink(reply.Link, reply.Expires)
	if err != nil {
		return err
	}
	guest := newRandomPeerKey()
	browser, err := newPeerBrowser(w.ctx, w.cleanup, guest, w.rvURL, nil)
	if err != nil {
		return err
	}
	return w.tryGuestEdit(browser, guest, link)
}

func (w *externalWorld) tryGuestEdit(browser *peerBrowser, guest identity.Key, link s2JoinLink) error {
	log := []proto.Envelope{nodetest.SignDeviceAdd(guest, guest.ID(), 1, "browser", "guest", guest.ID(), time.Now().UnixMilli())}
	conn, raw, err := joinGroupWithLink(w.ctx, w.cleanup, browser, guest, log, link, "Guest", w.ids.next(), time.Now())
	if err != nil {
		return err
	}
	var joined proto.CmdResult
	if err := proto.Decode(raw, &joined); err != nil {
		return err
	}
	return w.guestToolChange(conn, guest)
}

func (w *externalWorld) guestToolChange(conn *peerBrowserConn, guest identity.Key) error {
	cmd := proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: w.homeHeader("member"),
		Op: "set", ID: proto.ID(w.aiByName["X"].ID)}, Tool: new("Not the owner's label")}
	return externalRefusal(w.sendSignedCmd(conn.conn, guest, cmd), "forbidden")
}

func (w *externalWorld) startTask() error {
	if w.mcp == nil {
		if err := w.openMCP(); err != nil {
			return err
		}
	}
	result, err := w.call("start_task", map[string]string{"title": "External work", "plan": externalCriteria})
	if err != nil {
		return err
	}
	return w.recordTaskNumber(result)
}

func (w *externalWorld) recordTaskNumber(result *mcp.CallToolResult) error {
	raw, err := externalResult(result)
	if err != nil {
		return err
	}
	var got struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || result.IsError {
		return fmt.Errorf("start_task = %s (%v)", raw, err)
	}
	return w.waitForTask(got.Task)
}

func (w *externalWorld) waitForTask(ref string) error {
	taskNum, found := strings.CutPrefix(ref, "T-")
	n, err := strconv.Atoi(taskNum)
	if err != nil || n < 1 || !found {
		return fmt.Errorf("unexpected task reference: %q", ref)
	}
	return w.waitState(func() bool {
		w.taskID = w.tasksByNum[n].ID
		return w.taskID != ""
	})
}

func (w *externalWorld) takeTask() error {
	return s3Run(w.startTask, func() error { return w.look(0) }, func() error { return w.handout("task", externalCriteria) })
}

func (w *externalWorld) takeTaskWithSecretary() error {
	limits := w.group.Limits
	limits.TurnMinutes = 1
	cmd := proto.GroupSet{GroupCommand: proto.GroupCommand{HomeCommand: w.homeHeader("group"), Op: "set"}, Limits: &limits}
	return s3Run(w.joinSecretary, func() error { return w.hostCommand(cmd) },
		func() error { return w.scriptNotice() }, w.takeTask)
}

func (w *externalWorld) currentTask() proto.TaskRow {
	for _, task := range w.tasksByNum {
		if task.ID == w.taskID {
			return task
		}
	}
	return proto.TaskRow{}
}

func (w *externalWorld) taskHeader(op string) proto.TaskCommand {
	return proto.TaskCommand{HomeCommand: w.homeHeader("task"), Op: op, ID: proto.ID(w.taskID)}
}

func (w *externalWorld) taskOp(op string) error { return w.hostCommand(w.taskHeader(op)) }

func (w *externalWorld) pausedTask() error {
	return s3Run(w.startTask, func() error { return w.taskOp("pause") })
}

func (w *externalWorld) setHold(kind, op string) error {
	if kind == "host" {
		return w.hostCommand(w.header(kind + "." + op))
	}
	return w.hostCommand(proto.GroupCommand{HomeCommand: w.homeHeader("group"), Op: op})
}

func (w *externalWorld) queueJump() error {
	return s3Run(func() error { return w.whenOwnerMentionsAI("X", "Older item") },
		func() error { return w.whenOwnerMentionsAI("X", "Jump this item") },
		func() error { return w.waitState(w.findJumpMsg) }, func() error {
			return w.hostCommand(proto.Jump{HomeCommand: w.homeHeader("jump"), Msg: w.heldMsg})
		})
}

func (w *externalWorld) findJumpMsg() bool {
	for id, m := range w.msgs {
		if m.Text == "Jump this item" {
			w.heldMsg = proto.ID(id)
			return true
		}
	}
	return false
}

func (w *externalWorld) jumpFirst() error {
	if err := w.look(0); err != nil {
		return err
	}
	first, jump := strings.Index(w.run.Text, "Older item"), strings.Index(w.run.Text, "Jump this item")
	if jump < 0 || (first >= 0 && first < jump) {
		return fmt.Errorf("jump was not first: %s", w.run.Text)
	}
	return nil
}

func (w *externalWorld) interruptRun(end string) error {
	w.interruptDetail = ""
	switch end {
	case "failed":
		w.interruptDetail = "scripted interruption"
		return w.end(w.run.Run, "--failed", w.interruptDetail)
	case "timeout":
		return nil
	case "restart":
		return w.restartNode()
	default:
		return fmt.Errorf("unknown run end %q", end)
	}
}

func (w *externalWorld) restartNode() error {
	proc := w.homeNode.cmd.Process
	if err := proc.Kill(); err != nil {
		return err
	}
	if err := w.awaitNodeStopped(); err != nil {
		return err
	}
	return s3Run(func() error { return w.startNodeProc(w.homeNode) }, w.reconnectNode)
}

func (w *externalWorld) awaitNodeStopped() error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			conn, err := net.Dial("unix", filepath.Join(w.homeNode.data, "sock"))
			if err != nil {
				return nil
			}
			conn.Close()
		case <-w.ctx.Done():
			return w.ctx.Err()
		}
	}
}

func (w *externalWorld) reconnectNode() error {
	if err := w.browser.waitOnline(w.ctx, w.homeNode.key.ID()); err != nil {
		return err
	}
	link, err := w.dialAndWelcome(w.homeNode.key.ID())
	if err != nil {
		return err
	}
	w.homeLink = link
	return w.requestGroupSync()
}

func (w *externalWorld) interrupted(reason string) error {
	return s3Run(func() error { return w.waitState(func() bool { return w.hasInterruption(reason) }) },
		func() error {
			return w.waitState(func() bool { return w.hasTextPost("Secretary", "Interruption seen", "") })
		}, func() error { return w.waitForManagedNotice(reason) })
}

func (w *externalWorld) hasInterruption(reason string) bool {
	if w.currentTask().Failures != 1 || w.aiByName["X"].State == "working" {
		return false
	}
	for _, msg := range w.msgs {
		if externalInterruption(msg, w.aiByName["X"].ID, reason, w.interruptDetail) {
			return true
		}
	}
	return false
}

func externalInterruption(msg proto.MsgRow, member, reason, detail string) bool {
	var body proto.TurnInterruptedBody
	if msg.Kind != "system" || json.Unmarshal(msg.Body, &body) != nil {
		return false
	}
	return body.Code == "turn_interrupted" && body.AI == member && body.Reason == reason &&
		externalInterruptionDetail(body.Detail, detail)
}

func externalInterruptionDetail(got *string, want string) bool {
	return want == "" || got != nil && *got == want
}

func (w *externalWorld) lateDone() error {
	return s3Run(func() error { return w.end(w.run.Run) }, func() error {
		if w.currentTask().Failures != 1 {
			return fmt.Errorf("late done changed the failure count")
		}
		return nil
	})
}

func (w *externalWorld) noApproval() error {
	if w.holdCardID != "" {
		return fmt.Errorf("found unsolicited approval card %s", w.holdCardID)
	}
	return nil
}

func (w *externalWorld) askApproval(label string) error {
	w.holdCardID = ""
	ch := make(chan error, 1)
	w.approval = ch
	go func() { ch <- w.approvalCall(label) }()
	return w.waitState(func() bool { return w.holdCardID != "" })
}

func (w *externalWorld) approvalCall(label string) error {
	args := map[string]any{"tool_name": "Bash", "input": map[string]string{"command": "echo " + label}, "tool_use_id": label}
	res, err := w.call("approve", args)
	if err != nil {
		return err
	}
	return verifyApprovalResult(res, label)
}

func verifyApprovalResult(res *mcp.CallToolResult, label string) error {
	raw, err := externalResult(res)
	if err != nil {
		return err
	}
	var got struct{ Behavior, Message string }
	if err := json.Unmarshal(raw, &got); err != nil || res.IsError {
		return fmt.Errorf("invalid approval result: %s (%v)", raw, err)
	}
	if externalApprovalMatches(label, got.Behavior, got.Message) {
		return nil
	}
	return fmt.Errorf("unexpected decision for %s: %s", label, raw)
}

func externalApprovalMatches(label, behavior, message string) bool {
	return label == "Bash" && behavior == "allow" ||
		label == "second Bash" && behavior == "deny" && message == "Denied by the owner."
}

func (w *externalWorld) decideApproval(action string) error {
	card := w.msgs[w.holdCardID]
	if card.Thread == nil || *card.Thread != w.taskID {
		return fmt.Errorf("the approval card is not in task %q: %+v", w.taskID, card)
	}
	if err := w.hostCommand(proto.Act{HomeCommand: w.homeHeader("act"), Msg: proto.ID(card.ID), Action: action}); err != nil {
		return err
	}
	select {
	case err := <-w.approval:
		return err
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *externalWorld) expirePresence() error {
	return runExtBubble(w.f.t, checkExternalPresenceInterval)
}

func checkExternalPresenceInterval(b *extBubble) error {
	client, err := connectExternal(b, hlAI1)
	if err != nil {
		return err
	}
	b.leave()
	if err := client.Close(); err != nil {
		return err
	}
	synctest.Wait()
	<-time.After(180*time.Second - time.Nanosecond)
	if err := checkPresenceState(b, "waiting"); err != nil {
		return err
	}
	<-time.After(time.Nanosecond)
	synctest.Wait()
	return checkPresenceState(b, "offline")
}

func checkPresenceState(b *extBubble, want string) error {
	var reported any
	for _, cmd := range b.home.snapshotCmds() {
		if cmd["op"] == "status" && cmd["id"] == hlAI1 {
			reported = cmd["state"]
		}
	}
	if got := b.state(); got != want || reported != want {
		return fmt.Errorf("external state = %q, reported %v, want %q", got, reported, want)
	}
	return nil
}

func (w *externalWorld) removeConflict() error {
	w.mcp = nil
	if err := w.startTask(); err != nil {
		return err
	}
	return externalRefusal(w.remove(), "conflict")
}

func (w *externalWorld) reassign() error {
	if err := w.joinManaged("M", false); err != nil {
		return err
	}
	return w.hostCommand(proto.TaskAssign{TaskCommand: w.taskHeader("assign"), Assignee: proto.ID(w.aiByName["M"].ID)})
}

func (w *externalWorld) revoked() error {
	if err := w.confirmGone(); err != nil {
		return err
	}
	wait := newWait(5*time.Second, "removed member's MCP session to close", nil)
	defer wait.stop()
	result, err := recvWait(wait, w.mcpDone())
	if err != nil {
		return err
	}
	return result
}

func (w *externalWorld) mcpDone() <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- w.mcp.Wait() }()
	return ch
}

func (w *externalWorld) confirmGone() error {
	db, err := store.Open(filepath.Join(w.homeNode.data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Write(func(tx *store.Tx) error {
		_, err := tx.Row(string(w.groupID), "member", w.aiByName["X"].ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("removed external member still at the home (read error %v)", err)
	})
}

func (w *externalWorld) merged() error {
	if err := w.waitExternalPush("external task merge", func() bool { return w.currentTask().State == "merged" }); err != nil {
		return err
	}
	return waitUntil(5*time.Second, "merged worktree removal", func() error {
		_, err := os.Stat(w.folder)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return abort(err)
		}
		return fmt.Errorf("merged worktree still present: %s", w.folder)
	})
}
