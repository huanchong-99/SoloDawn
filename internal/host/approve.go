package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
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

var approveInputSchema = json.RawMessage(`{"type":"object","properties":{"input":{"type":"object"},"tool_name":{"type":"string"},"tool_use_id":{"type":"string"}},"required":["tool_name","input","tool_use_id"]}`)

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
	return (usesApproveTool(c.Agent) || c.External) && !c.Interactive
}

// approvalState tracks open Asked listener goroutines per host via Part[approvalState](h).
// Lock order: mu is a leaf lock; nothing called under mu acquires Host, Sessions, Rows or Store locks.
type approvalState struct {
	mu      sync.Mutex
	cancels map[SessionKey]map[*byte]context.CancelFunc
}

func (st *approvalState) registerAsk(key SessionKey, cancel context.CancelFunc) *byte {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.cancels == nil {
		st.cancels = map[SessionKey]map[*byte]context.CancelFunc{}
	}
	if st.cancels[key] == nil {
		st.cancels[key] = map[*byte]context.CancelFunc{}
	}
	id := new(byte)
	st.cancels[key][id] = cancel
	return id
}

func (st *approvalState) unregisterAsk(key SessionKey, id *byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.cancels[key], id)
	if len(st.cancels[key]) == 0 {
		delete(st.cancels, key)
	}
}

func cancelSessionAsks(h *Host, key SessionKey) {
	st := Part[approvalState](h)
	st.mu.Lock()
	active := st.cancels[key]
	delete(st.cancels, key)
	st.mu.Unlock()
	for _, cancel := range active {
		cancel()
	}
}

func listenApprove(h *Host, key SessionKey, e Event) {
	switch e.Kind {
	case TurnBegan, TurnEnded:
		cancelSessionAsks(h, key)
	case Asked:
		if s, ok := h.Sessions.Get(key); ok && e.Ask != nil {
			startAskedGoroutine(h, s, *e.Ask)
		}
	}
}

func startAskedGoroutine(h *Host, s Session, ask Ask) {
	callDone := h.Sessions.Calling(s.SessionKey)
	st := Part[approvalState](h)
	ctx, cancel := context.WithCancel(context.Background())
	id := st.registerAsk(s.SessionKey, cancel)
	h.bg(func(f func()) { go f() }, "ask", func() { runAskedEvent(ctx, cancel, h, st, s, id, ask, callDone) }, "g", s.Group, "task", s.Task)
}

func runAskedEvent(ctx context.Context, cancel context.CancelFunc, h *Host, st *approvalState, s Session, id *byte, ask Ask, callDone func()) {
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
	allow, _ := handleApproval(ctx, h, s, ask)
	if ctx.Err() == nil && Runner != nil {
		Runner.Answer(h, s.SessionKey, ask.ID, allow)
	}
}

type approveToolArgs struct {
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
}

func callApprove(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	a, err := decodeApproveArgs(raw)
	if err != nil {
		return nil, err
	}
	if a.ToolName == "AskUserQuestion" {
		return handleClaudeAskUser(ctx, h, c.Session, a.Input)
	}
	ask, err := buildClaudeAsk(a.ToolName, a.ToolUseID, a.Input)
	if err != nil {
		return nil, err
	}
	if c.Level == 1 {
		return ApprovalResult{Behavior: "deny", Message: denyReadOnlyMessage}, nil
	}
	return approvalResult(handleApproval(ctx, h, c.Session, ask)), nil
}

func approvalResult(allow bool, msg string) ApprovalResult {
	if allow {
		return ApprovalResult{Behavior: "allow"}
	}
	return ApprovalResult{Behavior: "deny", Message: msg}
}

