package host

import (
	"bytes"
	"cmp"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	defaultApprovalWaitS = 600
	maxCommandJSONBytes  = 3072
	maxWhyJSONBytes      = 256

	denyReadOnlyMessage = "Denied: this session is read only."
	denySecretMessage   = "This request contains a secret and cannot be shown for approval"
	denyRefusedPrefix   = "The approval request was refused: "
	denyOwnerMessage    = "Denied by the owner."
	denyTimeoutMessage  = "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."
)

var (
	// approvalWaitUnit scales limits.approval_wait_s so unit tests can exercise
	// expiry without 600-second sleeps.
	approvalWaitUnit = time.Second

	approveInputSchema = json.RawMessage(`{"type":"object","properties":{"input":{"type":"object"},"tool_name":{"type":"string"},"tool_use_id":{"type":"string"}},"required":["tool_name","input","tool_use_id"],"additionalProperties":false}`)
)

// ApprovalResult is the MCP tool output of approve (contracts/mcp-tools.md section 3).
type ApprovalResult struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

func init() {
	Register(Tools, "approve", Tool{
		Description: "Ask the owner for permission to run a CLI action.",
		Schema:      approveInputSchema,
		For:         approveForCaller,
		Call:        callApprove,
	})
	Register(Listeners, "approve", listenApprove)
}

func approveForCaller(c Caller) bool {
	return c.Agent == "claude" && !c.Interactive
}

// approvalState tracks open Asked listener goroutines per host via Part[approvalState](h).
// Lock order: mu is a leaf lock; nothing called under mu acquires Host, Sessions, Rows or Store locks.
type approvalState struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	nextID  uint64
	cancels map[SessionKey]map[uint64]context.CancelFunc
}

func approveStateOf(h *Host) *approvalState {
	st := Part[approvalState](h)
	st.mu.Lock()
	if st.cancels == nil {
		st.cancels = map[SessionKey]map[uint64]context.CancelFunc{}
	}
	st.mu.Unlock()
	return st
}

func (st *approvalState) registerAsk(key SessionKey, cancel context.CancelFunc) uint64 {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.nextID++
	id := st.nextID
	if st.cancels[key] == nil {
		st.cancels[key] = map[uint64]context.CancelFunc{}
	}
	st.cancels[key][id] = cancel
	return id
}

func (st *approvalState) unregisterAsk(key SessionKey, id uint64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.cancels[key], id)
	if len(st.cancels[key]) == 0 {
		delete(st.cancels, key)
	}
}

func cancelSessionAsks(h *Host, key SessionKey) {
	st := approveStateOf(h)
	st.mu.Lock()
	active := st.cancels[key]
	delete(st.cancels, key)
	st.mu.Unlock()
	for _, cancel := range active {
		cancel()
	}
}

func listenApprove(h *Host, key SessionKey, e Event) {
	if isTurnBoundary(e.Kind) {
		cancelSessionAsks(h, key)
		return
	}
	if e.Kind != Asked || e.Ask == nil {
		return
	}
	if s, ok := h.Sessions.Get(key); ok {
		startAskedGoroutine(h, s, *e.Ask)
	}
}

func isTurnBoundary(kind EventKind) bool {
	return kind == TurnBegan || kind == TurnEnded
}

func startAskedGoroutine(h *Host, s Session, ask Ask) {
	callDone := h.Sessions.Calling(s.SessionKey)
	st := approveStateOf(h)
	ctx, cancel := context.WithCancel(context.Background())
	id := st.registerAsk(s.SessionKey, cancel)
	st.wg.Add(1)
	go runAskedEvent(ctx, cancel, h, st, s, id, ask, callDone)
}

func runAskedEvent(ctx context.Context, cancel context.CancelFunc, h *Host, st *approvalState, s Session, id uint64, ask Ask, callDone func()) {
	defer st.wg.Done()
	defer cancel()
	defer st.unregisterAsk(s.SessionKey, id)
	defer callDone()
	level := resolveSessionLevel(h, s)
	allow, _ := handleApproval(ctx, h, s, level, ask)
	if ctx.Err() == nil && Runner != nil {
		Runner.Answer(h, s.SessionKey, ask.ID, allow)
	}
}

