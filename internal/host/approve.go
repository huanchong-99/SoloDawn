package host

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	// approvalWaitUnit scales limits.approval_wait_s so unit tests can exercise expiry without 600-second sleeps.
	approvalWaitUnit   = time.Second
	approveInputSchema = json.RawMessage(`{"type":"object","properties":{"input":{"type":"object"},"tool_name":{"type":"string"},"tool_use_id":{"type":"string"}},"required":["tool_name","input","tool_use_id"],"additionalProperties":false}`)
)

// ApprovalResult is the MCP tool output of approve (contracts/mcp-tools.md section 3).
type ApprovalResult struct {
	Behavior     string         `json:"behavior"`
	Message      string         `json:"message,omitempty"`
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
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
	if st.cancels[key] == nil {
		st.cancels[key] = map[uint64]context.CancelFunc{}
	}
	st.cancels[key][st.nextID] = cancel
	return st.nextID
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
	if e.Kind == TurnBegan || e.Kind == TurnEnded {
		cancelSessionAsks(h, key)
		return
	}
	if e.Kind == Asked && e.Ask != nil {
		if s, ok := h.Sessions.Get(key); ok {
			startAskedGoroutine(h, s, *e.Ask)
		}
	}
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
	if ask.Reply != nil {
		answers := handleCodexQuestion(ctx, h, s, ask.Input)
		if ctx.Err() == nil {
			ask.Reply(answers)
		}
		return
	}
	allow, _ := handleApproval(ctx, h, s, resolveSessionLevel(h, s), ask)
	if ctx.Err() == nil && Runner != nil {
		Runner.Answer(h, s.SessionKey, ask.ID, allow)
	}
}

const querySessionLevelSQL = `SELECT
	coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'all_level3'), 0),
	coalesce((SELECT level FROM perm WHERE agent = ? AND g = ?), (SELECT json_extract(value, '$') FROM setting WHERE key = 'default_level'), 3)`

func resolveSessionLevel(h *Host, s Session) int {
	all3, level := 0, 3
	read := func(tx *store.Tx) error {
		return tx.QueryRow(querySessionLevelSQL, s.Agent, s.Group).Scan(&all3, &level)
	}
	h.Store.Write(read)
	if all3 != 0 {
		return 3
	}
	return level
}

type approveToolArgs struct {
	ToolName  *string         `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID *string         `json:"tool_use_id"`
}

func callApprove(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	a, err := decodeApproveArgs(raw)
	if err != nil {
		return nil, err
	}
	if *a.ToolName == "AskUserQuestion" {
		return handleClaudeAskUser(ctx, h, c.Session, a.Input)
	}
	ask, err := buildClaudeAsk(*a.ToolName, *a.ToolUseID, a.Input)
	if err != nil {
		return nil, err
	}
	if allow, msg := handleApproval(ctx, h, c.Session, c.Level, ask); allow {
		return ApprovalResult{Behavior: "allow"}, nil
	} else {
		return ApprovalResult{Behavior: "deny", Message: msg}, nil
	}
}

func decodeApproveArgs(raw json.RawMessage) (approveToolArgs, error) {
	if !isJSONObject(raw) {
		return approveToolArgs{}, &Error{Code: "bad_request", Message: "approve arguments must be a JSON object"}
	}
	var a approveToolArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&a) != nil || !hasApproveFields(a) {
		return approveToolArgs{}, &Error{Code: "bad_request", Message: "invalid approve arguments"}
	}
	return a, nil
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
	var buf bytes.Buffer
	json.Compact(&buf, input)
	ask.Category, ask.Command = claudeToolCategory(toolName), buf.String()
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

type cliAskInput struct {
	Questions []cliQuestion `json:"questions"`
}

type cliQuestion struct {
	ID          string           `json:"id"`
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []cliQuestionOpt `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
	IsSecret    bool             `json:"isSecret"`
}

