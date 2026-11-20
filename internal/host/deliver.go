package host

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	Register(Attached, "deliver", attachHome)
	Register(Pushes, "inbox", pushInbox)
	Register(Pushes, "rows", pushRows)
	Register(Pushes, "reset", pushReset)
	Provide(&Deliver, deliverNow)
}

type aiGroupKey struct {
	group  string
	member string
}

type groupSync struct {
	home    string
	cursor  int64
	live    bool
	syncing bool
	token   uint64
}

type syncJob struct {
	group string
	since int64
	token uint64
}

type aiInfo struct {
	agent     string
	name      string
	secretary bool
	approvers []string
}

type verifiedItem struct {
	home     string
	group    string
	raw      json.RawMessage
	wire     proto.InboxItem
	item     proto.Item
	msg      *proto.MsgRow
	approval string // "approve" or "deny" for a late approval decision; "" otherwise
}

type heldItem struct {
	verifiedItem
}

// delivery holds the per-host sync cursors, held inbox queues and active merge
// tracking via Part[delivery](h). It creates no channels of its own; mu guards
// only fast in-memory state and is never held across store, session, network,
// runtime or Git calls.
type delivery struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	groups   map[string]*groupSync
	early    map[string][]verifiedItem
	held     map[aiGroupKey][]heldItem
	merges   map[string][]verifiedItem
	merging  map[string]bool
	reported map[string]bool
	busy     map[aiGroupKey]bool
	recheck  map[aiGroupKey]bool
	cancels  map[string]context.CancelFunc
}

func deliv(h *Host) *delivery {
	d := Part[delivery](h)
	d.mu.Lock()
	if d.groups == nil {
		d.groups = map[string]*groupSync{}
		d.early = map[string][]verifiedItem{}
		d.held = map[aiGroupKey][]heldItem{}
		d.merges = map[string][]verifiedItem{}
		d.merging = map[string]bool{}
		d.reported = map[string]bool{}
		d.busy = map[aiGroupKey]bool{}
		d.recheck = map[aiGroupKey]bool{}
		d.cancels = map[string]context.CancelFunc{}
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

type hostConfig struct {
	DBFile    string           `json:"db_file"`
	Owner     string           `json:"owner"`
	Offline   bool             `json:"offline"`
	MaxOpen   int              `json:"max_open"`
	Approvers []string         `json:"approvers"`
	Homes     []proto.Home     `json:"homes"`
	OwnerLog  []proto.Envelope `json:"owner_log"`
	owner     identity.Log
}

const readHostConfigSQL = `SELECT json_object(
	'db_file', coalesce((SELECT file FROM pragma_database_list WHERE name = 'main'), ''),
	'owner', coalesce((SELECT json(value) FROM setting WHERE key = 'owner'), '""'),
	'offline', coalesce((SELECT json(value) FROM setting WHERE key = 'offline'), json('false')),
	'max_open', coalesce((SELECT json(value) FROM setting WHERE key = 'max_open'), 4),
	'approvers', coalesce((SELECT json(value) FROM setting WHERE key = 'approvers'), json('[]')),
	'homes', coalesce((SELECT json(value) FROM setting WHERE key = 'homes'), json('[]')),
	'owner_log', coalesce((SELECT json_group_array(json(env)) FROM (
		SELECT env FROM device_log
		WHERE user = json_extract((SELECT value FROM setting WHERE key = 'owner'), '$')
		ORDER BY ver
	)), json('[]'))
)`

func loadHostConfig(h *Host) (hostConfig, error) {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(readHostConfigSQL).Scan(&raw)
	}
	err := h.Store.Write(read)
	if err != nil {
		return hostConfig{}, err
	}
	var cfg hostConfig
	json.Unmarshal([]byte(raw), &cfg)
	if log, err := identity.Replay(cfg.Owner, cfg.OwnerLog); err == nil {
		cfg.owner = *log
	}
	return cfg, nil
}

func (cfg hostConfig) projectDir(group string) string {
	if cfg.DBFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.DBFile), "projects", group)
}

func attachHome(h *Host, device string) {
	conn, ok := h.Home(device)
	if !ok {
		return
	}
	cfg, err := loadHostConfig(h)
	if err != nil {
		return
	}
	d := deliv(h)
	groups := groupsForHome(h, d, cfg, device)
	if len(groups) == 0 {
		return
	}
	ctx, jobs := d.beginHomeSync(device, groups)
	d.wg.Add(1)
	go syncHomeGroups(ctx, h, d, conn, device, jobs)
}

func groupsForHome(h *Host, d *delivery, cfg hostConfig, device string) []string {
	seen := map[string]bool{}
	d.mu.Lock()
	for g, gs := range d.groups {
		if gs.home == device {
			seen[g] = true
		}
	}
	d.mu.Unlock()
	collectStoredHomeGroups(h, cfg.Homes, device, seen)
	return slices.Sorted(maps.Keys(seen))
}

func collectStoredHomeGroups(h *Host, homes []proto.Home, device string, seen map[string]bool) {
	for _, s := range h.Sessions.All() {
		if s.Home == device {
			seen[s.Group] = true
		}
	}
	for _, entry := range homes {
		if entry.Home == device {
			seen[string(entry.Group)] = true
		}
	}
}

func (d *delivery) beginHomeSync(device string, groups []string) (context.Context, []syncJob) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if prev := d.cancels[device]; prev != nil {
		prev()
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancels[device] = cancel
	jobs := make([]syncJob, 0, len(groups))
	for _, g := range groups {
		gs := d.groupLocked(g, device)
		gs.token++
		gs.syncing = true
		gs.live = false
		jobs = append(jobs, syncJob{group: g, since: gs.cursor, token: gs.token})
	}
	return ctx, jobs
}

func syncHomeGroups(ctx context.Context, h *Host, d *delivery, conn Home, device string, jobs []syncJob) {
	defer d.wg.Done()
	for _, job := range jobs {
		syncOneGroup(ctx, h, d, conn, device, job)
	}
}

func syncOneGroup(ctx context.Context, h *Host, d *delivery, conn Home, device string, job syncJob) {
	build := func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: job.group, Since: job.since, StateOnly: true}
	}
	answer, err := conn.Request(ctx, build)
	var synced proto.Synced
	ok := err == nil && proto.Decode(answer, &synced) == nil && synced.Group == job.group
	if d.finishGroupSync(job, device, synced.Seq, ok) && ok {
		afterGroupState(h, d, job.group)
	}
}

