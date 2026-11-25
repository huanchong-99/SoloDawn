package host

import (
	"cmp"
	"context"
	"encoding/json"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	Register(Attached, "deliver", attachHome)
	Register(Settled, "deliver", settleDelivery)
	Register(Pushes, "inbox", pushInbox)
	Register(Pushes, "rows", pushRows)
	Register(Pushes, "reset", pushReset)
	Register(Pushes, "groups", pushHomeGroups)
	Register(Listeners, "jump", listenJump)
	Provide(&Deliver, deliverNow)
}

type aiGroupKey struct{ group, member string }

type groupSync struct {
	home          string
	cursor        int64
	live, syncing bool
	refresh       bool
	token         *byte
	waiters       []chan struct{}
}

type syncJob struct {
	group string
	since int64
	token *byte
}

type homeSync struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type aiInfo struct {
	agent, name string
	secretary   bool
}

type verifiedItem struct {
	home, group string
	raw         json.RawMessage
	wire        proto.InboxItem
	item        proto.Item
}

type routedTarget struct {
	key  SessionKey
	kind Kind
}

// A preemption is a turn that a queue jump for another session interrupted
// (flows/agent-turn.md C). It is kept in memory only: after a restart the
// home's row 13 resumes the task (contracts/task-machine.md section 4).
type preemption struct {
	session SessionKey // the interrupted session, owed a turn of its own
	msg     proto.ID   // the message whose jump interrupted it
	ended   bool       // its turn ended interrupted; set by listenJump under delivery.mu
	told    bool       // the jump turn was about to start: task preempted is reported or not needed
	paused  bool       // the home accepted task preempted: task resume is owed
}

// delivery holds per-host sync cursors, held queues, open pre-emptions and active merges via Part[delivery](h).
// mu guards in-memory state only and is never held across store, session, row, hold or Git calls.
type delivery struct {
	mu            sync.Mutex
	wg            sync.WaitGroup
	groups        map[string]*groupSync
	held          map[aiGroupKey][]verifiedItem
	pre           map[aiGroupKey]*preemption
	merges        map[string][]verifiedItem
	merging       map[string]bool
	busy, recheck map[aiGroupKey]bool
	cancels       map[string]*homeSync
}

func deliv(h *Host) *delivery {
	d := Part[delivery](h)
	d.mu.Lock()
	if d.groups == nil {
		d.groups, d.held, d.merges = map[string]*groupSync{}, map[aiGroupKey][]verifiedItem{}, map[string][]verifiedItem{}
		d.merging, d.cancels = map[string]bool{}, map[string]*homeSync{}
		d.busy, d.recheck, d.pre = map[aiGroupKey]bool{}, map[aiGroupKey]bool{}, map[aiGroupKey]*preemption{}
	}
	d.mu.Unlock()
	return d
}

func (d *delivery) groupLocked(group, home string) *groupSync {
	gs := d.groups[group]
	if gs == nil {
		gs = &groupSync{home: home}
		d.groups[group] = gs
	}
	return gs
}

func attachHome(h *Host, device string) {
	conn, ok := h.Home(device)
	if !ok {
		return
	}
	homes, err := joinSavedHomes(h.Store)
	if err != nil {
		log.Printf("host: load homes: %v", err)
		return
	}
	d := deliv(h)
	groups := groupsForHome(d, homes, device)
	if len(groups) == 0 {
		return
	}
	ctx, jobs := d.beginHomeSync(h, device, conn, groups)
	h.bg(d.wg.Go, "home_sync", func() {
		for _, job := range jobs {
			syncOneGroup(ctx, h, d, conn, device, job)
		}
	})
}

func settleDelivery(h *Host) {
	d := deliv(h)
	d.mu.Lock()
	for dev, entry := range d.cancels {
		entry.cancel()
		delete(d.cancels, dev)
	}
	d.mu.Unlock()
	d.wg.Wait()
}