type cliQuestionOpt struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func handleClaudeAskUser(ctx context.Context, h *Host, s Session, raw json.RawMessage) (any, error) {
	questions, ok := parseCLIQuestions(raw, false)
	if !ok {
		return nil, &Error{Code: "bad_request", Message: "invalid AskUserQuestion input"}
	}
	if hasQuestionSecret(questions) {
		recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: "AskUserQuestion", Result: "secret"})
		return ApprovalResult{Behavior: "deny", Message: denySecretMessage}, nil
	}
	answers, annotations := map[string]string{}, map[string]map[string]string{}
	for _, q := range questions {
		chosen, free, denyMsg, decided := postAndWaitDecision(ctx, h, s, q)
		if !decided {
			return ApprovalResult{Behavior: "deny", Message: denyMsg}, nil
		}
		recordClaudeAnswer(q.Question, chosen, free, answers, annotations)
	}
	return ApprovalResult{Behavior: "allow", UpdatedInput: buildUpdatedQuestionInput(raw, answers, annotations)}, nil
}

func handleCodexQuestion(ctx context.Context, h *Host, s Session, raw json.RawMessage) map[string][]string {
	questions, ok := parseCLIQuestions(raw, true)
	if !ok {
		return nil
	}
	if hasQuestionSecret(questions) {
		recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: "requestUserInput", Result: "secret"})
		return nil
	}
	answers := make(map[string][]string, len(questions))
	for _, q := range questions {
		chosen, free, _, decided := postAndWaitDecision(ctx, h, s, q)
		if !decided {
			return nil
		}
		answers[q.ID] = buildCodexAnswerValues(chosen, free)
	}
	return answers
}

func parseCLIQuestions(raw json.RawMessage, requireID bool) ([]cliQuestion, bool) {
	var in cliAskInput
	if json.Unmarshal(raw, &in) != nil || len(in.Questions) == 0 || hasBlankQuestion(in.Questions, requireID) {
		return nil, false
	}
	return in.Questions, true
}

func hasBlankQuestion(qs []cliQuestion, requireID bool) bool {
	for _, q := range qs {
		if q.Question == "" || (requireID && q.ID == "") {
			return true
		}
	}
	return false
}

func hasQuestionSecret(questions []cliQuestion) bool {
	for _, q := range questions {
		if questionFieldsSecret(q) || optionsHaveSecret(q.Options) {
			return true
		}
	}
	return false
}

func questionFieldsSecret(q cliQuestion) bool {
	return q.IsSecret || CheckOutboundSecret(q.Question) != nil || CheckOutboundSecret(q.Header) != nil
}

func optionsHaveSecret(opts []cliQuestionOpt) bool {
	for _, opt := range opts {
		if CheckOutboundSecret(opt.Label) != nil || CheckOutboundSecret(opt.Description) != nil {
			return true
		}
	}
	return false
}

func recordClaudeAnswer(question, chosen, free string, answers map[string]string, annotations map[string]map[string]string) {
	if chosen == "" {
		answers[question] = free
		return
	}
	answers[question] = chosen
	if free != "" {
		annotations[question] = map[string]string{"notes": free}
	}
}

func buildUpdatedQuestionInput(raw json.RawMessage, answers map[string]string, annotations map[string]map[string]string) map[string]any {
	updated := map[string]any{}
	json.Unmarshal(raw, &updated)
	updated["answers"] = answers
	if len(annotations) > 0 {
		updated["annotations"] = annotations
	}
	return updated
}

func buildCodexAnswerValues(chosen, free string) []string {
	vals := []string{}
	if chosen != "" {
		vals = append(vals, chosen)
	}
	if free != "" {
		vals = append(vals, free)
	}
	return vals
}

func optionID(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return strconv.Itoa(i + 1)
}

func buildDecisionOptions(in []cliQuestionOpt) []proto.DecisionOption {
	opts := make([]proto.DecisionOption, len(in))
	for i, opt := range in {
		opts[i] = proto.DecisionOption{ID: optionID(i), Label: opt.Label, Effect: opt.Description}
	}
	return opts
}

func buildDecisionPostPayload(h *Host, s Session, cardID proto.ID, q cliQuestion, opts []proto.DecisionOption, to []string, mentions []proto.ID) string {
	var thread *proto.ID
	var task, ctxText *string
	if s.Task != "" {
		id, t := proto.ID(s.Task), s.Task
		thread, task = &id, &t
	}
	if q.Header != "" {
		hdr := q.Header
		ctxText = &hdr
	}
	return marshalPostCard(h, s, cardID, "decision", thread, mentions, proto.DecisionBody{
		Task: task, Question: q.Question, Context: ctxText, Options: opts, To: to,
	})
}