func (d *delivery) finishGroupSync(job syncJob, device string, seq int64, ok bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groupLocked(job.group, device)
	if gs.token != job.token {
		return false
	}
	gs.syncing = false
	if ok {
		gs.cursor = seq
		gs.live = true
	}
	return true
}

func knownHome(h *Host, d *delivery, cfg hostConfig, group string) string {
	d.mu.Lock()
	gs := d.groups[group]
	var home string
	if gs != nil {
		home = gs.home
	}
	d.mu.Unlock()
	if home != "" {
		return home
	}
	return storedGroupHome(h, cfg, group)
}

func storedGroupHome(h *Host, cfg hostConfig, group string) string {
	if grp, ok := lookupGroupRow(h, group); ok {
		return grp.Home
	}
	for _, s := range h.Sessions.All() {
		if s.Group == group {
			return s.Home
		}
	}
	for _, entry := range cfg.Homes {
		if string(entry.Group) == group {
			return entry.Home
		}
	}
	return ""
}

func acceptGroupPush(h *Host, d *delivery, home, group string) (hostConfig, bool) {
	if group == "" {
		return hostConfig{}, false
	}
	cfg, err := loadHostConfig(h)
	if err != nil {
		return hostConfig{}, false
	}
	known := knownHome(h, d, cfg, group)
	if known != "" {
		return cfg, known == home && cfg.owner.Valid(home)
	}
	return cfg, cfg.owner.Valid(home)
}

func pushReset(h *Host, home string, frame json.RawMessage) {
	var rst proto.Reset
	if proto.Decode(frame, &rst) != nil {
		return
	}
	d := deliv(h)
	if _, ok := acceptGroupPush(h, d, home, rst.Group); !ok {
		return
	}
	h.Rows.Reset(rst.Group)
	d.mu.Lock()
	gs := d.groupLocked(rst.Group, home)
	gs.cursor = 0
	gs.live = false
	d.mu.Unlock()
}

func pushRows(h *Host, home string, frame json.RawMessage) {
	var batch proto.Rows
	if proto.Decode(frame, &batch) != nil {
		return
	}
	d := deliv(h)
	if _, ok := acceptGroupPush(h, d, home, batch.Group); !ok {
		return
	}
	for _, raw := range batch.Rows {
		applyGroupRow(h, d, home, batch.Group, raw)
	}
	if !d.isGroupSyncing(batch.Group, home) {
		afterGroupState(h, d, batch.Group)
	}
}

func (d *delivery) isGroupSyncing(group, home string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.groupLocked(group, home).syncing
}

func applyGroupRow(h *Host, d *delivery, home, group string, raw json.RawMessage) {
	hdr, ok := decodeRowHeader(raw)
	if !ok {
		return
	}
	if hdr.Type == "group" && !validGroupRowHome(raw, group, home) {
		return
	}
	if !newerRowSeq(h.Rows, group, hdr) {
		return
	}
	h.Rows.Put(group, hdr.Type, hdr.ID, raw)
	d.advanceLiveCursor(group, home, hdr.Seq)
}

func decodeRowHeader(raw json.RawMessage) (proto.RowHeader, bool) {
	var hdr proto.RowHeader
	ok := proto.Decode(raw, &hdr) == nil && hdr.Type != "" && hdr.ID != "" && hdr.Seq >= 1
	return hdr, ok
}

func validGroupRowHome(raw json.RawMessage, group, home string) bool {
	var grp proto.GroupRow
	return proto.Decode(raw, &grp) == nil && grp.ID == group && grp.Home == home
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
		gs.cursor = seq
	}
}

func afterGroupState(h *Host, d *delivery, group string) {
	cfg, err := loadHostConfig(h)
	if err != nil {
		return
	}
	flushEarlyItems(h, d, cfg, group)
	reconcileGroup(h, cfg, group)
	stepMerges(h, d, cfg, group)
	resumeGroupMerges(h, d, cfg, group)
	for _, key := range d.groupHeldKeys(group) {
		deliverPair(h, d, key)
	}
}

func flushEarlyItems(h *Host, d *delivery, cfg hostConfig, group string) {
	d.mu.Lock()
	items := d.early[group]
	delete(d.early, group)
	d.mu.Unlock()
	for _, v := range items {
		routeVerifiedItem(h, d, cfg, v)
	}
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
	cfg, err := loadHostConfig(h)
	if err != nil {
		return
	}
	d := deliv(h)
	group := string(outer.Group)
	for _, rawElem := range outer.Items {
		handleInboxElem(h, d, cfg, home, group, rawElem)
	}
}

func handleInboxElem(h *Host, d *delivery, cfg hostConfig, home, group string, rawElem json.RawMessage) {
	var wire proto.InboxItem
	if proto.Decode(rawElem, &wire) != nil {
		return
	}
	v, reason := verifyBoundary(h, d, cfg, home, group, rawElem, wire)
	if reason != "" {
		refuseItem(h, home, group, "", v.item.Item, wire.Env.Key, reason)
		return
	}
	seen, err := hasAuditItem(h.Store, home, v.item.Item)
	if err != nil {
		return
	}
	if seen {
		sendAck(h, home, group, v.item.Item)
		return
	}
	dispatchArrivedItem(h, d, cfg, v)
}

func dispatchArrivedItem(h *Host, d *delivery, cfg hostConfig, v verifiedItem) {
	if ai, ok := lookupLocalAI(h, v.group, string(v.item.AI)); ok {
		if tryOpenWait(h, v, ai.agent) {
			return
		}
	}
	if d.holdIfSyncing(v) {
		return
	}
	routeVerifiedItem(h, d, cfg, v)
}

func (d *delivery) holdIfSyncing(v verifiedItem) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groups[v.group]
	if gs == nil || !gs.syncing {
		return false
	}
	d.early[v.group] = append(d.early[v.group], v)
	return true
}

func verifyBoundary(h *Host, d *delivery, cfg hostConfig, home, group string, rawElem json.RawMessage, wire proto.InboxItem) (verifiedItem, string) {
	v := verifiedItem{home: home, group: group, raw: rawElem, wire: wire}
	json.Unmarshal([]byte(wire.Env.Payload), &v.item)
	expected := knownHome(h, d, cfg, group)
	if wire.Env.Key != home || (expected != "" && expected != home) {
		return v, "wrong_home"
	}
	if !openItemEnvelope(h, cfg, wire.Env, v.item.Timestamp) {
		return v, "invalid_envelope"
	}
	return checkParsedItem(group, v)
}

func openItemEnvelope(h *Host, cfg hostConfig, env proto.Envelope, ts int64) bool {
	_, err := identity.OpenCommand(env, h.Key.ID(), cfg.owner.Valid, time.UnixMilli(ts))
	return err == nil
}