func groupsForHome(d *delivery, homes []proto.Home, device string) []string {
	seen := map[string]bool{}
	d.mu.Lock()
	for g, gs := range d.groups {
		if gs.home == device {
			seen[g] = true
		}
	}
	d.mu.Unlock()
	for _, entry := range homes {
		if entry.Home == device {
			seen[string(entry.Group)] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func (d *delivery) setHomeEntryLocked(h *Host, device string, conn Home) *homeSync {
	if prev := d.cancels[device]; prev != nil {
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	entry := &homeSync{ctx: ctx, cancel: cancel}
	d.cancels[device] = entry
	h.bg(d.wg.Go, "home_watch", func() { watchHomeConn(h, d, device, conn, entry) })
	return entry
}

func watchHomeConn(h *Host, d *delivery, device string, conn Home, entry *homeSync) {
	select {
	case <-conn.Done():
		h.diag.Info("host.detach", "home", diagDevice(device))
		entry.cancel()
		d.mu.Lock()
		if d.cancels[device] == entry {
			delete(d.cancels, device)
		}
		d.mu.Unlock()
	case <-entry.ctx.Done():
	}
}

func (d *delivery) beginHomeSync(h *Host, device string, conn Home, groups []string) (context.Context, []syncJob) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry := d.setHomeEntryLocked(h, device, conn)
	jobs := make([]syncJob, 0, len(groups))
	for _, g := range groups {
		jobs = append(jobs, d.startGroupJobLocked(g, device))
	}
	return entry.ctx, jobs
}

func (d *delivery) startGroupJobLocked(group, device string) syncJob {
	gs := d.groupLocked(group, device)
	gs.token = new(byte)
	gs.syncing, gs.live = true, false
	return syncJob{group: group, since: gs.cursor, token: gs.token}
}

func (d *delivery) replaceHome(group, device string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groups[group]
	if gs == nil || gs.home == device {
		return false
	}
	for _, ch := range gs.waiters {
		close(ch)
	}
	*gs = groupSync{home: device}
	maps.DeleteFunc(d.held, func(key aiGroupKey, _ []verifiedItem) bool { return key.group == group })
	delete(d.merges, group)
	return true
}

func ensureGroupSynced(h *Host, d *delivery, device, group string) {
	conn, ok := h.Home(device)
	if !ok {
		return
	}
	if ctx, job, started := d.beginSingleGroupSync(h, device, conn, group); started {
		h.bg(d.wg.Go, "group_sync", func() { syncOneGroup(ctx, h, d, conn, device, job) }, "g", group)
	}
}

func (d *delivery) beginSingleGroupSync(h *Host, device string, conn Home, group string) (context.Context, syncJob, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if gs := d.groups[group]; gs != nil && (gs.syncing || gs.live) {
		return nil, syncJob{}, false
	}
	entry := d.cancels[device]
	if entry == nil {
		entry = d.setHomeEntryLocked(h, device, conn)
	}
	return entry.ctx, d.startGroupJobLocked(group, device), true
}

func syncGroupState(ctx context.Context, h *Host, conn Home, device, group string) error {
	d := deliv(h)
	for {
		if _, ok := joinCachedGroup(h, group); ok {
			return nil
		}
		job, waitCh := d.claimOrWaitGroupSync(device, group)
		if waitCh == nil {
			return syncOneGroup(ctx, h, d, conn, device, job)
		}
		if !joinWaitChannel(ctx, waitCh) {
			d.removeGroupWaiter(group, waitCh)
			return errHomeUnreachable
		}
	}
}

func (d *delivery) claimOrWaitGroupSync(device, group string) (syncJob, chan struct{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groupLocked(group, device)
	if gs.syncing {
		// Single reader: the joinWaitChannel call in syncGroupState that created ch.
		ch := make(chan struct{})
		gs.waiters = append(gs.waiters, ch)
		return syncJob{}, ch
	}
	gs.cursor = 0
	return d.startGroupJobLocked(group, device), nil
}

func (d *delivery) removeGroupWaiter(group string, ch chan struct{}) {
	match := func(cur chan struct{}) bool { return cur == ch }
	d.mu.Lock()
	gs := d.groups[group]
	gs.waiters = slices.DeleteFunc(gs.waiters, match)
	d.mu.Unlock()
}

func syncOneGroup(ctx context.Context, h *Host, d *delivery, conn Home, device string, job syncJob) error {
	build := func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: job.group, Since: job.since, StateOnly: true}
	}
	h.diag.Info("sync.begin", "g", job.group, "home", diagDevice(device))
	answer, err := conn.Request(ctx, build)
	synced, syncErr := decodeSyncAnswer(answer, err)
	ok := syncErr == nil
	h.diag.Info("sync.end", "g", job.group, "home", diagDevice(device), "seq", synced.Seq, "ok", ok)
	if current, refresh := d.finishGroupSync(job, device, synced.Seq, ok); current {
		settleHomeSync(h, d, device, job.group, answer, ok, refresh)
	}
	return syncErr
}

func decodeSyncAnswer(answer json.RawMessage, err error) (proto.Synced, error) {
	if err != nil {
		return proto.Synced{}, errHomeUnreachable
	}
	var refused proto.ErrorResponse
	if proto.Decode(answer, &refused) == nil {
		return proto.Synced{}, &Error{Code: refused.Err.Code, Message: refused.Err.Message}
	}
	var synced proto.Synced
	if proto.Decode(answer, &synced) != nil {
		return proto.Synced{}, errHomeUnreachable
	}
	return synced, nil
}

func (d *delivery) finishGroupSync(job syncJob, device string, seq int64, ok bool) (bool, bool) {
	d.mu.Lock()
	gs := d.groupLocked(job.group, device)
	if gs.token != job.token {
		d.mu.Unlock()
		return false, false
	}
	gs.syncing = false
	if ok {
		gs.cursor, gs.live = seq, true
	}
	waiters, refresh := gs.waiters, gs.refresh
	gs.waiters, gs.refresh = nil, false
	gs.live = gs.live && !refresh
	d.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
	return true, refresh
}

func pushReset(h *Host, home string, frame json.RawMessage) {
	var rst proto.Reset
	if proto.Decode(frame, &rst) != nil || rst.Group == "" {
		return
	}
	d := deliv(h)
	h.Rows.Reset(rst.Group)
	d.mu.Lock()
	gs := d.groupLocked(rst.Group, home)
	gs.cursor, gs.live = 0, false
	d.mu.Unlock()
}

func pushRows(h *Host, home string, frame json.RawMessage) {
	var batch proto.Rows
	if proto.Decode(frame, &batch) != nil || batch.Group == "" {
		return
	}
	h.diag.Info("rows.recv", "g", batch.Group, "home", diagDevice(home), "seq", diagSpan(batch.Rows))
	d := deliv(h)
	ensureGroupSynced(h, d, home, batch.Group)
	for _, raw := range batch.Rows {
		applyGroupRow(h, d, home, batch.Group, raw)
	}
	h.bg(d.wg.Go, "group_state", func() { afterGroupState(h, d, batch.Group) }, "g", batch.Group)
}

func applyGroupRow(h *Host, d *delivery, home, group string, raw json.RawMessage) {
	hdr, ok := decodeRowHeader(raw)
	if !ok || !newerRowSeq(h.Rows, group, hdr) {
		return
	}
	h.Rows.Put(group, hdr.Type, hdr.ID, raw)
	d.advanceLiveCursor(group, home, hdr.Seq)
}

func decodeRowHeader(raw json.RawMessage) (proto.RowHeader, bool) {
	var hdr proto.RowHeader
	return hdr, proto.Decode(raw, &hdr) == nil && hdr.Type != "" && hdr.ID != "" && hdr.Seq >= 1
}

func newerRowSeq(rows *Rows, group string, hdr proto.RowHeader) bool {
	prevRaw, ok := rows.Get(group, hdr.Type, hdr.ID)
	if !ok {
		return true
	}
	var prev proto.RowHeader
	return proto.Decode(prevRaw, &prev) != nil || hdr.Seq > prev.Seq
}

func (d *delivery) advanceLiveCursor(group, home string, seq int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groupLocked(group, home)
	if gs.live {
		gs.cursor = max(gs.cursor, seq)
	}
}

func afterGroupState(h *Host, d *delivery, group string) {
	interruptPausedTurns(h, d, group)
	cleanupGroupState(h, d, group)
	stepMerges(h, d, group)
	resumeGroupMerges(h, d, group)
	resumeGroupHostedChecks(h, d, group)
	for _, key := range d.heldKeys() {
		deliverPair(h, d, key)
	}
}

func interruptPausedTurns(h *Host, d *delivery, group string) {
	if d.groupSyncActive(group) {
		return
	}
	paused := groupIsPaused(h, group)
	for _, s := range h.Sessions.All() {
		if shouldInterruptPaused(h, group, paused, s) {
			interruptSessionTurn(h, s.SessionKey, "stop", "")
		}
	}
}

func shouldInterruptPaused(h *Host, group string, groupPaused bool, s Session) bool {
	if s.Group != group || s.State != Working {
		return false
	}
	return groupPaused || sessionTaskPaused(h, group, s.Task)
}

func sessionTaskPaused(h *Host, group, taskID string) bool {
	if taskID == "" {
		return false
	}
	task, ok := lookupTaskRow(h, group, taskID)
	return ok && task.State == "paused"
}

func cleanupGroupState(h *Host, d *delivery, group string) {
	if d.groupSyncActive(group) {
		return
	}
	cleanupGoneMembers(h, group)
	cleanupTaskSessions(h, group)
	handBackSaved(h, group)
	tasks := liveTasks(h, group)
	if len(tasks) == 0 {
		return
	}
	pDir := projectDir(h, group)
	if !hasGitClone(pDir) {
		return
	}
	grp, _ := lookupRow[proto.GroupRow](h, group, "group", group)
	for _, task := range tasks {
		cleanupTaskFolder(context.Background(), pDir, grp, task)
		cleanupReviewFolder(context.Background(), pDir, task)
	}
}

func cleanupGoneMembers(h *Host, group string) {
	for _, raw := range h.Rows.Of(group, "member") {
		var row proto.GoneRow
		if proto.Decode(raw, &row) == nil && row.Gone {
			watchGone(h, group, row.ID)
			leaveGoneMember(h, group, row.ID)
		}
	}
}

func cleanupTaskSessions(h *Host, group string) {
	for _, s := range h.Sessions.All() {
		if s.Group == group && s.Task != "" && sessionTaskObsolete(h, group, s) {
			Runner.Close(h, s.SessionKey)
			h.Sessions.End(s.SessionKey)
			if s.State == Takeover {
				h.ClosePTY(s.Group, s.Member)
			}
		}
	}
}

func sessionTaskObsolete(h *Host, group string, s Session) bool {
	task, ok := lookupTaskRow(h, group, s.Task)
	role := task.Reviewer
	if s.Kind == TaskSession {
		role = task.Assignee
	}
	return ok && (task.State == "merged" || task.State == "cancelled" || role != s.Member)
}

func cleanupTaskFolder(ctx context.Context, pDir string, grp proto.GroupRow, task proto.TaskRow) {
	name := strconv.Itoa(task.Num) + "-" + string(task.Slug)
	wtPath := filepath.Join(pDir, "wt", name)
	if !pathExists(wtPath) {
		return
	}
	if task.State == "merged" {
		RemoveTaskWorkspace(ctx, pDir, task.Num, string(task.Slug), newSlot(pDir, "wt", name, "", "").holder(ctx), TaskMerged)
		(ReviewedMerge{Repo: filepath.Join(pDir, "repo"), Branch: "task/" + name, Commit: task.Commit}).DeleteBranch(ctx)
		return
	}
	releaseUnmergedWorktree(ctx, pDir, grp, task, name, wtPath)
}

func releaseUnmergedWorktree(ctx context.Context, pDir string, grp proto.GroupRow, task proto.TaskRow, name, wtPath string) {
	holder := newSlot(pDir, "wt", name, "", "").holder(ctx)
	if task.State == "cancelled" {
		RemoveTaskWorkspace(ctx, pDir, task.Num, string(task.Slug), holder, TaskCancelled)
	} else if holder != task.Assignee {
		if grp.Forge != "none" && holder != "" {
			(TaskForge{Worktree: wtPath, Branch: "task/" + name, Base: grp.Base, Templates: grp.Templates}).PushBranch(ctx)
		}
		RemoveTaskWorkspace(ctx, pDir, task.Num, string(task.Slug), holder, TaskGivenAway)
	}
}

func cleanupReviewFolder(ctx context.Context, pDir string, task proto.TaskRow) {
	numStr := strconv.Itoa(task.Num)
	if pathExists(filepath.Join(pDir, "review", numStr)) {
		if hld := newSlot(pDir, "review", numStr, "", "").holder(ctx); task.State == "merged" || task.State == "cancelled" || hld != task.Reviewer {
			RemoveReviewWorkspace(ctx, pDir, task.Num, hld)
		}
	}
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

type rawInboxFrame struct {
	Type  string            `json:"t"`
	Group proto.ID          `json:"g"`
	Items []json.RawMessage `json:"items"`
}

func pushInbox(h *Host, home string, frame json.RawMessage) {
	var outer rawInboxFrame
	if proto.Decode(frame, &outer) != nil {
		return
	}
	d := deliv(h)
	group := string(outer.Group)
	ensureGroupSynced(h, d, home, group)
	for _, rawElem := range outer.Items {
		handleInboxElem(h, d, home, group, rawElem)
	}
}

func handleInboxElem(h *Host, d *delivery, home, group string, rawElem json.RawMessage) {
	var wire proto.InboxItem
	if proto.Decode(rawElem, &wire) != nil {
		return
	}
	it := wire.Item
	seen, err := hasAuditItem(h.Store, home, it.Item)
	if err != nil {
		log.Printf("host: check audit item %d: %v", it.Item, err)
		return
	}
	h.diag.Info("inbox.recv", "g", group, "item", it.Item, "ai", string(it.AI), "kind", it.Why, "seen", seen)
	if seen {
		sendAck(h, home, group, it.Item)
		return
	}
	dispatchArrivedItem(h, d, verifiedItem{home: home, group: group, raw: rawElem, wire: wire, item: it})
}

func dispatchArrivedItem(h *Host, d *delivery, v verifiedItem) {
	if v.item.Why == "watch" {
		watchItem(h, v)
		return
	}
	if ai, local, _ := lookupLocalAI(h, v.group, string(v.item.AI)); local && tryOpenWait(h, v, ai.agent) {
		return
	}
	enqueueVerified(h, d, v)
}

func waitTargetID(v verifiedItem) string {
	if v.item.Why == "decided" {
		return string(v.item.Msg)
	}
	if v.item.Why == "reply" {
		return replyCallID(v)
	}
	return ""
}

func replyCallID(v verifiedItem) string {
	var msg proto.MsgRow
	if proto.Decode(v.wire.Msg, &msg) != nil || msg.ID != string(v.item.Msg) || msg.Kind != "reply" || msg.Re == nil {
		return ""
	}
	return *msg.Re
}

func tryOpenWait(h *Host, v verifiedItem, agent string) bool {
	target := waitTargetID(v)
	if target == "" {
		return false
	}
	waited, err := h.Hand(target, v.raw)
	if !waited {
		return false
	}
	h.diag.Info("inbox.hand", "g", v.group, "item", v.item.Item, "wait", target)
	if err != nil {
		refuseItem(h, v, agent, err.Error())
		return true
	}
	row := AuditRow{Group: v.group, Agent: agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: v.item.Why, By: v.home}
	if recordAudit(h, row) == nil {
		sendAck(h, v.home, v.group, v.item.Item)
	}
	return true
}

func enqueueVerified(h *Host, d *delivery, v verifiedItem) {
	if v.item.Why == "merge" {
		d.enqueueMerge(v)
	} else {
		key := aiGroupKey{group: v.group, member: string(v.item.AI)}
		d.mu.Lock()
		d.held[key] = enqueueHeld(d.held[key], v)
		d.mu.Unlock()
	}
	h.bg(d.wg.Go, "deliver", func() { deliverNow(h, v.group, string(v.item.AI)) }, "g", v.group)
}

func (d *delivery) enqueueMerge(v verifiedItem) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, existing := range d.merges[v.group] {
		if existing.item.Item == v.item.Item {
			return
		}
	}
	d.merges[v.group] = append(d.merges[v.group], v)
}

func enqueueHeld(queue []verifiedItem, incoming verifiedItem) []verifiedItem {
	it := incoming.item
	known := func(e verifiedItem) bool { return e.item.Item == it.Item && (bool(e.item.Jump) || !bool(it.Jump)) }
	if slices.ContainsFunc(queue, known) {
		return queue
	}
	if it.Jump {
		replaced := func(e verifiedItem) bool { return e.item.Item == it.Item || retiredByJump(e.item, it) }
		queue = slices.DeleteFunc(queue, replaced)
	}
	return insertHeld(queue, incoming)
}

// retiredByJump reports whether old is an item the home retired when it
// queued jump for the same message (contracts/commands.md 1.4), be it the
// waiting item or an earlier jump of that message: the home never sends old
// again, and the message is delivered once.
func retiredByJump(old, jump proto.Item) bool {
	return old.Why == jump.Why && old.Msg == jump.Msg
}

func insertHeld(queue []verifiedItem, incoming verifiedItem) []verifiedItem {
	if !bool(incoming.item.Jump) {
		return append(queue, incoming)
	}
	nonJump := func(e verifiedItem) bool { return !bool(e.item.Jump) }
	if pos := slices.IndexFunc(queue, nonJump); pos >= 0 {
		return slices.Insert(queue, pos, incoming)
	}
	return append(queue, incoming)
}

func lookupLocalAI(h *Host, group, member string) (ai aiInfo, local, present bool) {
	raw, ok := h.Rows.Get(group, "member", member)
	if !ok {
		return aiInfo{}, false, false
	}
	var m proto.AIMemberRow
	if proto.Decode(raw, &m) == nil && m.Kind == "ai" && m.Host == h.Key.ID() {
		return aiInfo{agent: m.CLI, name: m.Name, secretary: m.Secretary}, true, true
	}
	return aiInfo{}, false, true
}

func lookupRow[T any](h *Host, group, kind, id string) (T, bool) {
	var zero T
	raw, ok := h.Rows.Get(group, kind, id)
	if !ok {
		return zero, false
	}
	var out T
	return out, proto.Decode(raw, &out) == nil
}

func lookupTaskRow(h *Host, group, taskID string) (proto.TaskRow, bool) {
	return lookupRow[proto.TaskRow](h, group, "task", taskID)
}

func deliverNow(h *Host, group, member string) {
	d := deliv(h)
	primary := aiGroupKey{group: group, member: member}
	deliverPair(h, d, primary)
	for _, g := range d.mergeGroups() {
		stepMerges(h, d, g)
	}
	for _, other := range d.heldKeys() {
		if other != primary {
			deliverPair(h, d, other)
		}
	}
}

func (d *delivery) mergeGroups() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Sorted(maps.Keys(d.merges))
}

func deliverPair(h *Host, d *delivery, key aiGroupKey) {
	if !d.enterAI(key) {
		return
	}
	for {
		for stepAI(h, d, key) {
		}
		if !d.leaveAI(key) {
			return
		}
	}
}

func (d *delivery) enterAI(key aiGroupKey) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.busy[key] {
		d.recheck[key] = true
		return false
	}
	d.busy[key] = true
	return true
}