func postAndWaitDecision(ctx context.Context, h *Host, s Session, q cliQuestion) (string, string, string, bool) {
	cardID := newApproveID()
	approvers, mentions := resolveApproversAndMentions(h, s.Group, s.Member)
	opts := buildDecisionOptions(q.Options)
	check := func(raw json.RawMessage) error { return checkOpenDecisionWait(s, cardID, raw) }
	payload := buildDecisionPostPayload(h, s, cardID, q, opts, approvers, mentions)
	card, denyMsg, ok := postCardAndWait(ctx, h, s, cardID, payload, q.Question, check)
	if !ok {
		return "", "", denyMsg, false
	}
	chosen, free := extractDecisionAnswer(card, opts)
	recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: q.Question, By: cardDecider(card, s.Home), Result: "decided"})
	return chosen, free, "", true
}

func extractDecisionAnswer(card proto.MsgRow, opts []proto.DecisionOption) (string, string) {
	chosen, free := "", ""
	if card.State.Choice != nil {
		chosen = findOptionLabel(opts, *card.State.Choice)
	}
	if card.State.Text != nil {
		free = *card.State.Text
	}
	return chosen, free
}

func findOptionLabel(opts []proto.DecisionOption, id string) string {
	for _, opt := range opts {
		if opt.ID == id {
			return opt.Label
		}
	}
	return ""
}

func handleApproval(ctx context.Context, h *Host, s Session, level int, ask Ask) (bool, string) {
	if level == 1 {
		return false, denyReadOnlyMessage
	}
	shortCmd, full := shortenCommand(ask.Command)
	digest := hashApprovalInput(ask.Input)
	if consumeGrantedApproval(ctx, h, s, ask.Tool, digest, shortCmd) {
		return true, ""
	}
	if CheckOutboundSecret(ask.Command) != nil || CheckOutboundSecret(ask.Why) != nil {
		recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: ask.Tool, Result: "secret"})
		return false, denySecretMessage
	}
	return postAndWaitApproval(ctx, h, s, ask, shortCmd, full, digest)
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
	if len(marshalApproveJSON(text)) <= limit {
		return text, false
	}
	cut := 0
	for {
		_, width := utf8.DecodeRuneInString(text[cut:])
		if len(marshalApproveJSON(text[:cut+width])) > limit {
			return text[:cut], true
		}
		cut += width
	}
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

const (
	deleteOneGrantedSQL = `DELETE FROM approval WHERE card = (
		SELECT card FROM approval
		WHERE g = ? AND member = ? AND task = ? AND tool = ? AND input = ? AND state = 'granted'
		ORDER BY rowid LIMIT 1)`
	selectPendingCardsSQL = `SELECT coalesce(json_group_array(card), '[]') FROM (
		SELECT card FROM approval
		WHERE g = ? AND member = ? AND task = ? AND tool = ? AND input = ? AND state = 'pending'
		ORDER BY rowid)`
)

func consumeGrantedApproval(ctx context.Context, h *Host, s Session, tool, digest, shortCmd string) bool {
	if deleteApprovalAndAudit(h, s, shortCmd, "", deleteOneGrantedSQL, s.Group, s.Member, s.Task, tool, digest) {
		return true
	}
	return consumeFetchedPendingApproval(ctx, h, s, tool, digest, shortCmd)
}

func deleteApprovalAndAudit(h *Host, s Session, shortCmd, by, query string, args ...any) bool {
	consumed := false
	work := func(tx *store.Tx) error {
		res, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		consumed = true
		_, err = h.Audit(tx, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: shortCmd, By: by, Result: "approved"})
		return err
	}
	err := h.Store.Write(work)
	return err == nil && consumed
}

func consumeFetchedPendingApproval(ctx context.Context, h *Host, s Session, tool, digest, shortCmd string) bool {
	home, ok := h.Home(s.Home)
	if !ok {
		return false
	}
	for _, cardID := range listPendingCards(h.Store, s, tool, digest) {
		if resolveFetchedPendingCard(ctx, h, home, s, cardID, shortCmd) {
			return true
		}
	}
	return false
}