func checkParsedItem(group string, v verifiedItem) (verifiedItem, string) {
	var strict proto.Item
	if proto.Decode([]byte(v.wire.Env.Payload), &strict) != nil {
		return v, "bad_payload"
	}
	v.item = strict
	if !validEnvelopeTarget(group, strict) || !validItemShape(strict, v.wire) {
		return v, "bad_item"
	}
	msg, ok := parseOptionalMsg(v.wire.Msg)
	if !ok {
		return v, "bad_msg"
	}
	v.msg = msg
	return v, ""
}

func validEnvelopeTarget(group string, it proto.Item) bool {
	return it.Type == "item" && it.Item >= 1 && string(it.Group) == group
}

func validItemShape(it proto.Item, wire proto.InboxItem) bool {
	switch it.Why {
	case "mention", "reply":
		return it.Msg != ""
	case "assign", "resume":
		return it.Task != ""
	case "review", "merge":
		return it.Task != "" && it.Commit != ""
	default:
		return validSecondaryShape(it, wire)
	}
}

func validSecondaryShape(it proto.Item, wire proto.InboxItem) bool {
	switch it.Why {
	case "reviewed", "checks":
		return it.Task != "" && it.Msg != ""
	case "decided", "confirmed":
		return it.Msg != "" && wire.Cause != nil
	default:
		return it.Why == "notice" && it.Code != ""
	}
}

func parseOptionalMsg(raw json.RawMessage) (*proto.MsgRow, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	var msg proto.MsgRow
	if proto.Decode(raw, &msg) != nil {
		return nil, false
	}
	return &msg, true
}

func waitTargetID(v verifiedItem) string {
	if v.item.Why == "decided" {
		return string(v.item.Msg)
	}
	if v.item.Why == "reply" {
		return replyCallID(string(v.item.Msg), v.msg)
	}
	return ""
}

func replyCallID(msgID string, msg *proto.MsgRow) string {
	if msg != nil && msg.ID == msgID && msg.Kind == "reply" && msg.Re != nil {
		return *msg.Re
	}
	return ""
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
	finishHandedItem(h, v, agent, err)
	return true
}

func finishHandedItem(h *Host, v verifiedItem, agent string, handErr error) {
	by := itemSigner(v)
	if handErr != nil {
		refuseItem(h, v.home, v.group, agent, v.item.Item, by, handErr.Error())
		return
	}
	row := AuditRow{Group: v.group, Agent: agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: v.item.Why, By: by}
	if recordAudit(h, row) == nil {
		sendAck(h, v.home, v.group, v.item.Item)
	}
}

func itemSigner(v verifiedItem) string {
	if v.wire.Cause != nil {
		return v.wire.Cause.Key
	}
	return v.wire.Env.Key
}

func routeVerifiedItem(h *Host, d *delivery, cfg hostConfig, v verifiedItem) {
	ai, ok := lookupLocalAI(h, v.group, string(v.item.AI))
	if !ok {
		refuseItem(h, v.home, v.group, "", v.item.Item, v.wire.Env.Key, "not_hosted_here")
		return
	}
	if tryOpenWait(h, v, ai.agent) {
		return
	}
	action, ok := checkApprovalDecision(h, cfg, ai, v)
	if !ok {
		refuseItem(h, v.home, v.group, ai.agent, v.item.Item, itemSigner(v), "invalid_approval_cause")
		return
	}
	v.approval = action
	enqueueVerified(h, d, cfg, v)
}