func (d *delivery) leaveAI(key aiGroupKey) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recheck[key] {
		delete(d.recheck, key)
		return true
	}
	delete(d.busy, key)
	return false
}

// heldKeys is the AIs that delivery owes something: held items, or the turn
// of its own of a session that a jump interrupted.
func (d *delivery) heldKeys() []aiGroupKey {
	d.mu.Lock()
	defer d.mu.Unlock()
	keys := slices.AppendSeq(slices.Collect(maps.Keys(d.held)), maps.Keys(d.pre))
	order := func(a, b aiGroupKey) int {
		return cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(a.member, b.member))
	}
	slices.SortFunc(keys, order)
	return slices.Compact(keys)
}

func routeSessionItem(h *Host, ai aiInfo, v verifiedItem) routedTarget {
	g, m := v.group, string(v.item.AI)
	if ai.secretary {
		return routedTarget{key: SessionKey{Group: g, Member: m}, kind: GroupSession}
	}
	if isTaskWorkWhy(v.item.Why) {
		return routedTarget{key: SessionKey{Group: g, Member: m, Task: string(v.item.Task)}, kind: TaskSession}
	}
	if v.item.Why == "review" {
		return routedTarget{key: SessionKey{Group: g, Member: m, Task: string(v.item.Task)}, kind: ReviewSession}
	}
	if v.item.Why == "decided" {
		return routeDecidedItem(h, g, m, string(v.item.Task))
	}
	return routeThreadOrGroup(h, g, m, v)
}