func listPendingCards(db *store.Store, s Session, tool, digest string) []string {
	var raw string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(selectPendingCardsSQL, s.Group, s.Member, s.Task, tool, digest).Scan(&raw)
	}
	if db.Write(read) != nil {
		return nil
	}
	var cards []string
	json.Unmarshal([]byte(raw), &cards)
	return cards
}

func resolveFetchedPendingCard(ctx context.Context, h *Host, home Home, s Session, cardID, shortCmd string) bool {
	card, ok := fetchCardState(ctx, home, s.Group, cardID)
	if !ok {
		return false
	}
	if card.State.Status == "approved" {
		return deleteApprovalAndAudit(h, s, shortCmd, cardDecider(card, s.Home), "DELETE FROM approval WHERE card = ? AND state = 'pending'", cardID)
	}
	if isStaleApprovalStatus(card.State.Status) {
		deleteApprovalCard(h, cardID)
	}
	return false
}

func isStaleApprovalStatus(status string) bool {
	return status == "denied" || status == "expired" || status == "reassigned"
}

func fetchCardState(ctx context.Context, home Home, group, cardID string) (proto.MsgRow, bool) {
	build := func(id int64) any {
		return proto.FetchRow{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group, Limit: 1}, Row: "msg", RowID: cardID}
	}
	ans, err := home.Request(ctx, build)
	if err != nil {
		return proto.MsgRow{}, false
	}
	return decodeFetchedApproval(ans)
}

func decodeFetchedApproval(ans json.RawMessage) (proto.MsgRow, bool) {
	var fetched proto.Fetched
	var card proto.MsgRow
	if proto.Decode(ans, &fetched) != nil || len(fetched.Rows) == 0 {
		return card, false
	}
	return card, proto.Decode(fetched.Rows[0], &card) == nil && card.Kind == "approval" && card.State != nil
}

func postCardAndWait(ctx context.Context, h *Host, s Session, cardID proto.ID, payload, detail string, check func(json.RawMessage) error) (proto.MsgRow, string, bool) {
	itemCh, done := h.Await(string(cardID), check)
	defer done()
	if _, err := h.Command(ctx, s.Home, payload); err != nil {
		return proto.MsgRow{}, denyRefusedPrefix + err.Error(), false
	}
	recordApprovalAudit(h, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditRequested, Detail: detail})
	raw, ok := awaitApprovalItem(ctx, approvalWaitDuration(h, s.Group), itemCh, done)
	if !ok {
		return proto.MsgRow{}, denyTimeoutMessage, false
	}
	_, _, card, _ := decodeDecidedInbox(raw)
	return card, "", true
}

func postAndWaitApproval(ctx context.Context, h *Host, s Session, ask Ask, shortCmd string, full *int, digest string) (bool, string) {
	cardID := newApproveID()
	approvers, mentions := resolveApproversAndMentions(h, s.Group, s.Member)
	if storePendingApproval(h, cardID, s, ask.Tool, digest) != nil {
		return false, denyRefusedPrefix + "internal: failed to store approval record"
	}
	check := func(raw json.RawMessage) error { return checkOpenApprovalWait(h, s, cardID, raw) }
	payload := buildApprovalPostPayload(h, s, cardID, ask, shortCmd, full, approvers, mentions)
	card, denyMsg, ok := postCardAndWait(ctx, h, s, cardID, payload, shortCmd, check)
	if !ok {
		if denyMsg != denyTimeoutMessage {
			deleteApprovalCard(h, string(cardID))
		}
		return false, denyMsg
	}
	return finishTimelyDecision(h, s, string(cardID), shortCmd, card)
}