func enqueueVerified(h *Host, d *delivery, cfg hostConfig, v verifiedItem) {
	if v.item.Why == "merge" {
		d.enqueueMerge(v)
		stepMerges(h, d, cfg, v.group)
		return
	}
	key := aiGroupKey{group: v.group, member: string(v.item.AI)}
	d.mu.Lock()
	d.held[key] = enqueueHeld(d.held[key], heldItem{verifiedItem: v})
	d.mu.Unlock()
	deliverNow(h, key.group, key.member)
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

func enqueueHeld(queue []heldItem, incoming heldItem) []heldItem {
	match := func(e heldItem) bool { return e.item.Item == incoming.item.Item }
	idx := slices.IndexFunc(queue, match)
	if idx >= 0 {
		if !bool(incoming.item.Jump) || bool(queue[idx].item.Jump) {
			return queue
		}
		queue = slices.Delete(queue, idx, idx+1)
	}
	return insertHeld(queue, incoming)
}

func insertHeld(queue []heldItem, incoming heldItem) []heldItem {
	if !bool(incoming.item.Jump) {
		return append(queue, incoming)
	}
	nonJump := func(e heldItem) bool { return !bool(e.item.Jump) }
	pos := slices.IndexFunc(queue, nonJump)
	if pos < 0 {
		return append(queue, incoming)
	}
	return slices.Insert(queue, pos, incoming)
}

func checkApprovalDecision(h *Host, cfg hostConfig, ai aiInfo, v verifiedItem) (string, bool) {
	if v.item.Why != "decided" {
		return "", true
	}
	if v.msg != nil && v.msg.Kind != "approval" {
		return "", hasNoPendingApproval(h.Store, string(v.item.Msg), v.group, string(v.item.AI))
	}
	act, ok := verifyApprovalCause(h, cfg, ai, v)
	if !ok {
		return "", false
	}
	return act.Action, hasPendingApproval(h.Store, string(v.item.Msg), v.group, string(v.item.AI))
}

func verifyApprovalCause(h *Host, cfg hostConfig, ai aiInfo, v verifiedItem) (proto.Act, bool) {
	var act proto.Act
	if proto.Decode([]byte(v.wire.Cause.Payload), &act) != nil {
		return act, false
	}
	if !validApprovalAct(act, v) {
		return act, false
	}
	allowed := approverDeviceValidator(h, cfg, ai, v.group)
	_, err := identity.OpenCommand(*v.wire.Cause, h.Key.ID(), allowed, time.UnixMilli(act.Timestamp))
	return act, err == nil
}

func validApprovalAct(act proto.Act, v verifiedItem) bool {
	return act.Type == "act" && string(act.Group) == v.group && act.As == "" && act.Msg == v.item.Msg && validApproveVerb(act)
}

func validApproveVerb(act proto.Act) bool {
	return (act.Action == "approve" || act.Action == "deny") && act.Arg == "" && act.Text == ""
}

func approverDeviceValidator(h *Host, cfg hostConfig, ai aiInfo, group string) func(string) bool {
	users := map[string]bool{}
	for _, u := range cfg.Approvers {
		users[u] = true
	}
	for _, u := range ai.approvers {
		users[u] = true
	}
	logs := groupApproverLogs(h.Rows, group, users)
	return func(dev string) bool {
		return cfg.owner.Valid(dev) || anyLogValid(logs, dev)
	}
}

func anyLogValid(logs []*identity.Log, dev string) bool {
	for _, l := range logs {
		if l.Valid(dev) {
			return true
		}
	}
	return false
}

func groupApproverLogs(rows *Rows, group string, users map[string]bool) []*identity.Log {
	byUser := map[string][]proto.DeviceRow{}
	for _, raw := range rows.Of(group, "device") {
		var dr proto.DeviceRow
		if proto.Decode(raw, &dr) == nil && users[dr.User] {
			byUser[dr.User] = append(byUser[dr.User], dr)
		}
	}
	var out []*identity.Log
	for _, list := range byUser {
		if l := buildUserLog(list); l != nil {
			out = append(out, l)
		}
	}
	return out
}

func buildUserLog(list []proto.DeviceRow) *identity.Log {
	order := func(a, b proto.DeviceRow) int { return cmp.Compare(a.Version, b.Version) }
	slices.SortFunc(list, order)
	envs := make([]proto.Envelope, len(list))
	for i, d := range list {
		envs[i] = d.Env
	}
	l, err := identity.Replay(list[0].User, envs)
	if err != nil {
		return nil
	}
	return l
}

func lookupLocalAI(h *Host, group, member string) (aiInfo, bool) {
	if raw, ok := h.Rows.Get(group, "member", member); ok {
		var m proto.AIMemberRow
		if proto.Decode(raw, &m) == nil && m.Kind == "ai" && m.Host == h.Key.ID() {
			return aiInfo{agent: m.CLI, name: m.Name, secretary: m.Secretary, approvers: m.Approvers}, true
		}
		return aiInfo{}, false
	}
	return lookupSessionAI(h, group, member)
}

func lookupSessionAI(h *Host, group, member string) (aiInfo, bool) {
	for _, s := range h.Sessions.All() {
		if s.Group == group && s.Member == member {
			return aiInfo{agent: s.Agent, name: s.Name, secretary: s.Secretary}, true
		}
	}
	return aiInfo{}, false
}

func lookupGroupRow(h *Host, group string) (proto.GroupRow, bool) {
	raw, ok := h.Rows.Get(group, "group", group)
	if !ok {
		return proto.GroupRow{}, false
	}
	var grp proto.GroupRow
	return grp, proto.Decode(raw, &grp) == nil
}

func lookupTaskRow(h *Host, group, taskID string) (proto.TaskRow, bool) {
	raw, ok := h.Rows.Get(group, "task", taskID)
	if !ok {
		return proto.TaskRow{}, false
	}
	var task proto.TaskRow
	return task, proto.Decode(raw, &task) == nil
}

func deliverNow(h *Host, group, member string) {
	d := deliv(h)
	primary := aiGroupKey{group: group, member: member}
	deliverPair(h, d, primary)
	if cfg, err := loadHostConfig(h); err == nil {
		for _, g := range d.mergeGroups() {
			stepMerges(h, d, cfg, g)
		}
	}
	for _, other := range d.allHeldKeys() {
		if other != primary {
			deliverPair(h, d, other)
		}
	}
}

func (d *delivery) mergeGroups() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	groups := slices.Sorted(maps.Keys(d.merges))
	return groups
}