func isTaskWorkWhy(why string) bool {
	return why == "assign" || why == "checks" || why == "reviewed" || why == "resume"
}

func routeDecidedItem(h *Host, group, member, task string) routedTarget {
	if s, ok := h.Sessions.Get(SessionKey{Group: group, Member: member, Task: task}); ok && (s.Kind == TaskSession || s.State != Dormant) {
		return routedTarget{key: s.SessionKey, kind: s.Kind}
	}
	return routedTarget{key: SessionKey{Group: group, Member: member}, kind: GroupSession}
}

func routeThreadOrGroup(h *Host, group, member string, v verifiedItem) routedTarget {
	if v.item.Why == "mention" || v.item.Why == "reply" {
		if thread := threadTaskID(v); thread != "" {
			if s, ok := h.Sessions.Get(SessionKey{Group: group, Member: member, Task: thread}); ok {
				return routedTarget{key: s.SessionKey, kind: s.Kind}
			}
		}
	}
	return routedTarget{key: SessionKey{Group: group, Member: member}, kind: GroupSession}
}

func threadTaskID(v verifiedItem) string {
	if v.item.Task != "" {
		return string(v.item.Task)
	}
	var msg proto.MsgRow
	if proto.Decode(v.wire.Msg, &msg) == nil && msg.Thread != nil {
		return *msg.Thread
	}
	return ""
}

func stepAI(h *Host, d *delivery, key aiGroupKey) bool {
	if d.groupSyncActive(key.group) {
		return false
	}
	ai, local, present := lookupLocalAI(h, key.group, key.member)
	if !local {
		if present {
			refuseHeldNotHosted(h, d, key)
		}
		return false
	}
	drainOpenWaits(h, d, key, ai.agent)
	if ai.agent == externalAgent {
		extWake(h, key)
		return false
	}
	return stepSessions(h, d, key, ai)
}

// stepSessions gives the AI its next turn. While it runs one, only a jump
// item does anything: it interrupts that turn and stays held until the turn
// has ended (flows/agent-turn.md C).
func stepSessions(h *Host, d *delivery, key aiGroupKey, ai aiInfo) bool {
	if s, busy := findSessionInState(memberSessions(h, key.group, key.member), Working); busy {
		jumpRunningTurn(h, d, key, ai, s.SessionKey)
		return false
	}
	d.dropUninterrupted(key)
	target, batch, ok := takeEligibleBatch(h, d, key, ai)
	if !ok {
		return continueInterrupted(h, d, key)
	}
	reportPreempted(h, d, key)
	return deliverTargetBatch(h, d, key, ai, target, batch)
}

// jumpRunningTurn interrupts the turn the AI is running, for the first jump
// item that only this turn holds (flows/agent-turn.md step 12). The item
// stays held and unacknowledged until the turn has ended, however it ends. A
// jump for another session opens a pre-emption; while one is open, a further
// jump for another session interrupts nothing and goes when the jump turn
// has ended.
func jumpRunningTurn(h *Host, d *delivery, key aiGroupKey, ai aiInfo, running SessionKey) {
	jump, target, ok := firstJump(h, d, key, ai)
	if !ok {
		return
	}
	other := target != running
	if other && !d.openPreemption(key, &preemption{session: running, msg: jump.Msg}) {
		return
	}
	if !interruptSessionTurn(h, running, "jump", "") && other {
		d.closePreemption(key)
	}
}

func firstJump(h *Host, d *delivery, key aiGroupKey, ai aiInfo) (proto.Item, SessionKey, bool) {
	d.mu.Lock()
	queue := slices.Clone(d.held[key])
	d.mu.Unlock()
	paused := groupIsPaused(h, key.group)
	if len(queue) > 0 {
		diagTake(h, key, "", nil, queue, paused)
	}
	for _, v := range queue {
		if target := routeSessionItem(h, ai, v); bool(v.item.Jump) && !heldBesidesTurn(h, target, v, paused) {
			return v.item, target.key, true
		}
	}
	return proto.Item{}, SessionKey{}, false
}

// diagTake writes inbox.take: what a look at the AI's queue took and left.
// While the AI runs a turn it takes nothing.
func diagTake(h *Host, key aiGroupKey, task string, took, left []verifiedItem, paused bool) {
	h.diag.Info("inbox.take", "g", key.group, "ai", key.member, "task", task, "took", diagItems(took), "left", diagItems(left), "paused", paused)
}

func (d *delivery) openPreemption(key aiGroupKey, p *preemption) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pre[key] != nil {
		return false
	}
	d.pre[key] = p
	return true
}

func (d *delivery) closePreemption(key aiGroupKey) {
	d.mu.Lock()
	delete(d.pre, key)
	d.mu.Unlock()
}