func newApproveID() proto.ID {
	var b [16]byte
	crand.Read(b[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(b[:]))
}

func storePendingApproval(h *Host, cardID proto.ID, s Session, tool, digest string) error {
	work := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO approval(card, g, member, task, tool, input, state) VALUES(?, ?, ?, ?, ?, ?, 'pending')", string(cardID), s.Group, s.Member, s.Task, tool, digest)
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

func marshalPostCard[B any](h *Host, s Session, cardID proto.ID, kind string, thread *proto.ID, mentions []proto.ID, body B) string {
	return marshalApproveJSON(proto.Post[B]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: cardID, Timestamp: h.Now().UnixMilli(), Group: proto.ID(s.Group), As: proto.ID(s.Member)},
		Kind:        kind, Thread: thread, Main: true, Mentions: mentions, Body: body,
	})
}

func buildApprovalPostPayload(h *Host, s Session, cardID proto.ID, ask Ask, shortCmd string, full *int, approvers []string, mentions []proto.ID) string {
	thread := proto.ID(s.Task)
	return marshalPostCard(h, s, cardID, "approval", &thread, mentions, proto.ApprovalBody{
		Task: s.Task, AI: s.Member, Tool: ask.Tool, Command: shortCmd, Full: full, Why: optionalWhy(ask.Why), Category: ask.Category, Approvers: approvers,
	})
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
	var ai proto.AIMemberRow
	if !ok || proto.Decode(raw, &ai) != nil || ai.Kind != "ai" || len(ai.Approvers) == 0 {
		return nil
	}
	return slices.Clone(ai.Approvers)
}

const queryApproversFallbackSQL = `SELECT
	coalesce((SELECT value FROM setting WHERE key = 'approvers'), '[]'),
	coalesce((SELECT value FROM setting WHERE key = 'owner'), '""')`

func readStoreApprovers(db *store.Store) []string {
	var listJSON, ownerJSON string
	read := func(tx *store.Tx) error { return tx.QueryRow(queryApproversFallbackSQL).Scan(&listJSON, &ownerJSON) }
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

func finishTimelyDecision(h *Host, s Session, cardID, shortCmd string, card proto.MsgRow) (bool, string) {
	status := card.State.Status
	by := cardDecider(card, s.Home)
	work := func(tx *store.Tx) error {
		tx.Exec("DELETE FROM approval WHERE card = ?", cardID)
		_, err := h.Audit(tx, AuditRow{Group: s.Group, Agent: s.Agent, What: AuditApproval, Detail: shortCmd, By: by, Result: status})
		return err
	}
	h.Store.Write(work)
	if status == "approved" {
		return true, ""
	}
	return false, denyOwnerMessage
}

func decodeDecidedInbox(raw json.RawMessage) (proto.InboxItem, proto.Item, proto.MsgRow, bool) {
	var wire proto.InboxItem
	var it proto.Item
	var card proto.MsgRow
	if proto.Decode(raw, &wire) != nil || proto.Decode([]byte(wire.Env.Payload), &it) != nil {
		return wire, it, card, false
	}
	return wire, it, card, isDecidedItem(it) && proto.Decode(wire.Msg, &card) == nil
}

func isDecidedItem(it proto.Item) bool {
	return it.Type == "item" && it.Item >= 1 && it.Why == "decided" && it.Msg != ""
}

func matchesWaitingSession(it proto.Item, s Session, cardID proto.ID) bool {
	return string(it.Group) == s.Group && string(it.AI) == s.Member && it.Msg == cardID
}

func isApprovalDecided(card proto.MsgRow) bool {
	return card.Kind == "approval" && card.State != nil && (card.State.Status == "approved" || card.State.Status == "denied")
}

func isDecisionDecided(card proto.MsgRow) bool {
	return card.Kind == "decision" && card.State != nil && card.State.Status == "decided"
}

func cardDecider(card proto.MsgRow, fallback string) string {
	if card.State.Act != nil && card.State.Act.Key != "" {
		return card.State.Act.Key
	}
	if card.State.By != nil && *card.State.By != "" {
		return *card.State.By
	}
	return fallback
}

func checkOpenApprovalWait(h *Host, s Session, cardID proto.ID, raw json.RawMessage) error {
	_, it, card, ok := decodeDecidedInbox(raw)
	if !ok || !matchesWaitingSession(it, s, cardID) || !isApprovalDecided(card) || !hasPendingApprovalRow(h.Store, string(cardID), s.Group, s.Member) {
		return &Error{Code: "bad_request", Message: "invalid approval item"}
	}
	return nil
}

func checkOpenDecisionWait(s Session, cardID proto.ID, raw json.RawMessage) error {
	_, it, card, ok := decodeDecidedInbox(raw)
	if !ok || !matchesWaitingSession(it, s, cardID) || !isDecisionDecided(card) {
		return &Error{Code: "bad_request", Message: "invalid decision item"}
	}
	return nil
}

func hasPendingApprovalRow(db *store.Store, card, group, member string) bool {
	var exists bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT EXISTS(SELECT 1 FROM approval WHERE card = ? AND g = ? AND member = ? AND state = 'pending')", card, group, member).Scan(&exists)
	}
	err := db.Write(read)
	return err == nil && exists
}