func deliverPair(h *Host, d *delivery, key aiGroupKey) {
	if !d.enterAI(key) {
		return
	}
	for {
		stepAI(h, d, key)
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

func (d *delivery) allHeldKeys() []aiGroupKey {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := slices.Collect(maps.Keys(d.held))
	slices.SortFunc(out, compareAIKey)
	return out
}

func (d *delivery) groupHeldKeys(group string) []aiGroupKey {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []aiGroupKey
	for k := range d.held {
		if k.group == group {
			out = append(out, k)
		}
	}
	slices.SortFunc(out, compareAIKey)
	return out
}

func compareAIKey(a, b aiGroupKey) int {
	return cmp.Or(cmp.Compare(a.group, b.group), cmp.Compare(a.member, b.member))
}

type routedTarget struct {
	key  SessionKey
	kind Kind
}

func routeSessionItem(h *Host, ai aiInfo, v verifiedItem) routedTarget {
	g, m := v.group, string(v.item.AI)
	if ai.secretary {
		return routedTarget{key: SessionKey{Group: g, Member: m}, kind: GroupSession}
	}
	if isTaskWorkWhy(v.item.Why) || isAssignedTaskDecision(h, g, m, v.item) {
		return routedTarget{key: SessionKey{Group: g, Member: m, Task: string(v.item.Task)}, kind: TaskSession}
	}
	if v.item.Why == "review" {
		return routedTarget{key: SessionKey{Group: g, Member: m, Task: string(v.item.Task)}, kind: ReviewSession}
	}
	return routeThreadOrGroup(h, g, m, v)
}

func isTaskWorkWhy(why string) bool {
	return why == "assign" || why == "checks" || why == "reviewed" || why == "resume"
}

func isAssignedTaskDecision(h *Host, group, member string, it proto.Item) bool {
	if it.Why != "decided" || it.Task == "" {
		return false
	}
	if task, ok := lookupTaskRow(h, group, string(it.Task)); ok {
		return task.Assignee == member
	}
	_, exists := h.Sessions.Get(SessionKey{Group: group, Member: member, Task: string(it.Task)})
	return exists
}

func routeThreadOrGroup(h *Host, group, member string, v verifiedItem) routedTarget {
	if v.item.Why == "mention" || v.item.Why == "reply" {
		thread := threadTaskID(v)
		if thread != "" {
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
	if v.msg != nil && v.msg.Thread != nil {
		return *v.msg.Thread
	}
	return ""
}

func stepAI(h *Host, d *delivery, key aiGroupKey) {
	cfg, err := loadHostConfig(h)
	if err != nil || d.groupSyncActive(key.group) {
		return
	}
	ai, ok := lookupLocalAI(h, key.group, key.member)
	if !ok {
		return
	}
	target, batch, ok := takeEligibleBatch(h, d, cfg, key, ai)
	if !ok {
		return
	}
	deliverTargetBatch(h, d, cfg, key, ai, target, batch)
}

func (d *delivery) groupSyncActive(group string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	gs := d.groups[group]
	return gs != nil && gs.syncing
}

func takeEligibleBatch(h *Host, d *delivery, cfg hostConfig, key aiGroupKey, ai aiInfo) (routedTarget, []heldItem, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	queue := d.held[key]
	target, ok := firstEligibleTarget(h, cfg, ai, queue)
	if !ok {
		return routedTarget{}, nil, false
	}
	var taken, rest []heldItem
	for _, entry := range queue {
		if routeSessionItem(h, ai, entry.verifiedItem) == target && !isItemHeld(h, cfg, target, entry.verifiedItem) {
			taken = append(taken, entry)
		} else {
			rest = append(rest, entry)
		}
	}
	setHeldLocked(d, key, rest)
	return target, taken, true
}

func firstEligibleTarget(h *Host, cfg hostConfig, ai aiInfo, queue []heldItem) (routedTarget, bool) {
	for _, entry := range queue {
		cand := routeSessionItem(h, ai, entry.verifiedItem)
		if !isItemHeld(h, cfg, cand, entry.verifiedItem) {
			return cand, true
		}
	}
	return routedTarget{}, false
}

func setHeldLocked(d *delivery, key aiGroupKey, rest []heldItem) {
	if len(rest) == 0 {
		delete(d.held, key)
		return
	}
	d.held[key] = rest
}

func (d *delivery) prependHeld(key aiGroupKey, items []heldItem) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.held[key] = append(items, d.held[key]...)
}

func isItemHeld(h *Host, cfg hostConfig, target routedTarget, v verifiedItem) bool {
	if cfg.Offline || groupIsPaused(h, v.group) || taskIsHeld(h, v) {
		return true
	}
	if _, held := h.Held(target.key); held {
		return true
	}
	return sessionBusyOrCapped(h, cfg, target.key, bool(v.item.Jump))
}

func groupIsPaused(h *Host, group string) bool {
	grp, ok := lookupGroupRow(h, group)
	return ok && grp.Paused != nil
}

func taskIsHeld(h *Host, v verifiedItem) bool {
	if v.item.Why == "notice" {
		return false
	}
	tid := threadTaskID(v)
	if tid == "" {
		return false
	}
	task, ok := lookupTaskRow(h, v.group, tid)
	return ok && (task.State == "paused" || task.State == "failed")
}

func sessionBusyOrCapped(h *Host, cfg hostConfig, key SessionKey, jump bool) bool {
	aiKey := aiGroupKey{group: key.Group, member: key.Member}
	if aiInTakeover(h, aiKey) {
		return true
	}
	working := aiWorkingInGroup(h, aiKey)
	if working && !jump {
		return true
	}
	return slotUnavailable(h, cfg.MaxOpen, key, jump && working)
}

func aiInTakeover(h *Host, key aiGroupKey) bool {
	for _, s := range h.Sessions.All() {
		if s.Group == key.group && s.Member == key.member && s.State == Takeover {
			return true
		}
	}
	return false
}

func aiWorkingInGroup(h *Host, key aiGroupKey) bool {
	for _, s := range h.Sessions.All() {
		if s.Group == key.group && s.Member == key.member && s.State == Working {
			return true
		}
	}
	return false
}

func slotUnavailable(h *Host, maxOpen int, key SessionKey, interrupting bool) bool {
	if interrupting {
		return false
	}
	if s, ok := h.Sessions.Get(key); ok && s.State != Dormant {
		return false
	}
	open, waiting := countOpenSessions(h)
	return open >= maxOpen && waiting == 0
}

func countOpenSessions(h *Host) (open, waiting int) {
	for _, s := range h.Sessions.All() {
		if s.State != Dormant {
			open++
		}
		if s.State == Waiting {
			waiting++
		}
	}
	return open, waiting
}

func deliverTargetBatch(h *Host, d *delivery, cfg hostConfig, key aiGroupKey, ai aiInfo, target routedTarget, batch []heldItem) {
	current := filterStillCurrent(h, ai.agent, batch)
	if len(current) == 0 {
		return
	}
	session, err := prepareTargetSession(h, cfg, ai, target, current)
	if err != nil {
		d.prependHeld(key, current)
		return
	}
	runSessionTurn(h, d, key, ai, session, current)
}

func filterStillCurrent(h *Host, agent string, batch []heldItem) []heldItem {
	var kept []heldItem
	for _, entry := range batch {
		if reason := checkStillCurrent(h, entry.verifiedItem); reason != "" {
			refuseItem(h, entry.home, entry.group, agent, entry.item.Item, itemSigner(entry.verifiedItem), reason)
		} else {
			kept = append(kept, entry)
		}
	}
	return kept
}

func checkStillCurrent(h *Host, v verifiedItem) string {
	if v.approval != "" && !hasPendingApproval(h.Store, string(v.item.Msg), v.group, string(v.item.AI)) {
		return "stale_approval"
	}
	if !needsTaskCurrentCheck(v.item.Why) {
		return ""
	}
	task, ok := lookupTaskRow(h, v.group, string(v.item.Task))
	if !ok || !matchesTaskRole(task, v.item) {
		return "stale_task"
	}
	return ""
}

func needsTaskCurrentCheck(why string) bool {
	return isTaskWorkWhy(why) || why == "review" || why == "merge"
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

func prepareTargetSession(h *Host, cfg hostConfig, ai aiInfo, target routedTarget, batch []heldItem) (Session, error) {
	ctx := context.Background()
	grp, _ := lookupGroupRow(h, target.key.Group)
	pDir := cfg.projectDir(target.key.Group)
	if err := ensureCloneIfConfigured(ctx, pDir, grp); err != nil {
		return Session{}, err
	}
	return openOrCreateSession(ctx, h, pDir, grp, ai, target, batch)
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

func openOrCreateSession(ctx context.Context, h *Host, pDir string, grp proto.GroupRow, ai aiInfo, target routedTarget, batch []heldItem) (Session, error) {
	existing, exists := h.Sessions.Get(target.key)
	folder, branchToReport, err := prepareSessionFolder(ctx, h, pDir, grp, target, exists, existing, batch)
	if err != nil {
		return Session{}, err
	}
	if !exists {
		existing, err = createSessionRow(h, target, folder, ai, batch[0].home)
		if err != nil {
			return Session{}, err
		}
	}
	reportInitialBranch(ctx, h, batch[0].home, target.key.Group, target.key.Task, branchToReport)
	return existing, nil
}

func prepareSessionFolder(ctx context.Context, h *Host, pDir string, grp proto.GroupRow, target routedTarget, exists bool, existing Session, batch []heldItem) (string, string, error) {
	remote := grp.Forge == "github"
	switch target.kind {
	case TaskSession:
		return prepareTaskFolder(ctx, h, pDir, grp.Base, remote, target.key, exists, existing.Folder)
	case ReviewSession:
		return prepareReviewFolder(ctx, h, pDir, grp.Base, remote, target.key, existing.Folder, batch)
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
	if exists {
		return folder, "", nil
	}
	task, _ := lookupTaskRow(h, key.Group, key.Task)
	name, err := taskName(task.Num, string(task.Slug))
	if err != nil {
		return "", "", err
	}
	if hasGitClone(pDir) {
		if _, err := CreateTaskWorkspace(ctx, pDir, task.Num, string(task.Slug), key.Member, base, remote); err != nil {
			return "", "", err
		}
	}
	return "wt/" + name, initialTaskBranch(task, name), nil
}

func initialTaskBranch(task proto.TaskRow, name string) string {
	if task.Step == "branch" {
		return "task/" + name
	}
	return ""
}

func prepareReviewFolder(ctx context.Context, h *Host, pDir, base string, remote bool, key SessionKey, folder string, batch []heldItem) (string, string, error) {
	commit := latestReviewCommit(batch)
	task, _ := lookupTaskRow(h, key.Group, key.Task)
	folder = reviewFolderName(folder, task.Num)
	if hasGitClone(pDir) && commit != "" {
		_, err := PrepareReviewWorkspace(ctx, pDir, task.Num, key.Member, commit, base, remote)
		return folder, "", err
	}
	return folder, "", nil
}

func reviewFolderName(folder string, num int) string {
	if folder != "" {
		return folder
	}
	return "review/" + strconv.Itoa(num)
}

func latestReviewCommit(batch []heldItem) string {
	commit := ""
	for _, entry := range batch {
		if entry.item.Why == "review" {
			commit = entry.item.Commit
		}
	}
	return commit
}

func createSessionRow(h *Host, target routedTarget, folder string, ai aiInfo, home string) (Session, error) {
	s := Session{
		SessionKey: target.key,
		Kind:       target.kind,
		Folder:     folder,
		Agent:      ai.agent,
		Home:       home,
		Name:       ai.name,
		Secretary:  ai.secretary,
	}
	err := h.Sessions.Create(s)
	if err != nil {
		return Session{}, err
	}
	saved, _ := h.Sessions.Get(target.key)
	return saved, nil
}

func reportInitialBranch(ctx context.Context, h *Host, home, group, taskID, branch string) {
	if branch == "" {
		return
	}
	cmd := proto.TaskBranch{
		TaskCommand: hostTaskCommand(h, group, "branch", taskID),
		Branch:      branch,
	}
	h.Command(ctx, home, encodeJSON(cmd))
}

func runSessionTurn(h *Host, d *delivery, key aiGroupKey, ai aiInfo, session Session, batch []heldItem) {
	rawItems, jumping := collectRawItems(batch)
	text, turn, err := Compose(h, session, ItemsTurn, rawItems)
	if err != nil {
		d.prependHeld(key, batch)
		return
	}
	if jumping {
		turn.Jump = true
	}
	if commitTurnItems(h, ai.agent, batch) != nil {
		d.prependHeld(key, batch)
		return
	}
	ackBatch(h, batch)
	Runner.Turn(context.Background(), h, session.SessionKey, text, turn)
}

func collectRawItems(batch []heldItem) ([]json.RawMessage, bool) {
	raw := make([]json.RawMessage, len(batch))
	jumping := false
	for i, entry := range batch {
		raw[i] = entry.raw
		jumping = jumping || bool(entry.item.Jump)
	}
	return raw, jumping
}

func commitTurnItems(h *Host, agent string, batch []heldItem) error {
	run := func(tx *store.Tx) error {
		for _, entry := range batch {
			if err := writeDeliveredItem(h, tx, agent, entry.verifiedItem); err != nil {
				return err
			}
		}
		return nil
	}
	return h.Store.Write(run)
}

func writeDeliveredItem(h *Host, tx *store.Tx, agent string, v verifiedItem) error {
	row := AuditRow{
		Group:  v.group,
		Agent:  agent,
		What:   AuditDelivered,
		Item:   v.item.Item,
		Home:   v.home,
		Detail: v.item.Why,
		By:     itemSigner(v),
	}
	if _, err := h.Audit(tx, row); err != nil {
		return err
	}
	return applyApprovalDecisionTx(tx, v)
}

func applyApprovalDecisionTx(tx *store.Tx, v verifiedItem) error {
	if v.approval == "" {
		return nil
	}
	if v.approval == "approve" {
		_, err := tx.Exec("UPDATE approval SET state = 'granted' WHERE card = ?", string(v.item.Msg))
		return err
	}
	_, err := tx.Exec("DELETE FROM approval WHERE card = ?", string(v.item.Msg))
	return err
}

func ackBatch(h *Host, batch []heldItem) {
	for _, entry := range batch {
		sendAck(h, entry.home, entry.group, entry.item.Item)
	}
}

func sendAck(h *Host, home, group string, item int64) {
	if conn, ok := h.Home(home); ok {
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

func hasPendingApproval(db *store.Store, card, group, member string) bool {
	var ok bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT EXISTS(SELECT 1 FROM approval WHERE card = ? AND g = ? AND member = ? AND state = 'pending')", card, group, member).Scan(&ok)
	}
	db.Write(read)
	return ok
}

func hasNoPendingApproval(db *store.Store, card, group, member string) bool {
	var exists bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT EXISTS(SELECT 1 FROM approval WHERE card = ? AND g = ? AND member = ?)", card, group, member).Scan(&exists)
	}
	err := db.Write(read)
	return err == nil && !exists
}

func recordAudit(h *Host, row AuditRow) error {
	write := func(tx *store.Tx) error {
		_, err := h.Audit(tx, row)
		return err
	}
	return h.Store.Write(write)
}

func refuseItem(h *Host, home, group, agent string, item int64, by, detail string) {
	if item < 1 {
		recordAudit(h, AuditRow{Group: group, Agent: agent, What: AuditRefused, Home: home, Detail: detail, By: by})
		return
	}
	row := AuditRow{Group: group, Agent: agent, What: AuditRefused, Item: item, Home: home, Detail: detail, By: by}
	if recordAudit(h, row) == nil {
		sendAck(h, home, group, item)
	}
}

func mergeHeldNow(h *Host, d *delivery, cfg hostConfig, group, member string) bool {
	if d.groupSyncActive(group) || cfg.Offline || groupIsPaused(h, group) {
		return true
	}
	reason, held := h.Held(SessionKey{Group: group, Member: member})
	return held && reason == "offline"
}

func stepMerges(h *Host, d *delivery, cfg hostConfig, group string) {
	d.mu.Lock()
	pending := d.merges[group]
	delete(d.merges, group)
	d.mu.Unlock()
	var kept []verifiedItem
	for _, v := range pending {
		if mergeHeldNow(h, d, cfg, group, string(v.item.AI)) {
			kept = append(kept, v)
		} else {
			takeOneMerge(h, d, cfg, v)
		}
	}
	d.requeueMerges(group, kept)
}

func (d *delivery) requeueMerges(group string, kept []verifiedItem) {
	if len(kept) == 0 {
		return
	}
	d.mu.Lock()
	d.merges[group] = append(kept, d.merges[group]...)
	d.mu.Unlock()
}

func takeOneMerge(h *Host, d *delivery, cfg hostConfig, v verifiedItem) {
	ai, _ := lookupLocalAI(h, v.group, string(v.item.AI))
	if reason := checkStillCurrent(h, v); reason != "" {
		refuseItem(h, v.home, v.group, ai.agent, v.item.Item, v.wire.Env.Key, reason)
		return
	}
	task, _ := lookupTaskRow(h, v.group, string(v.item.Task))
	mKey := mergeTrackKey(v.group, task.ID, v.item.Commit)
	if !d.claimMerge(mKey) {
		return
	}
	defer d.releaseMerge(mKey)
	if recordMergeTaken(h, ai.agent, v, task.ID) != nil {
		d.enqueueMerge(v)
		return
	}
	sendAck(h, v.home, v.group, v.item.Item)
	runReviewedMerge(h, d, cfg, v.home, v.group, task, v.item.Commit, v.item.PR, false, time.Time{})
}

func mergeTrackKey(group, taskID, commit string) string {
	return group + "\x00" + taskID + "\x00" + commit
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

func (d *delivery) releaseMerge(key string) {
	d.mu.Lock()
	delete(d.merging, key)
	d.mu.Unlock()
}

func recordMergeTaken(h *Host, agent string, v verifiedItem, taskID string) error {
	write := func(tx *store.Tx) error {
		delivRow := AuditRow{Group: v.group, Agent: agent, What: AuditDelivered, Item: v.item.Item, Home: v.home, Detail: "merge", By: v.wire.Env.Key}
		if _, err := h.Audit(tx, delivRow); err != nil {
			return err
		}
		mergeRow := AuditRow{Group: v.group, Agent: agent, What: AuditMerge, Detail: taskID + " " + v.item.Commit}
		_, err := h.Audit(tx, mergeRow)
		return err
	}
	return h.Store.Write(write)
}

func resumeGroupMerges(h *Host, d *delivery, cfg hostConfig, group string) {
	home := knownHome(h, d, cfg, group)
	if home == "" {
		return
	}
	for _, raw := range h.Rows.Of(group, "task") {
		var task proto.TaskRow
		if proto.Decode(raw, &task) == nil && task.State == "active" && task.Step == "merge" {
			resumeOneMerge(h, d, cfg, home, group, task)
		}
	}
}

func resumeOneMerge(h *Host, d *delivery, cfg hostConfig, home, group string, task proto.TaskRow) {
	if _, local := lookupLocalAI(h, group, task.Assignee); !local || mergeHeldNow(h, d, cfg, group, task.Assignee) {
		return
	}
	ts, found := findAuditMergeTS(h.Store, group, task.ID+" "+task.Commit)
	if !found {
		return
	}
	mKey := mergeTrackKey(group, task.ID, task.Commit)
	if !d.claimUnreportedMerge(mKey) {
		return
	}
	defer d.releaseMerge(mKey)
	runReviewedMerge(h, d, cfg, home, group, task, task.Commit, task.PR, true, time.UnixMilli(ts))
}

func (d *delivery) claimUnreportedMerge(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.merging[key] || d.reported[key] {
		return false
	}
	d.merging[key] = true
	return true
}

func findAuditMergeTS(db *store.Store, group, detail string) (int64, bool) {
	var ts int64
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT ts FROM audit WHERE g = ? AND what = ? AND detail = ? ORDER BY id DESC LIMIT 1), 0)",
			group, AuditMerge, detail).Scan(&ts)
	}
	db.Write(read)
	return ts, ts > 0
}

func runReviewedMerge(h *Host, d *delivery, cfg hostConfig, home, group string, task proto.TaskRow, commit string, pr int, inspectOnly bool, taken time.Time) {
	ctx := context.Background()
	grp, _ := lookupGroupRow(h, group)
	pDir := cfg.projectDir(group)
	if pDir == "" {
		return
	}
	ensureCloneIfConfigured(ctx, pDir, grp)
	rm := buildReviewedMerge(pDir, grp, task, commit, pr, taken)
	res, err := executeMergeOp(ctx, rm, inspectOnly)
	if err != nil {
		return
	}
	reportMergeOutcome(ctx, h, d, home, group, grp, rm.Repo, task, commit, res)
}

func buildReviewedMerge(pDir string, grp proto.GroupRow, task proto.TaskRow, commit string, pr int, taken time.Time) ReviewedMerge {
	branch := task.Branch
	if branch == "" {
		if name, err := taskName(task.Num, string(task.Slug)); err == nil {
			branch = "task/" + name
		}
	}
	var forgePR int
	var ghRepo string
	if grp.Forge == "github" {
		forgePR = pr
		ghRepo = githubRepo(grp.Repo)
	}
	return ReviewedMerge{
		Repo:       filepath.Join(pDir, "repo"),
		Base:       grp.Base,
		Branch:     branch,
		Task:       task.ID,
		Commit:     commit,
		PR:         forgePR,
		GitHubRepo: ghRepo,
		Taken:      taken,
	}
}

func executeMergeOp(ctx context.Context, rm ReviewedMerge, inspectOnly bool) (MergeResult, error) {
	if inspectOnly {
		return rm.Inspect(ctx)
	}
	return rm.Apply(ctx)
}

func reportMergeOutcome(ctx context.Context, h *Host, d *delivery, home, group string, grp proto.GroupRow, repoDir string, task proto.TaskRow, commit string, res MergeResult) {
	var payload string
	if res.MergeCommit != "" {
		stats := computeMergeStats(ctx, repoDir, grp, commit, task.Stats)
		payload = encodeJSON(proto.TaskMerged{
			TaskCommand: hostTaskCommand(h, group, "merged", task.ID),
			Commit:      commit,
			MergeCommit: res.MergeCommit,
			Stats:       stats,
		})
	} else {
		payload = encodeJSON(proto.TaskConflict{
			TaskCommand: hostTaskCommand(h, group, "conflict", task.ID),
			Commit:      commit,
			Detail:      res.Conflict,
		})
	}
	if _, err := h.Command(ctx, home, payload); err == nil {
		d.markMergeReported(mergeTrackKey(group, task.ID, commit))
	}
}

func (d *delivery) markMergeReported(key string) {
	d.mu.Lock()
	d.reported[key] = true
	d.mu.Unlock()
}

var (
	shortstatAdded   = regexp.MustCompile(`(\d+) insertion`)
	shortstatRemoved = regexp.MustCompile(`(\d+) deletion`)
)

func computeMergeStats(ctx context.Context, repoDir string, grp proto.GroupRow, commit string, fallback proto.TaskStats) proto.DiffStats {
	baseRef := "refs/heads/" + grp.Base
	if grp.Forge == "github" {
		baseRef = "refs/remotes/origin/" + grp.Base
	}
	out, err := workspaceGit(ctx, repoDir, "diff", "--shortstat", baseRef+"..."+commit)
	if err != nil {
		return proto.DiffStats{Added: fallback.Added, Removed: fallback.Removed}
	}
	return proto.DiffStats{
		Added:   submatchInt(shortstatAdded, out),
		Removed: submatchInt(shortstatRemoved, out),
	}
}

func submatchInt(re *regexp.Regexp, text string) int {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func githubRepo(raw string) string {
	trimmed := strings.TrimSuffix(strings.TrimRight(raw, `/\`), ".git")
	parts := strings.FieldsFunc(trimmed, isRepoSep)
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func isRepoSep(r rune) bool {
	return r == '/' || r == ':' || r == '\\'
}

func reconcileGroup(h *Host, cfg hostConfig, group string) {
	pDir := cfg.projectDir(group)
	grp, _ := lookupGroupRow(h, group)
	for _, s := range h.Sessions.All() {
		if s.Group == group {
			reconcileSession(h, pDir, grp, s)
		}
	}
}

func reconcileSession(h *Host, pDir string, grp proto.GroupRow, s Session) {
	if isRowGone(h.Rows, s.Group, "member", s.Member) {
		endSessionAndApprovals(h, s)
		dropHeldMember(h, s.Group, s.Member)
		return
	}
	if s.Kind == TaskSession {
		reconcileTaskSession(h, pDir, grp, s)
	}
	if s.Kind == ReviewSession {
		reconcileReviewSession(h, pDir, s)
	}
}

func isRowGone(rows *Rows, group, kind, id string) bool {
	raw, ok := rows.Get(group, kind, id)
	if !ok {
		return false
	}
	var gone proto.GoneRow
	return proto.Decode(raw, &gone) == nil && gone.Gone
}

func dropHeldMember(h *Host, group, member string) {
	d := deliv(h)
	d.mu.Lock()
	delete(d.held, aiGroupKey{group: group, member: member})
	d.mu.Unlock()
}

func reconcileTaskSession(h *Host, pDir string, grp proto.GroupRow, s Session) {
	if isRowGone(h.Rows, s.Group, "task", s.Task) {
		endSessionAndApprovals(h, s)
		return
	}
	task, ok := lookupTaskRow(h, s.Group, s.Task)
	if !ok {
		return
	}
	release, ended := taskReleaseKind(task, s.Member)
	if !ended {
		return
	}
	cleanupTaskWorkspace(pDir, grp, s, task, release)
	endSessionAndApprovals(h, s)
}

func taskReleaseKind(task proto.TaskRow, member string) (TaskRelease, bool) {
	if task.State == "merged" {
		return TaskMerged, true
	}
	if task.State == "cancelled" {
		return TaskCancelled, true
	}
	if task.Assignee != member {
		return TaskGivenAway, true
	}
	return 0, false
}

func cleanupTaskWorkspace(pDir string, grp proto.GroupRow, s Session, task proto.TaskRow, why TaskRelease) {
	if !hasGitClone(pDir) {
		return
	}
	ctx := context.Background()
	name, err := taskName(task.Num, string(task.Slug))
	if err != nil {
		return
	}
	branch := "task/" + name
	if why == TaskGivenAway && grp.Forge == "github" {
		TaskForge{Worktree: filepath.Join(pDir, s.Folder), Branch: branch}.PushBranch(ctx)
	}
	RemoveTaskWorkspace(ctx, pDir, task.Num, string(task.Slug), s.Member, why)
	if why == TaskMerged {
		ReviewedMerge{Repo: filepath.Join(pDir, "repo"), Branch: branch, Commit: task.Commit}.DeleteBranch(ctx)
	}
}

func reconcileReviewSession(h *Host, pDir string, s Session) {
	if isRowGone(h.Rows, s.Group, "task", s.Task) {
		h.Sessions.End(s.SessionKey)
		return
	}
	task, ok := lookupTaskRow(h, s.Group, s.Task)
	if !ok || !reviewEnded(task, s.Member) {
		return
	}
	finishEndedReview(h, pDir, s, task)
}

func finishEndedReview(h *Host, pDir string, s Session, task proto.TaskRow) {
	if hasGitClone(pDir) {
		RemoveReviewWorkspace(context.Background(), pDir, task.Num, s.Member)
	}
	if task.State == "merged" || task.State == "cancelled" {
		deleteTaskApprovals(h.Store, s.Group, s.Task)
	}
	h.Sessions.End(s.SessionKey)
}

func reviewEnded(task proto.TaskRow, member string) bool {
	return task.State == "merged" || task.State == "cancelled" || task.Reviewer != member
}

func endSessionAndApprovals(h *Host, s Session) {
	if s.Task == "" {
		deleteMemberApprovals(h.Store, s.Group, s.Member)
	} else {
		deleteTaskApprovals(h.Store, s.Group, s.Task)
	}
	h.Sessions.End(s.SessionKey)
}

func deleteMemberApprovals(db *store.Store, group, member string) {
	del := func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM approval WHERE g = ? AND member = ?", group, member)
		return err
	}
	db.Write(del)
}

func deleteTaskApprovals(db *store.Store, group, taskID string) {
	del := func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM approval WHERE g = ? AND task = ?", group, taskID)
		return err
	}
	db.Write(del)
}

func hostTaskCommand(h *Host, group, op, taskID string) proto.TaskCommand {
	return proto.TaskCommand{
		HomeCommand: proto.HomeCommand{
			Type:      "task",
			Nonce:     newNonce(),
			Timestamp: h.Now().UnixMilli(),
			Group:     proto.ID(group),
		},
		Op: op,
		ID: proto.ID(taskID),
	}
}

func newNonce() proto.ID {
	buf := make([]byte, 16)
	rand.Read(buf)
	return proto.ID(base64.RawURLEncoding.EncodeToString(buf))
}

func encodeJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}