// listenJump notes that the turn a jump interrupted has ended interrupted.
// One that crashes, or ends by itself first, is owed nothing: section E of
// flows/agent-turn.md has its one outcome.
func listenJump(h *Host, key SessionKey, e Event) {
	if e.Kind != TurnEnded || e.Status != "interrupted" {
		return
	}
	d := deliv(h)
	d.mu.Lock()
	defer d.mu.Unlock()
	if p := d.pre[aiGroupKey{group: key.Group, member: key.Member}]; p != nil && p.session == key {
		p.ended = true
	}
}

// dropUninterrupted closes a pre-emption whose turn is over without having
// ended interrupted.
func (d *delivery) dropUninterrupted(key aiGroupKey) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p := d.pre[key]; p != nil && !p.ended {
		delete(d.pre, key)
	}
}

// reportPreempted reports task preempted for an interrupted task session,
// once, before the jump turn in another session (flows/agent-turn.md step
// 12). A refusal, as at step merge, is not an error: then no resume is owed.
func reportPreempted(h *Host, d *delivery, key aiGroupKey) {
	d.mu.Lock()
	p := d.pre[key]
	d.mu.Unlock()
	if p == nil || p.told {
		return
	}
	p.told = true
	if s, ok := h.Sessions.Get(p.session); ok && s.Kind == TaskSession {
		cmd := proto.TaskPreempted{TaskCommand: hostTaskCommand(h, key.group, "preempted", s.Task), Msg: p.msg}
		p.paused = sendHostCommand(context.Background(), h, s.Home, marshalWire(cmd)) == nil
	}
}

// continueInterrupted gives the session that a jump interrupted its turn of
// its own, once no jump item can go before it (flows/agent-turn.md step 14).
// It waits while that session gets no turn, and for the home when task
// resume is owed.
func continueInterrupted(h *Host, d *delivery, key aiGroupKey) bool {
	d.mu.Lock()
	p := d.pre[key]
	d.mu.Unlock()
	if p == nil {
		return false
	}
	s, ok := h.Sessions.Get(p.session)
	if ok && !mayContinue(h, p, s) {
		return false
	}
	d.closePreemption(key)
	if ok {
		continueTurn(h, s)
	}
	return true
}

func mayContinue(h *Host, p *preemption, s Session) bool {
	return !groupIsPaused(h, s.Group) && !turnsHeld(h, s.SessionKey) && !aiWorking(h, s.SessionKey) && resumePreempted(h, p, s)
}

// resumePreempted reports task resume for a task this host pre-empted
// (contracts/task-machine.md row 13b). It reports false when the home could
// not be reached: the report is made again when delivery next runs.
func resumePreempted(h *Host, p *preemption, s Session) bool {
	if !p.paused {
		return true
	}
	err := sendHostCommand(context.Background(), h, s.Home, marshalWire(hostTaskCommand(h, s.Group, "resume", s.Task)))
	p.paused = isUnreachable(err)
	return !p.paused
}

// continueTurn gives a session the turn of its own, unless its task is
// paused or failed by now: then it gets no turn (contracts/task-machine.md
// section 4).
func continueTurn(h *Host, s Session) {
	if taskStopped(h, s.Group, s.Task) {
		return
	}
	text, turn, err := Compose(h, s, ContinueTurn, nil)
	if err == nil {
		err = Runner.Turn(context.Background(), h, s.SessionKey, text, turn)
	}
	if err != nil {
		log.Printf("host: continue %v: %v", s.SessionKey, err)
	}
}

func refuseHeldNotHosted(h *Host, d *delivery, key aiGroupKey) {
	d.mu.Lock()
	items := d.held[key]
	delete(d.held, key)
	delete(d.pre, key)
	d.mu.Unlock()
	for _, v := range items {
		refuseItem(h, v, "", "not_hosted_here")
	}
}

func drainOpenWaits(h *Host, d *delivery, key aiGroupKey, agent string) {
	d.mu.Lock()
	queue := slices.Clone(d.held[key])
	d.mu.Unlock()
	handed := map[int64]bool{}
	for _, v := range queue {
		if tryOpenWait(h, v, agent) {
			handed[v.item.Item] = true
		}
	}
	d.removeHeldItems(key, handed)
}

func (d *delivery) groupSyncActive(group string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groups[group]
	return gs != nil && !gs.live
}

func (d *delivery) removeHeldItems(key aiGroupKey, ids map[int64]bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	drop := func(e verifiedItem) bool { return ids[e.item.Item] }
	setQueue(d.held, key, slices.DeleteFunc(d.held[key], drop))
}

func takeEligibleBatch(h *Host, d *delivery, key aiGroupKey, ai aiInfo) (routedTarget, []verifiedItem, bool) {
	d.mu.Lock()
	queue := slices.Clone(d.held[key])
	jumpsOnly := d.pre[key] != nil
	d.mu.Unlock()
	var paused bool
	readPause := func() bool {
		paused = groupIsPaused(h, key.group)
		return paused
	}
	target, taken, takenIDs := selectEligibleBatch(h, ai, itemsToGo(queue, jumpsOnly), readPause)
	if len(queue) > 0 {
		left := slices.DeleteFunc(queue, func(v verifiedItem) bool { return takenIDs[v.item.Item] })
		diagTake(h, key, target.key.Task, taken, left, paused)
	}
	if len(taken) == 0 {
		return routedTarget{}, nil, false
	}
	d.removeHeldItems(key, takenIDs)
	return target, taken, true
}

// itemsToGo is the held items that may go now. While a pre-emption is open
// only jump items do: nothing else comes between the interrupted turn of a
// session and its turn of its own.
func itemsToGo(queue []verifiedItem, jumpsOnly bool) []verifiedItem {
	if !jumpsOnly {
		return queue
	}
	var jumps []verifiedItem
	for _, v := range queue {
		if v.item.Jump {
			jumps = append(jumps, v)
		}
	}
	return jumps
}

func selectEligibleBatch(h *Host, ai aiInfo, queue []verifiedItem, readPause func() bool) (routedTarget, []verifiedItem, map[int64]bool) {
	groupPaused := readPause()
	var target routedTarget
	var taken []verifiedItem
	takenIDs := map[int64]bool{}
	for _, v := range queue {
		cand := routeSessionItem(h, ai, v)
		if (len(taken) == 0 || cand == target) && !isItemHeld(h, cand, v, groupPaused) {
			target = cand
			taken = append(taken, v)
			takenIDs[v.item.Item] = true
		}
	}
	return target, taken, takenIDs
}

func setQueue[K comparable](m map[K][]verifiedItem, key K, rest []verifiedItem) {
	if len(rest) == 0 {
		delete(m, key)
		return
	}
	m[key] = rest
}

func (d *delivery) prependHeld(key aiGroupKey, items []verifiedItem) {
	d.mu.Lock()
	defer d.mu.Unlock()
	setQueue(d.held, key, append(items, d.held[key]...))
}

// isItemHeld reports whether an item is held for its session
// (flows/agent-turn.md step 3). A turn the AI is running holds every item, a
// jump item too: jumpRunningTurn interrupts for that one. The takeover is
// read before the running turn, so that a hand-back, which goes from the one
// to the other, is seen as one of them.
func isItemHeld(h *Host, target routedTarget, v verifiedItem, paused bool) bool {
	return heldBesidesTurn(h, target, v, paused) || aiWorking(h, target.key)
}