// RecordApprovalDecision handles a decided inbox item from home for an
// approval card: it hands the item to an open wait when the CLI is still
// waiting, or applies a late decision to the stored approval record in one
// transaction with its delivered audit row (flows/approval.md sections D and E).
// An item already in the audit record is acknowledged again without a second effect.
func (h *Host) RecordApprovalDecision(home string, raw json.RawMessage) (string, error) {
	it, card, ok := decodeApprovalDecision(h.Key.ID(), raw)
	if !ok {
		return "", &Error{Code: "bad_request", Message: "invalid decision item"}
	}
	if alreadyAuditedItem(h.Store, home, it.Item) {
		ackDecisionItem(h, home, it.Group, it.Item)
		return "", nil
	}
	agent := lookupApprovalAgent(h, string(it.Group), string(it.AI))
	by := cardDecider(card, home)
	if done, status, err := handOpenApproval(h, home, agent, by, it, card.State.Status, raw); done {
		return status, err
	}
	return applyLateApprovalDecision(h, home, agent, by, it, card.State.Status)
}

func decodeApprovalDecision(to string, raw json.RawMessage) (proto.Item, proto.MsgRow, bool) {
	_, it, card, ok := decodeDecidedInbox(raw)
	return it, card, ok && it.To == to && isApprovalDecided(card)
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

func handOpenApproval(h *Host, home, agent, by string, it proto.Item, status string, raw json.RawMessage) (bool, string, error) {
	waited, err := h.Hand(string(it.Msg), raw)
	if !waited || err != nil {
		return waited, "", err
	}
	row := AuditRow{Group: string(it.Group), Agent: agent, What: AuditDelivered, Item: it.Item, Home: home, Detail: "decided", By: by}
	if recordApprovalAudit(h, row) == nil {
		ackDecisionItem(h, home, it.Group, it.Item)
	}
	return true, status, nil
}

func applyLateApprovalDecision(h *Host, home, agent, by string, it proto.Item, status string) (string, error) {
	if !hasPendingApprovalRow(h.Store, string(it.Msg), string(it.Group), string(it.AI)) {
		return "", &Error{Code: "not_found", Message: "no pending approval record"}
	}
	if err := commitLateDecision(h, home, agent, by, it, status); err != nil {
		return "", err
	}
	ackDecisionItem(h, home, it.Group, it.Item)
	return status, nil
}

func commitLateDecision(h *Host, home, agent, by string, it proto.Item, status string) error {
	work := func(tx *store.Tx) error {
		row := AuditRow{Group: string(it.Group), Agent: agent, What: AuditDelivered, Item: it.Item, Home: home, Detail: "decided", By: by}
		if _, err := h.Audit(tx, row); err != nil {
			return err
		}
		if status == "approved" {
			_, err := tx.Exec("UPDATE approval SET state = 'granted' WHERE card = ?", string(it.Msg))
			return err
		}
		_, err := tx.Exec("DELETE FROM approval WHERE card = ?", string(it.Msg))
		return err
	}
	return h.Store.Write(work)
}

// DeleteTaskApprovals removes every stored approval record of task in group,
// pending or granted, when the task is reassigned or ends (flows/approval.md E).
func (h *Host) DeleteTaskApprovals(group, task string) error {
	work := func(tx *store.Tx) error {
		_, err := tx.Exec("DELETE FROM approval WHERE g = ? AND task = ?", group, task)
		return err
	}
	return h.Store.Write(work)
}