func decodeApproveArgs(raw json.RawMessage) (approveToolArgs, error) {
	var a approveToolArgs
	if json.Unmarshal(raw, &a) != nil || a.ToolName == "" {
		return approveToolArgs{}, &Error{Code: "bad_request", Message: "invalid approve arguments"}
	}
	return a, nil
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
	if json.Compact(&buf, input) != nil {
		return Ask{}, &Error{Code: "bad_request", Message: "invalid tool input"}
	}
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
	if !ok || hasQuestionSecret(questions) {
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

func approveDecisionOptions(in []cliQuestionOpt) []proto.DecisionOption {
	opts := make([]proto.DecisionOption, len(in))
	for i, opt := range in {
		opts[i] = decisionOption(i, opt.Label, opt.Description)
	}
	return opts
}

func sessionCardThread(s Session) (*proto.ID, *string, string) {
	if s.Kind == TaskSession || s.Kind == ReviewSession {
		id, task := proto.ID(s.Task), s.Task
		return &id, &task, s.Task
	}
	return nil, nil, ""
}

func buildDecisionPostPayload(h *Host, s Session, cardID proto.ID, q cliQuestion, opts []proto.DecisionOption, to []string, mentions []proto.ID) string {
	thread, task, _ := sessionCardThread(s)
	var ctxText *string
	if q.Header != "" {
		hdr := q.Header
		ctxText = &hdr
	}
	return marshalPostCard(h, s, cardID, "decision", thread, mentions, proto.DecisionBody{
		Task: task, Question: q.Question, Context: ctxText, Options: opts, To: to,
	})
}

func postAndWaitDecision(ctx context.Context, h *Host, s Session, q cliQuestion) (string, string, string, bool) {
	cardID := proto.NewNonce()
	approvers, mentions := resolveApproversAndMentions(h, s.Group, s.Member)
	opts := approveDecisionOptions(q.Options)
	payload := buildDecisionPostPayload(h, s, cardID, q, opts, approvers, mentions)
	state, denyMsg, ok := postCardAndWait(ctx, h, s, cardID, payload, isDecisionStatus)
	if !ok {
		return "", "", denyMsg, false
	}
	chosen, free := extractDecisionAnswer(state, opts)
	return chosen, free, "", true
}

func extractDecisionAnswer(state *proto.CardState, opts []proto.DecisionOption) (string, string) {
	chosen, free := "", ""
	if state.Choice != nil {
		chosen = findOptionLabel(opts, *state.Choice)
	}
	if state.Text != nil {
		free = *state.Text
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

func handleApproval(ctx context.Context, h *Host, s Session, ask Ask) (bool, string) {
	digest := hashApprovalInput(ask.Input)
	if consumePendingApproval(ctx, h, s, ask.Tool, digest) {
		return true, ""
	}
	if CheckOutboundSecret(ask.Command) != nil || CheckOutboundSecret(ask.Why) != nil {
		return false, denySecretMessage
	}
	shortCmd, full := shortenCommand(ask.Command)
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
	if len(marshalWire(text)) <= limit {
		return text, false
	}
	cut := 0
	for {
		_, width := utf8.DecodeRuneInString(text[cut:])
		if len(marshalWire(text[:cut+width])) > limit {
			return text[:cut], true
		}
		cut += width
	}
}

func hashApprovalInput(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var val any
	data := bytes.TrimSpace(raw)
	if dec.Decode(&val) == nil {
		data = []byte(marshalWire(val))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const selectPendingCardsSQL = `SELECT coalesce(json_group_array(card), '[]') FROM (
	SELECT card FROM approval
	WHERE g = ? AND member = ? AND task = ? AND tool = ? AND input = ? AND state = 'pending'
	ORDER BY rowid)`

func consumePendingApproval(ctx context.Context, h *Host, s Session, tool, digest string) bool {
	home, ok := h.Home(s.Home)
	if !ok {
		return false
	}
	for _, cardID := range listPendingCards(h.Store, s, tool, digest) {
		if resolveFetchedPendingCard(ctx, h, home, s.Group, cardID) {
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

func resolveFetchedPendingCard(ctx context.Context, h *Host, home Home, group, cardID string) bool {
	card, ok := fetchCardState(ctx, home, group, cardID)
	if !ok {
		return false
	}
	if card.State.Status == "approved" {
		return deletePendingCard(h, cardID)
	}
	if isStaleApprovalStatus(card.State.Status) {
		deleteApprovalCard(h, cardID)
	}
	return false
}

func deletePendingCard(h *Host, cardID string) bool {
	var deleted int64
	work := func(tx *store.Tx) error {
		res, err := tx.Exec("DELETE FROM approval WHERE card = ? AND state = 'pending'", cardID)
		if err != nil {
			return err
		}
		deleted, _ = res.RowsAffected()
		return nil
	}
	h.Store.Write(work)
	return deleted != 0
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

func postCardAndWait(ctx context.Context, h *Host, s Session, cardID proto.ID, payload string, decided func(string) bool) (*proto.CardState, string, bool) {
	itemCh, done := h.Await(string(cardID), nil)
	defer done()
	if _, err := h.Command(ctx, s.Home, payload); err != nil {
		return nil, denyRefusedPrefix + err.Error(), false
	}
	raw, ok := awaitApprovalItem(ctx, approvalWaitDuration(h, s.Group), itemCh, done)
	if !ok {
		return nil, denyTimeoutMessage, false
	}
	card, ok := decodeDecidedInbox(raw)
	if !ok || !decided(card.State.Status) {
		return nil, denyTimeoutMessage, false
	}
	return card.State, "", true
}

func postAndWaitApproval(ctx context.Context, h *Host, s Session, ask Ask, shortCmd string, full *int, digest string) (bool, string) {
	cardID := proto.NewNonce()
	approvers, mentions := resolveApproversAndMentions(h, s.Group, s.Member)
	if storePendingApproval(h, cardID, s, ask.Tool, digest) != nil {
		return false, denyRefusedPrefix + "internal: failed to store approval record"
	}
	payload := buildApprovalPostPayload(h, s, cardID, ask, shortCmd, full, approvers, mentions)
	state, denyMsg, ok := postCardAndWait(ctx, h, s, cardID, payload, isApprovalStatus)
	if !ok {
		if denyMsg != denyTimeoutMessage {
			deleteApprovalCard(h, string(cardID))
		}
		return false, denyMsg
	}
	return finishTimelyDecision(h, string(cardID), state.Status)
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

func marshalPostCard[B any](h *Host, s Session, cardID proto.ID, kind string, thread *proto.ID, mentions []proto.ID, body B) string {
	return marshalWire(proto.Post[B]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: cardID, Timestamp: h.Now().UnixMilli(), Group: proto.ID(s.Group), As: proto.ID(s.Member)},
		Kind:        kind, Thread: thread, Main: true, Mentions: mentions, Body: body,
	})
}

func buildApprovalPostPayload(h *Host, s Session, cardID proto.ID, ask Ask, shortCmd string, full *int, approvers []string, mentions []proto.ID) string {
	thread, _, task := sessionCardThread(s)
	return marshalPostCard(h, s, cardID, "approval", thread, mentions, proto.ApprovalBody{
		Task: task, AI: s.Member, Tool: ask.Tool, Command: shortCmd, Full: full, Why: optionalWhy(ask.Why), Category: ask.Category, Approvers: approvers,
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
	for _, m := range liveMembers(rows, group) {
		if m.Kind == "human" && slices.Contains(approvers, m.User) {
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
	return time.Duration(sec) * time.Second
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

func finishTimelyDecision(h *Host, cardID, status string) (bool, string) {
	deleteApprovalCard(h, cardID)
	if status == "approved" {
		return true, ""
	}
	return false, denyOwnerMessage
}

func decodeDecidedInbox(raw json.RawMessage) (proto.MsgRow, bool) {
	var wire proto.InboxItem
	var card proto.MsgRow
	if proto.Decode(raw, &wire) != nil || proto.Decode(wire.Msg, &card) != nil {
		return card, false
	}
	return card, card.State != nil
}

func isApprovalStatus(status string) bool {
	return status == "approved" || status == "denied"
}

func isDecisionStatus(status string) bool {
	return status == "decided"
}