func heldBesidesTurn(h *Host, target routedTarget, v verifiedItem, paused bool) bool {
	return paused || taskIsHeld(h, v) || turnsHeld(h, target.key)
}

func aiWorking(h *Host, key SessionKey) bool {
	return aiHasState(h, aiGroupKey{group: key.Group, member: key.Member}, Working)
}

// turnsHeld reports whether a session gets no turn for now: a hold of Holds
// is on it, or its AI is taken over.
func turnsHeld(h *Host, key SessionKey) bool {
	if _, held := h.Held(key); held {
		return true
	}
	return aiHasState(h, aiGroupKey{group: key.Group, member: key.Member}, Takeover)
}

// QueuedCount returns how many items are held for an AI in a group: the
// queued count of host.info (contracts/datachannel.md section 7).
func (h *Host) QueuedCount(group, member string) int {
	d := deliv(h)
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.held[aiGroupKey{group: group, member: member}])
}

func groupIsPaused(h *Host, group string) bool {
	grp, ok := lookupRow[proto.GroupRow](h, group, "group", group)
	return ok && grp.Paused != nil
}

func taskIsHeld(h *Host, v verifiedItem) bool {
	if v.item.Why == "notice" {
		return false
	}
	return taskStopped(h, v.group, threadTaskID(v))
}

// taskStopped reports whether a task is paused or failed: its sessions get
// no turn (contracts/task-machine.md section 4).
func taskStopped(h *Host, group, taskID string) bool {
	task, ok := lookupTaskRow(h, group, taskID)
	return ok && (task.State == "paused" || task.State == "failed")
}

func aiHasState(h *Host, key aiGroupKey, want State) bool {
	for _, s := range h.Sessions.All() {
		if s.Group == key.group && s.Member == key.member && s.State == want {
			return true
		}
	}
	return false
}

func deliverTargetBatch(h *Host, d *delivery, key aiGroupKey, ai aiInfo, target routedTarget, batch []verifiedItem) bool {
	current, consumed := filterStillCurrent(h, d, key, ai.agent, batch)
	if len(current) == 0 {
		return consumed
	}
	session, gitErr, err := prepareTargetSession(h, ai, target, current[0].home, current)
	if gitErr != nil && target.kind != GroupSession {
		return failTargetSession(h, d, key, ai.agent, target, current, gitErr)
	}
	if gitErr != nil || err != nil {
		d.prependHeld(key, current)
		return false
	}
	return runSessionTurn(h, d, key, ai, session, current)
}

func filterStillCurrent(h *Host, d *delivery, key aiGroupKey, agent string, batch []verifiedItem) ([]verifiedItem, bool) {
	var kept, held []verifiedItem
	for _, v := range batch {
		kept, held = classifyBatchEntry(h, agent, v, kept, held)
	}
	d.prependHeld(key, held)
	return kept, len(held) != len(batch)
}

func classifyBatchEntry(h *Host, agent string, v verifiedItem, kept, held []verifiedItem) ([]verifiedItem, []verifiedItem) {
	if seen, _ := hasAuditItem(h.Store, v.home, v.item.Item); seen {
		sendAck(h, v.home, v.group, v.item.Item)
		return kept, held
	}
	reason, hold := checkStillCurrent(h, v)
	if hold {
		return kept, append(held, v)
	}
	if reason != "" {
		refuseItem(h, v, agent, reason)
		return kept, held
	}
	return append(kept, v), held
}

func checkStillCurrent(h *Host, v verifiedItem) (string, bool) {
	if !isTaskWorkWhy(v.item.Why) && v.item.Why != "review" && v.item.Why != "merge" {
		return "", false
	}
	task, ok := lookupTaskRow(h, v.group, string(v.item.Task))
	if !ok {
		return "", true
	}
	if !matchesTaskRole(task, v.item) {
		return "stale_task", false
	}
	return "", false
}

func matchesTaskRole(task proto.TaskRow, it proto.Item) bool {
	if task.State != "active" {
		return false
	}
	ai := string(it.AI)
	switch it.Why {
	case "review":
		return matchesStepCommit(task.Reviewer == ai, task, "review", it.Commit)
	case "merge":
		return matchesStepCommit(task.Assignee == ai, task, "merge", it.Commit)
	default:
		return task.Assignee == ai
	}
}

func matchesStepCommit(inRole bool, task proto.TaskRow, step, commit string) bool {
	return inRole && task.Step == step && task.Commit == commit
}

func prepareTargetSession(h *Host, ai aiInfo, target routedTarget, home string, batch []verifiedItem) (Session, error, error) {
	ctx := context.Background()
	grp, _ := lookupRow[proto.GroupRow](h, target.key.Group, "group", target.key.Group)
	pDir := projectDir(h, target.key.Group)
	if err := ensureCloneIfConfigured(ctx, pDir, grp); err != nil {
		return Session{}, err, nil
	}
	existing, exists := h.Sessions.Get(target.key)
	folder, branchToReport, err := prepareSessionFolder(ctx, h, pDir, grp, target, exists, existing, batch)
	if err != nil {
		return Session{}, err, nil
	}
	session, err := ensureSessionRow(h, ai, target, folder, home, exists, existing)
	if err != nil {
		return Session{}, nil, err
	}
	if err := reportInitialBranch(ctx, h, home, target.key.Group, target.key.Task, branchToReport); err != nil {
		return Session{}, nil, err
	}
	return session, nil, nil
}

func ensureSessionRow(h *Host, ai aiInfo, target routedTarget, folder, home string, exists bool, existing Session) (Session, error) {
	if exists {
		return existing, nil
	}
	s := Session{SessionKey: target.key, Kind: target.kind, Folder: folder, Agent: ai.agent, Home: home, Name: ai.name, Secretary: ai.secretary}
	if err := h.Sessions.Create(s); err != nil {
		log.Printf("host: create session %v: %v", target.key, err)
		return Session{}, err
	}
	existing, exists = h.Sessions.Get(target.key)
	return existing, nil
}

func ensureCloneIfConfigured(ctx context.Context, projectDir string, grp proto.GroupRow) error {
	if projectDir == "" || grp.Repo == "" {
		return nil
	}
	_, err := EnsureGroupWorkspace(ctx, projectDir, grp.Repo, grp.Base)
	return err
}

func hasGitClone(projectDir string) bool {
	if projectDir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(projectDir, "repo", ".git"))
	return err == nil
}

func prepareSessionFolder(ctx context.Context, h *Host, pDir string, grp proto.GroupRow, target routedTarget, exists bool, existing Session, batch []verifiedItem) (string, string, error) {
	remote := grp.Forge != "none"
	switch target.kind {
	case TaskSession:
		return prepareTaskFolder(ctx, h, pDir, grp.Base, remote, target.key, exists, existing.Folder)
	case ReviewSession:
		return prepareReviewFolder(ctx, h, pDir, grp.Base, remote, target.key, batch)
	default:
		return prepareGroupFolder(ctx, pDir, grp.Base, remote, exists, existing.State)
	}
}

func prepareGroupFolder(ctx context.Context, pDir, base string, remote, exists bool, state State) (string, string, error) {
	if hasGitClone(pDir) && (!exists || state == Dormant) {
		if err := RefreshGroupWorkspace(ctx, pDir, base, remote); err != nil {
			return "", "", err
		}
	}
	return "repo", "", nil
}