const querySessionLevelSQL = `SELECT
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'all_level3'), 0),
	coalesce((SELECT level FROM perm WHERE agent = ? AND g = ?), 0),
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'default_level'), 2)`

func resolveSessionLevel(h *Host, s Session) int {
	if s.Kind != TaskSession {
		return 1
	}
	all3, permLevel, defLevel := 0, 0, 2
	read := func(tx *store.Tx) error {
		return tx.QueryRow(querySessionLevelSQL, s.Agent, s.Group).Scan(&all3, &permLevel, &defLevel)
	}
	h.Store.Write(read)
	if all3 != 0 {
		return 3
	}
	if permLevel != 0 {
		return permLevel
	}
	return defLevel
}

type approveToolArgs struct {
	ToolName  *string         `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID *string         `json:"tool_use_id"`
}

func callApprove(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	ask, err := parseApproveArgs(raw)
	if err != nil {
		return nil, err
	}
	allow, msg := handleApproval(ctx, h, c.Session, c.Level, ask)
	if allow {
		return ApprovalResult{Behavior: "allow"}, nil
	}
	return ApprovalResult{Behavior: "deny", Message: msg}, nil
}

func parseApproveArgs(raw json.RawMessage) (Ask, error) {
	if !isJSONObject(raw) {
		return Ask{}, &Error{Code: "bad_request", Message: "approve arguments must be a JSON object"}
	}
	var a approveToolArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&a) != nil || !hasApproveFields(a) {
		return Ask{}, &Error{Code: "bad_request", Message: "invalid approve arguments"}
	}
	return buildClaudeAsk(*a.ToolName, *a.ToolUseID, a.Input)
}

func hasApproveFields(a approveToolArgs) bool {
	return a.ToolName != nil && *a.ToolName != "" && a.ToolUseID != nil && isJSONObject(a.Input)
}

func isJSONObject(raw json.RawMessage) bool {
	return bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) && json.Valid(raw)
}

func buildClaudeAsk(toolName, toolUseID string, input json.RawMessage) (Ask, error) {
	ask := Ask{ID: toolUseID, Tool: toolName, Input: input}
	if toolName == "Bash" {
		var bash struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		if json.Unmarshal(input, &bash) != nil {
			return Ask{}, &Error{Code: "bad_request", Message: "invalid Bash input"}
		}
		ask.Category, ask.Command, ask.Why = "command", bash.Command, bash.Description
		return ask, nil
	}
	ask.Category = claudeToolCategory(toolName)
	ask.Command = compactJSONString(input)
	return ask, nil
}

func claudeToolCategory(toolName string) string {
	switch toolName {
	case "Write", "Edit", "NotebookEdit", "Read":
		return "file"
	case "WebFetch", "WebSearch":
		return "network"
	default:
		return "other"
	}
}

func compactJSONString(raw json.RawMessage) string {
	var buf bytes.Buffer
	json.Compact(&buf, raw)
	return buf.String()
}

func handleApproval(ctx context.Context, h *Host, s Session, level int, ask Ask) (bool, string) {
	if level == 1 {
		return false, denyReadOnlyMessage
	}
	if level == 3 {
		return true, ""
	}
	shortCmd, full := shortenCommand(ask.Command)
	digest := hashApprovalInput(ask.Input)
	if consumeGrantedApproval(h, s, ask.Tool, digest, shortCmd) {
		return true, ""
	}
	if hasApprovalSecret(ask) {
		recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: ask.Tool, Result: "secret"})
		return false, denySecretMessage
	}
	return postAndWaitApproval(ctx, h, s, ask, shortCmd, full, digest)
}

func hasApprovalSecret(ask Ask) bool {
	return CheckOutboundSecret(ask.Command) != nil || CheckOutboundSecret(ask.Why) != nil
}

func shortenCommand(command string) (string, *int) {
	short, cut := shortenJSONText(command, maxCommandJSONBytes)
	if !cut {
		return short, nil
	}
	full := len(command)
	return short, &full
}

func optionalWhy(why string) *string {
	if why == "" {
		return nil
	}
	short, _ := shortenJSONText(why, maxWhyJSONBytes)
	return &short
}

func shortenJSONText(text string, limit int) (string, bool) {
	if jsonEncodedLen(text) <= limit {
		return text, false
	}
	cut := 0
	for {
		_, width := utf8.DecodeRuneInString(text[cut:])
		if jsonEncodedLen(text[:cut+width]) > limit {
			return text[:cut], true
		}
		cut += width
	}
}

func jsonEncodedLen(s string) int {
	return len(marshalApproveJSON(s))
}

func marshalApproveJSON(v any) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

func hashApprovalInput(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var val any
	data := bytes.TrimSpace(raw)
	if dec.Decode(&val) == nil {
		data = []byte(marshalApproveJSON(val))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const deleteOneGrantedSQL = `DELETE FROM approval WHERE card = (
	SELECT card FROM approval
	WHERE g = ? AND member = ? AND task = ? AND tool = ? AND input = ? AND state = 'granted'
	ORDER BY rowid LIMIT 1
)`

func consumeGrantedApproval(h *Host, s Session, tool, digest, shortCmd string) bool {
	consumed := false
	work := func(tx *store.Tx) error {
		res, err := tx.Exec(deleteOneGrantedSQL, s.Group, s.Member, s.Task, tool, digest)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return nil
		}
		consumed = true
		_, err = h.Audit(tx, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: shortCmd, Result: "approved"})
		return err
	}
	err := h.Store.Write(work)
	return err == nil && consumed
}

func postAndWaitApproval(ctx context.Context, h *Host, s Session, ask Ask, shortCmd string, full *int, digest string) (bool, string) {
	cardID := newApproveID()
	approvers, mentions := resolveApproversAndMentions(h, s.Group, s.Member)
	if storePendingApproval(h, cardID, s, ask.Tool, digest) != nil {
		return false, denyRefusedPrefix + "internal: failed to store approval record"
	}
	check := func(raw json.RawMessage) error {
		return verifyOpenWaitCause(h, s, cardID, approvers, raw)
	}
	itemCh, done := h.Await(string(cardID), check)
	defer done()
	payload := buildApprovalPostPayload(h, s, cardID, ask, shortCmd, full, approvers, mentions)
	if _, err := h.Command(ctx, s.Home, payload); err != nil {
		deleteApprovalCard(h, string(cardID))
		return false, denyRefusedPrefix + err.Error()
	}
	recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditRequested, Detail: shortCmd})
	raw, ok := awaitApprovalItem(ctx, approvalWaitDuration(h, s.Group), itemCh, done)
	if !ok {
		return false, denyTimeoutMessage
	}
	return finishTimelyDecision(h, s, string(cardID), shortCmd, raw)
}

func newApproveID() proto.ID {
	var b [16]byte
	crand.Read(b[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(b[:]))
}

func storePendingApproval(h *Host, cardID proto.ID, s Session, tool, digest string) error {
	work := func(tx *store.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO approval(card, g, member, task, tool, input, state) VALUES(?, ?, ?, ?, ?, ?, 'pending')",
			string(cardID), s.Group, s.Member, s.Task, tool, digest,
		)
		return err
	}
	return h.Store.Write(work)
}

func deleteApprovalCard(h *Host, cardID string) {
	work := func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM approval WHERE card = ?", cardID)
		return err
	}
	h.Store.Write(work)
}

func recordApprovalAudit(h *Host, row AuditRow) error {
	work := func(tx *store.Tx) error {
		_, err := h.Audit(tx, row)
		return err
	}
	return h.Store.Write(work)
}

func buildApprovalPostPayload(h *Host, s Session, cardID proto.ID, ask Ask, shortCmd string, full *int, approvers []string, mentions []proto.ID) string {
	thread := proto.ID(s.Task)
	cmd := proto.Post[proto.ApprovalBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     cardID,
			Timestamp: h.Now().UnixMilli(),
			Group:     proto.ID(s.Group),
			As:        proto.ID(s.Member),
		},
		Kind:     "approval",
		Thread:   &thread,
		Main:     true,
		Text:     "",
		Mentions: mentions,
		Re:       nil,
		Body: proto.ApprovalBody{
			Task:      s.Task,
			AI:        s.Member,
			Tool:      ask.Tool,
			Command:   shortCmd,
			Full:      full,
			Why:       optionalWhy(ask.Why),
			Category:  ask.Category,
			Approvers: approvers,
		},
	}
	return marshalApproveJSON(cmd)
}

func resolveApproversAndMentions(h *Host, group, member string) ([]string, []proto.ID) {
	approvers := memberApprovers(h.Rows, group, member)
	if len(approvers) == 0 {
		approvers = readStoreApprovers(h.Store)
	}
	return approvers, approverMentionIDs(h.Rows, group, approvers)
}

func memberApprovers(rows *Rows, group, member string) []string {
	raw, ok := rows.Get(group, "member", member)
	if !ok {
		return nil
	}
	var ai proto.AIMemberRow
	if proto.Decode(raw, &ai) != nil || ai.Kind != "ai" || len(ai.Approvers) == 0 {
		return nil
	}
	return slices.Clone(ai.Approvers)
}

const queryApproversFallbackSQL = `SELECT
	coalesce((SELECT value FROM setting WHERE key = 'approvers'), '[]'),
	coalesce((SELECT value FROM setting WHERE key = 'owner'), '""')`

func readStoreApprovers(db *store.Store) []string {
	var listJSON, ownerJSON string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(queryApproversFallbackSQL).Scan(&listJSON, &ownerJSON)
	}
	db.Write(read)
	var list []string
	json.Unmarshal([]byte(listJSON), &list)
	if len(list) != 0 {
		return list
	}
	var owner string
	json.Unmarshal([]byte(ownerJSON), &owner)
	if owner != "" {
		return []string{owner}
	}
	return []string{}
}

func approverMentionIDs(rows *Rows, group string, approvers []string) []proto.ID {
	mentions := []proto.ID{}
	for _, raw := range rows.Of(group, "member") {
		var m proto.MemberRow
		if proto.Decode(raw, &m) == nil && m.Kind == "human" && slices.Contains(approvers, m.User) {
			mentions = append(mentions, proto.ID(m.ID))
		}
	}
	return mentions
}

func approvalWaitDuration(h *Host, group string) time.Duration {
	sec := defaultApprovalWaitS
	if raw, ok := h.Rows.Get(group, "group", group); ok {
		var grp proto.GroupRow
		if proto.Decode(raw, &grp) == nil && grp.Limits.ApprovalWaitS > 0 {
			sec = grp.Limits.ApprovalWaitS
		}
	}
	return time.Duration(sec) * approvalWaitUnit
}

// awaitApprovalItem is the single reader of itemCh (returned by Host.Await).
func awaitApprovalItem(ctx context.Context, wait time.Duration, itemCh <-chan json.RawMessage, done func()) (json.RawMessage, bool) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case raw := <-itemCh:
		return raw, true
	case <-timer.C:
		done()
	case <-ctx.Done():
		done()
	}
	select {
	case raw := <-itemCh:
		return raw, true
	default:
		return nil, false
	}
}

func finishTimelyDecision(h *Host, s Session, cardID, shortCmd string, raw json.RawMessage) (bool, string) {
	action, by := extractDecisionAct(raw)
	result := "denied"
	if action == "approve" {
		result = "approved"
	}
	work := func(tx *store.Tx) error {
		tx.Exec("DELETE FROM approval WHERE card = ?", cardID)
		_, err := h.Audit(tx, AuditRow{
			Group:  s.Group,
			Agent:  s.Agent,
			What:   AuditApproval,
			Detail: shortCmd,
			By:     by,
			Result: result,
		})
		return err
	}
	h.Store.Write(work)
	if action == "approve" {
		return true, ""
	}
	return false, denyOwnerMessage
}

func extractDecisionAct(raw json.RawMessage) (action, by string) {
	var wire proto.InboxItem
	json.Unmarshal(raw, &wire)
	if wire.Cause == nil {
		return "", ""
	}
	var act proto.Act
	json.Unmarshal([]byte(wire.Cause.Payload), &act)
	return act.Action, wire.Cause.Key
}

type refusalMeta struct {
	group string
	item  int64
	by    string
}

func extractRefusalMeta(raw json.RawMessage) refusalMeta {
	var wire struct {
		Env   proto.Envelope  `json:"env"`
		Cause *proto.Envelope `json:"cause"`
	}
	json.Unmarshal(raw, &wire)
	var it struct {
		Group string `json:"g"`
		Item  int64  `json:"item"`
	}
	json.Unmarshal([]byte(wire.Env.Payload), &it)
	by := wire.Env.Key
	if wire.Cause != nil {
		by = cmp.Or(wire.Cause.Key, by)
	}
	return refusalMeta{group: it.Group, item: it.Item, by: by}
}

func verifyOpenWaitCause(h *Host, s Session, cardID proto.ID, approvers []string, raw json.RawMessage) error {
	wire, it, ok := parseDecidedWire(raw)
	if !ok || !matchesWaitingSession(it, s, cardID) {
		meta := extractRefusalMeta(raw)
		recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditRefused, Item: meta.item, Home: s.Home, Detail: "invalid approval item", By: meta.by})
		return &Error{Code: "bad_request", Message: "invalid approval item"}
	}
	if _, ok := verifyApprovalAct(h, s.Group, cardID, approvers, *wire.Cause); !ok || !hasPendingApprovalRow(h.Store, string(cardID), s.Group, s.Member) {
		recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditRefused, Item: it.Item, Home: s.Home, Detail: "invalid approval cause", By: wire.Cause.Key})
		return &Error{Code: "forbidden", Message: "invalid approval cause"}
	}
	return nil
}

func parseDecidedWire(raw json.RawMessage) (proto.InboxItem, proto.Item, bool) {
	var wire proto.InboxItem
	var it proto.Item
	if proto.Decode(raw, &wire) != nil || wire.Cause == nil {
		return wire, it, false
	}
	ok := proto.Decode([]byte(wire.Env.Payload), &it) == nil && isDecidedItem(it)
	return wire, it, ok
}

func isDecidedItem(it proto.Item) bool {
	return it.Type == "item" && it.Item >= 1 && it.Why == "decided" && it.Msg != ""
}

func matchesWaitingSession(it proto.Item, s Session, cardID proto.ID) bool {
	return string(it.Group) == s.Group && string(it.AI) == s.Member && it.Msg == cardID
}

func verifyApprovalAct(h *Host, group string, cardID proto.ID, approvers []string, cause proto.Envelope) (proto.Act, bool) {
	var act proto.Act
	if proto.Decode([]byte(cause.Payload), &act) != nil || !isValidApproveAct(act, group, cardID) {
		return act, false
	}
	known := func(dev string) bool {
		return isApproverDevice(h, group, approvers, dev)
	}
	_, err := identity.OpenCommand(cause, h.Key.ID(), known, time.UnixMilli(act.Timestamp))
	return act, err == nil
}

func isValidApproveAct(act proto.Act, group string, cardID proto.ID) bool {
	if act.Type != "act" || string(act.Group) != group || act.Msg != cardID {
		return false
	}
	return act.As == "" && isApproveOrDenyVerb(act)
}

func isApproveOrDenyVerb(act proto.Act) bool {
	return (act.Action == "approve" || act.Action == "deny") && act.Arg == "" && act.Text == ""
}

func isApproverDevice(h *Host, group string, approvers []string, device string) bool {
	for _, user := range approvers {
		if validInStoreLog(h.Store, user, device) || validInGroupRows(h.Rows, group, user, device) {
			return true
		}
	}
	return false
}

const readUserLogSQL = `SELECT coalesce(json_group_array(json(env)), '[]') FROM (
	SELECT env FROM device_log WHERE user = ? ORDER BY ver
)`

func validInStoreLog(db *store.Store, user, device string) bool {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(readUserLogSQL, user).Scan(&raw)
	}
	if db.Write(read) != nil {
		return false
	}
	var envs []proto.Envelope
	json.Unmarshal([]byte(raw), &envs)
	log, err := identity.Replay(user, envs)
	return err == nil && log.Valid(device)
}

func validInGroupRows(rows *Rows, group, user, device string) bool {
	var list []proto.DeviceRow
	for _, raw := range rows.Of(group, "device") {
		var dr proto.DeviceRow
		if proto.Decode(raw, &dr) == nil && dr.User == user {
			list = append(list, dr)
		}
	}
	slices.SortFunc(list, compareDeviceVer)
	envs := make([]proto.Envelope, len(list))
	for i, dr := range list {
		envs[i] = dr.Env
	}
	log, err := identity.Replay(user, envs)
	return err == nil && log.Valid(device)
}

func compareDeviceVer(a, b proto.DeviceRow) int {
	return cmp.Compare(a.Version, b.Version)
}

func hasPendingApprovalRow(db *store.Store, card, group, member string) bool {
	var exists bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM approval WHERE card = ? AND g = ? AND member = ? AND state = 'pending')",
			card, group, member,
		).Scan(&exists)
	}
	err := db.Write(read)
	return err == nil && exists
}

// RecordApprovalDecision verifies a decided inbox item from home for an
// approval card, hands it to an open wait when the CLI is still waiting, or
// applies the late decision to the stored approval record in one transaction
// with its delivered audit row (flows/approval.md sections C and D). An item
// already in the audit record is acknowledged again without a second effect.
func (h *Host) RecordApprovalDecision(home string, raw json.RawMessage) (string, error) {
	wire, it, ok := verifyDecisionEnvelope(h, home, raw)
	if !ok {
		return "", refuseDecisionRaw(h, home, raw)
	}
	if alreadyAuditedItem(h.Store, home, it.Item) {
		ackDecisionItem(h, home, it.Group, it.Item)
		return "", nil
	}
	agent := lookupApprovalAgent(h, string(it.Group), string(it.AI))
	if done, action, err := handOpenApproval(h, home, agent, wire, it, raw); done {
		return action, err
	}
	return applyLateApprovalDecision(h, home, agent, wire, it)
}

func verifyDecisionEnvelope(h *Host, home string, raw json.RawMessage) (proto.InboxItem, proto.Item, bool) {
	wire, it, ok := parseDecidedWire(raw)
	if !ok || wire.Env.Key != home {
		return wire, it, false
	}
	owner := readOwnerUser(h.Store)
	known := func(dev string) bool {
		return validInStoreLog(h.Store, owner, dev)
	}
	_, err := identity.OpenCommand(wire.Env, h.Key.ID(), known, time.UnixMilli(it.Timestamp))
	return wire, it, err == nil
}

func readOwnerUser(db *store.Store) string {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT value FROM setting WHERE key = 'owner'), '\"\"')").Scan(&raw)
	}
	db.Write(read)
	var owner string
	json.Unmarshal([]byte(raw), &owner)
	return owner
}

func alreadyAuditedItem(db *store.Store, home string, item int64) bool {
	var seen bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT EXISTS(SELECT 1 FROM audit WHERE home = ? AND item = ?)", home, item).Scan(&seen)
	}
	err := db.Write(read)
	return err == nil && seen
}

func ackDecisionItem(h *Host, home string, group proto.ID, item int64) {
	if conn, ok := h.Home(home); ok {
		conn.Send(proto.Ack{Type: "ack", Group: group, Item: item})
	}
}

func refuseDecisionRaw(h *Host, home string, raw json.RawMessage) error {
	meta := extractRefusalMeta(raw)
	row := AuditRow{Group: meta.group, What: AuditRefused, Item: meta.item, Home: home, Detail: "invalid decision item", By: meta.by}
	if recordApprovalAudit(h, row) == nil && meta.item >= 1 && meta.group != "" {
		ackDecisionItem(h, home, proto.ID(meta.group), meta.item)
	}
	return &Error{Code: "forbidden", Message: "invalid decision item"}
}

func lookupApprovalAgent(h *Host, group, member string) string {
	if agent := memberRowCLI(h.Rows, group, member); agent != "" {
		return agent
	}
	return sessionRowAgent(h.Sessions, group, member)
}

func memberRowCLI(rows *Rows, group, member string) string {
	raw, ok := rows.Get(group, "member", member)
	var ai proto.AIMemberRow
	if !ok || proto.Decode(raw, &ai) != nil || ai.Kind != "ai" {
		return ""
	}
	return ai.CLI
}

func sessionRowAgent(sessions *Sessions, group, member string) string {
	for _, s := range sessions.All() {
		if s.Group == group && s.Member == member {
			return s.Agent
		}
	}
	return ""
}

func handOpenApproval(h *Host, home, agent string, wire proto.InboxItem, it proto.Item, raw json.RawMessage) (bool, string, error) {
	waited, err := h.Hand(string(it.Msg), raw)
	if !waited {
		return false, "", nil
	}
	if err != nil {
		recordApprovalAudit(h, AuditRow{Group: string(it.Group), Agent: agent, What: AuditRefused, Item: it.Item, Home: home, Detail: "invalid approval cause", By: wire.Cause.Key})
		ackDecisionItem(h, home, it.Group, it.Item)
		return true, "", err
	}
	action, _ := extractDecisionAct(raw)
	row := AuditRow{Group: string(it.Group), Agent: agent, What: AuditDelivered, Item: it.Item, Home: home, Detail: "decided", By: wire.Cause.Key}
	if recordApprovalAudit(h, row) == nil {
		ackDecisionItem(h, home, it.Group, it.Item)
	}
	return true, action, nil
}

func applyLateApprovalDecision(h *Host, home, agent string, wire proto.InboxItem, it proto.Item) (string, error) {
	group, member := string(it.Group), string(it.AI)
	approvers, _ := resolveApproversAndMentions(h, group, member)
	act, ok := verifyApprovalAct(h, group, it.Msg, approvers, *wire.Cause)
	if !ok || !hasPendingApprovalRow(h.Store, string(it.Msg), group, member) {
		recordApprovalAudit(h, AuditRow{Group: group, Agent: agent, What: AuditRefused, Item: it.Item, Home: home, Detail: "invalid approval cause", By: wire.Cause.Key})
		ackDecisionItem(h, home, it.Group, it.Item)
		return "", &Error{Code: "forbidden", Message: "invalid approval cause"}
	}
	if err := commitLateDecision(h, home, agent, wire.Cause.Key, it, act.Action); err != nil {
		return "", err
	}
	ackDecisionItem(h, home, it.Group, it.Item)
	return act.Action, nil
}

func commitLateDecision(h *Host, home, agent, by string, it proto.Item, action string) error {
	work := func(tx *store.Tx) error {
		row := AuditRow{Group: string(it.Group), Agent: agent, What: AuditDelivered, Item: it.Item, Home: home, Detail: "decided", By: by}
		if _, err := h.Audit(tx, row); err != nil {
			return err
		}
		if action == "approve" {
			_, err := tx.Exec("UPDATE approval SET state = 'granted' WHERE card = ?", string(it.Msg))
			return err
		}
		_, err := tx.Exec("DELETE FROM approval WHERE card = ?", string(it.Msg))
		return err
	}
	return h.Store.Write(work)
}

// DeleteTaskApprovals removes every stored approval record of task in group,
// pending or granted, when the task is reassigned or ends (flows/approval.md D).
func (h *Host) DeleteTaskApprovals(group, task string) error {
	work := func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM approval WHERE g = ? AND task = ?", group, task)
		return err
	}
	return h.Store.Write(work)
}