func prepareTaskFolder(ctx context.Context, h *Host, pDir, base string, remote bool, key SessionKey, exists bool, folder string) (string, string, error) {
	task, _ := lookupTaskRow(h, key.Group, key.Task)
	name := strings.TrimPrefix(folder, "wt/")
	if !exists {
		var err error
		if name, err = taskName(task.Num, string(task.Slug)); err != nil {
			return "", "", err
		}
		if hasGitClone(pDir) {
			if _, err := CreateTaskWorkspace(ctx, pDir, task.Num, string(task.Slug), key.Member, base, remote); err != nil {
				return "", "", err
			}
		}
		folder = "wt/" + name
	}
	if task.Step == "branch" {
		return folder, "task/" + name, nil
	}
	return folder, "", nil
}

func prepareReviewFolder(ctx context.Context, h *Host, pDir, base string, remote bool, key SessionKey, batch []verifiedItem) (string, string, error) {
	commit := ""
	for _, v := range batch {
		if v.item.Why == "review" {
			commit = v.item.Commit
		}
	}
	task, _ := lookupTaskRow(h, key.Group, key.Task)
	folder := "review/" + strconv.Itoa(task.Num)
	if hasGitClone(pDir) && commit != "" {
		_, err := PrepareReviewWorkspace(ctx, pDir, task.Num, key.Member, commit, base, remote)
		return folder, "", err
	}
	return folder, "", nil
}

func reportInitialBranch(ctx context.Context, h *Host, home, group, taskID, branch string) error {
	if branch == "" {
		return nil
	}
	cmd := proto.TaskBranch{TaskCommand: hostTaskCommand(h, group, "branch", taskID), Branch: branch}
	return sendHostCommand(ctx, h, home, marshalWire(cmd))
}

func failTargetSession(h *Host, d *delivery, key aiGroupKey, agent string, target routedTarget, batch []verifiedItem, gitErr error) bool {
	log.Printf("host: prepare workspace %v: %v", target.key, gitErr)
	if err := commitTurnItems(h, agent, batch); err != nil {
		log.Printf("host: commit failed-workspace items %v: %v", target.key, err)
		d.prependHeld(key, batch)
		return false
	}
	ackBatch(h, batch)
	reportWorkspaceFailure(context.Background(), h, batch[0].home, target, RedactSecrets(gitErr.Error()))
	return true
}

func reportWorkspaceFailure(ctx context.Context, h *Host, home string, target routedTarget, detail string) {
	state := "dormant"
	if aiHasState(h, aiGroupKey{group: target.key.Group, member: target.key.Member}, Waiting) {
		state = "waiting"
	}
	var path string
	if conn, ok := h.Home(home); ok {
		path = conn.Path()
	}
	hc := homeHeader(h, target.key.Group, "", "member")
	mc := proto.MemberCommand{HomeCommand: hc, Op: "status", ID: proto.ID(target.key.Member)}
	ev := &proto.TurnInterruptedEvent{SystemCode: proto.SystemCode{Code: "turn_interrupted"}, Reason: "crash", Task: proto.ID(target.key.Task), Detail: detail}
	sendHostCommand(ctx, h, home, marshalWire(proto.MemberStatus[proto.TurnInterruptedEvent]{MemberCommand: mc, State: state, Path: path, Event: ev}))
	if target.kind == TaskSession {
		fail := proto.TaskFail{TaskCommand: hostTaskCommand(h, target.key.Group, "fail", target.key.Task), Reason: "crash", Detail: detail}
		sendHostCommand(ctx, h, home, marshalWire(fail))
	}
}

func sendHostCommand(ctx context.Context, h *Host, home, payload string) error {
	cmdCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	_, err := h.Command(cmdCtx, home, payload)
	if err != nil {
		log.Printf("host: command to %s: %v", home, err)
	}
	return err
}

func runSessionTurn(h *Host, d *delivery, key aiGroupKey, ai aiInfo, session Session, batch []verifiedItem) bool {
	rawItems, jumping := collectRawItems(batch)
	text, turn, err := Compose(h, session, ItemsTurn, rawItems)
	if err != nil {
		log.Printf("host: compose %v: %v", session.SessionKey, err)
		d.prependHeld(key, batch)
		return false
	}
	if jumping {
		turn.Jump = true
	}
	if err := commitTurnItems(h, ai.agent, batch); err != nil {
		log.Printf("host: commit delivered %v: %v", session.SessionKey, err)
		d.prependHeld(key, batch)
		return false
	}
	ackBatch(h, batch)
	if err := Runner.Turn(context.Background(), h, session.SessionKey, text, turn); err != nil {
		log.Printf("host: turn %v: %v", session.SessionKey, err)
	}
	return true
}

func collectRawItems(batch []verifiedItem) ([]json.RawMessage, bool) {
	raw := make([]json.RawMessage, len(batch))
	jumping := false
	for i, v := range batch {
		raw[i] = v.raw
		jumping = jumping || bool(v.item.Jump)
	}
	return raw, jumping
}

func commitTurnItems(h *Host, agent string, batch []verifiedItem) error {
	run := func(tx *store.Tx) error {
		for _, v := range batch {
			row := AuditRow{Group: v.group, Agent: agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: v.item.Why, By: v.home}
			if _, err := h.Audit(tx, row); err != nil {
				return err
			}
		}
		return nil
	}
	return h.Store.Write(run)
}

func ackBatch(h *Host, batch []verifiedItem) {
	for _, v := range batch {
		sendAck(h, v.home, v.group, v.item.Item)
	}
}

func sendAck(h *Host, home, group string, item int64) {
	if conn, ok := h.Home(home); ok {
		h.diag.Info("ack.send", "g", group, "home", diagDevice(home), "item", item)
		conn.Send(proto.Ack{Type: "ack", Group: proto.ID(group), Item: item})
	}
}

func hasAuditItem(db *store.Store, home string, item int64) (bool, error) {
	var exists bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT EXISTS(SELECT 1 FROM audit WHERE home = ? AND item = ?)", home, item).Scan(&exists)
	}
	err := db.Write(read)
	return exists, err
}

func recordAudit(h *Host, row AuditRow) error {
	write := func(tx *store.Tx) error {
		_, err := h.Audit(tx, row)
		return err
	}
	err := h.Store.Write(write)
	if err != nil {
		log.Printf("host: write audit %s: %v", row.What, err)
	}
	return err
}

func refuseItem(h *Host, v verifiedItem, agent, detail string) {
	row := AuditRow{Group: v.group, Agent: agent, What: AuditRefused, Item: v.item.Item, Home: v.home, Detail: detail, By: v.home}
	if recordAudit(h, row) == nil {
		sendAck(h, v.home, v.group, v.item.Item)
	}
}

func mergeHeldNow(h *Host, d *delivery, group, member string) bool {
	if d.groupSyncActive(group) || groupIsPaused(h, group) {
		return true
	}
	reason, held := h.Held(SessionKey{Group: group, Member: member})
	return held && reason == "offline"
}

func stepMerges(h *Host, d *delivery, group string) {
	d.mu.Lock()
	pending := slices.Clone(d.merges[group])
	d.mu.Unlock()
	for _, v := range pending {
		if !mergeHeldNow(h, d, group, string(v.item.AI)) && d.takeQueuedMerge(group, v.item.Item) {
			takeOneMerge(h, d, v)
		}
	}
}

func (d *delivery) takeQueuedMerge(group string, item int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	match := func(v verifiedItem) bool { return v.item.Item == item }
	idx := slices.IndexFunc(d.merges[group], match)
	if idx < 0 {
		return false
	}
	setQueue(d.merges, group, slices.Delete(d.merges[group], idx, idx+1))
	return true
}

func takeOneMerge(h *Host, d *delivery, v verifiedItem) {
	ai, local, present := lookupLocalAI(h, v.group, string(v.item.AI))
	if !local {
		if present {
			refuseItem(h, v, "", "not_hosted_here")
		} else {
			d.enqueueMerge(v)
		}
		return
	}
	reason, hold := checkStillCurrent(h, v)
	if hold {
		d.enqueueMerge(v)
		return
	}
	if reason != "" {
		refuseItem(h, v, ai.agent, reason)
		return
	}
	runClaimedMerge(h, d, ai.agent, v)
}

func runClaimedMerge(h *Host, d *delivery, agent string, v verifiedItem) {
	task, _ := lookupTaskRow(h, v.group, string(v.item.Task))
	mKey := v.group + "\x00" + task.ID + "\x00" + v.item.Commit
	if !d.claimMerge(mKey) {
		d.enqueueMerge(v)
		return
	}
	written, err := recordMergeTaken(h, agent, v, task.ID)
	if err != nil {
		d.unclaimMerge(mKey)
		log.Printf("host: audit merge item %d: %v", v.item.Item, err)
		d.enqueueMerge(v)
		return
	}
	sendAck(h, v.home, v.group, v.item.Item)
	if written {
		runReviewedMerge(h, v.home, v.group, task, v.item.Commit, v.item.PR, false, time.Time{})
	}
	d.unclaimMerge(mKey)
	stepMerges(h, d, v.group)
}

func (d *delivery) claimMerge(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.merging[key] {
		return false
	}
	d.merging[key] = true
	return true
}

func (d *delivery) unclaimMerge(key string) {
	d.mu.Lock()
	delete(d.merging, key)
	d.mu.Unlock()
}

func recordMergeTaken(h *Host, agent string, v verifiedItem, taskID string) (bool, error) {
	var written bool
	write := func(tx *store.Tx) error {
		delivRow := AuditRow{Group: v.group, Agent: agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: "merge", By: v.home}
		var err error
		written, err = h.Audit(tx, delivRow)
		if err != nil || !written {
			return err
		}
		_, err = h.Audit(tx, AuditRow{Group: v.group, Agent: agent, What: AuditMerge, Detail: taskID + " " + v.item.Commit})
		return err
	}
	err := h.Store.Write(write)
	return written, err
}

func (d *delivery) groupHome(group string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if gs := d.groups[group]; gs != nil {
		return gs.home
	}
	return ""
}

func resumeGroupMerges(h *Host, d *delivery, group string) {
	home := d.groupHome(group)
	if home == "" {
		return
	}
	for _, task := range liveTasks(h, group) {
		if task.State == "active" && task.Step == "merge" {
			resumeOneMerge(h, d, home, group, task)
		}
	}
}

func resumeGroupHostedChecks(h *Host, d *delivery, group string) {
	home := d.groupHome(group)
	if home == "" {
		return
	}
	for _, hc := range HostedChecksToResume(liveTasks(h, group), localGroupAIs(h, group)) {
		startHostedCheck(h, home, group, hc)
	}
}

func localGroupAIs(h *Host, group string) []string {
	var ais []string
	for _, raw := range h.Rows.Of(group, "member") {
		var m proto.AIMemberRow
		proto.Decode(raw, &m)
		if m.Host == h.Key.ID() {
			ais = append(ais, m.ID)
		}
	}
	return ais
}

func resumeOneMerge(h *Host, d *delivery, home, group string, task proto.TaskRow) {
	_, local, _ := lookupLocalAI(h, group, task.Assignee)
	if !local || mergeHeldNow(h, d, group, task.Assignee) {
		return
	}
	ts, found := findAuditMergeTS(h.Store, group, task.ID+" "+task.Commit)
	if !found {
		return
	}
	mKey := group + "\x00" + task.ID + "\x00" + task.Commit
	if !d.claimMerge(mKey) {
		return
	}
	runReviewedMerge(h, home, group, task, task.Commit, task.PR, true, time.UnixMilli(ts))
	d.unclaimMerge(mKey)
	stepMerges(h, d, group)
}

func findAuditMergeTS(db *store.Store, group, detail string) (int64, bool) {
	var ts int64
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT ts FROM audit WHERE g = ? AND what = ? AND detail = ? ORDER BY id DESC LIMIT 1), 0)",
			group, AuditMerge, detail).Scan(&ts)
	}
	db.Write(read)
	return ts, ts != 0
}

func runReviewedMerge(h *Host, home, group string, task proto.TaskRow, commit string, pr int, inspectOnly bool, taken time.Time) {
	ctx := context.Background()
	grp, _ := lookupRow[proto.GroupRow](h, group, "group", group)
	pDir := projectDir(h, group)
	if pDir == "" {
		return
	}
	if err := ensureCloneIfConfigured(ctx, pDir, grp); err != nil {
		log.Printf("host: ensure merge clone %s: %v", group, err)
		return
	}
	rm := buildReviewedMerge(pDir, grp, task, commit, pr, taken)
	var res MergeResult
	var err error
	if inspectOnly {
		res, err = rm.Inspect(ctx)
	} else {
		res, err = rm.Apply(ctx)
	}
	if err != nil {
		log.Printf("host: merge %s %s: %v", group, task.ID, err)
		return
	}
	reportMergeOutcome(ctx, h, home, group, task, commit, res)
}

func buildReviewedMerge(pDir string, grp proto.GroupRow, task proto.TaskRow, commit string, pr int, taken time.Time) ReviewedMerge {
	var branch string
	if name, err := taskName(task.Num, string(task.Slug)); err == nil {
		branch = "task/" + name
	}
	var forgePR int
	var ghRepo string
	if grp.Forge != "none" {
		forgePR, ghRepo = pr, githubRepo(grp.Repo)
	}
	return ReviewedMerge{
		Repo: filepath.Join(pDir, "repo"), Base: grp.Base, Branch: branch,
		Task: task.ID, Commit: commit, PR: forgePR, GitHubRepo: ghRepo, Taken: taken, Templates: grp.Templates,
	}
}

func reportMergeOutcome(ctx context.Context, h *Host, home, group string, task proto.TaskRow, commit string, res MergeResult) {
	if res.MergeCommit != "" {
		stats := proto.DiffStats{Added: task.Stats.Added, Removed: task.Stats.Removed}
		sendHostCommand(ctx, h, home, marshalWire(proto.TaskMerged{TaskCommand: hostTaskCommand(h, group, "merged", task.ID), Commit: commit, MergeCommit: res.MergeCommit, Stats: stats}))
		return
	}
	sendHostCommand(ctx, h, home, marshalWire(proto.TaskConflict{TaskCommand: hostTaskCommand(h, group, "conflict", task.ID), Commit: commit, Detail: res.Conflict}))
}

func githubRepo(raw string) string {
	trimmed := strings.TrimSuffix(strings.TrimRight(raw, `/\`), ".git")
	sep := func(r rune) bool { return r == '/' || r == ':' || r == '\\' }
	parts := strings.FieldsFunc(trimmed, sep)
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func hostTaskCommand(h *Host, group, op, taskID string) proto.TaskCommand {
	return tskCommand(h, group, "", op, proto.ID(taskID))
}
