package host

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	//go:embed instructions/group.md
	groupInstructions string
	//go:embed instructions/task.md
	taskInstructions string
	//go:embed instructions/review.md
	reviewInstructions string
	//go:embed instructions/secretary.md
	secretaryInstructions string
)

const (
	maxNoteBytes     = 8192
	notePageLimit    = 100
	historyLimit     = 20
	readOwnerSetting = `SELECT coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'owner'), '')`
)

func init() {
	Provide(&Instructions, instructionsFor)
	Provide(&Compose, composeTurn)
}

func instructionsFor(s Session) string {
	if s.Secretary {
		return secretaryInstructions
	}
	switch s.Kind {
	case TaskSession:
		return taskInstructions
	case ReviewSession:
		return reviewInstructions
	default:
		return groupInstructions
	}
}

type turnCtx struct {
	h         *Host
	s         Session
	kind      TurnKind
	owner     string
	itemTasks map[string]proto.TaskRow
}

type parsedItem struct {
	item proto.Item
	msg  *proto.MsgRow
}

type itemBlock struct {
	why       string
	whoPlace  string
	authority string
	callID    string
	body      string
}

type itemBuilder func(c *turnCtx, p parsedItem, first bool) (itemBlock, int, error)

var itemBuilders = map[string]itemBuilder{
	"mention":   (*turnCtx).buildMention,
	"reply":     (*turnCtx).buildReply,
	"assign":    (*turnCtx).buildAssign,
	"review":    (*turnCtx).buildReview,
	"reviewed":  (*turnCtx).buildReviewed,
	"checks":    (*turnCtx).buildChecks,
	"resume":    (*turnCtx).buildResume,
	"decided":   (*turnCtx).buildDecided,
	"confirmed": (*turnCtx).buildConfirmed,
	"notice":    (*turnCtx).buildNotice,
}

var msgFormatters = map[string]func(proto.MsgRow) (string, error){
	"review":   formatReviewMsg,
	"checks":   formatChecksMsg,
	"handback": formatHandbackMsg,
	"system":   formatSystemMsg,
}

func composeTurn(h *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
	c, err := newTurnCtx(h, s, kind)
	if err != nil {
		return "", Turn{}, err
	}
	switch kind {
	case ContinueTurn, HandbackTurn:
		return c.composeOwnTurn(items)
	case ItemsTurn, ReplacedTurn:
		return c.composeItemsTurn(items)
	default:
		return "", Turn{}, &Error{"bad_request", "unknown turn kind: " + string(kind)}
	}
}

func newTurnCtx(h *Host, s Session, kind TurnKind) (*turnCtx, error) {
	owner, err := loadOwner(h.Store)
	if err != nil {
		return nil, err
	}
	return &turnCtx{
		h:         h,
		s:         s,
		kind:      kind,
		owner:     owner,
		itemTasks: map[string]proto.TaskRow{},
	}, nil
}

func loadOwner(db *store.Store) (string, error) {
	var owner string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(readOwnerSetting).Scan(&owner)
	}
	err := db.Write(read)
	return owner, err
}

func (c *turnCtx) composeOwnTurn(items []json.RawMessage) (string, Turn, error) {
	if len(items) != 0 {
		return "", Turn{}, &Error{"bad_request", string(c.kind) + " turn takes no items"}
	}
	block := itemBlock{why: string(c.kind), authority: "instruction", body: c.ownTurnBody()}
	text := c.sessionHeader() + "\n" + renderBlock(1, 1, block)
	return text, Turn{Kind: c.kind, Thread: c.s.Task}, nil
}

func (c *turnCtx) ownTurnBody() string {
	if c.kind == ContinueTurn {
		if c.s.Kind == TaskSession {
			return "You were interrupted by a direct request from your owner. Continue the task."
		}
		return "You were interrupted by a direct request from your owner. Continue."
	}
	const base = "Your owner worked with you directly in the terminal. Post a short account of what was asked and what was done (`post`, kind `handback`)."
	if c.s.Kind == TaskSession {
		return base + " Then continue the task: if the work is complete, run the checks and submit; otherwise post what remains."
	}
	return base
}

func (c *turnCtx) composeItemsTurn(rawItems []json.RawMessage) (string, Turn, error) {
	if len(rawItems) == 0 {
		return c.composeTakeoverBootstrap()
	}
	parsed, err := c.parseItems(rawItems)
	if err != nil {
		return "", Turn{}, err
	}
	lines, err := c.turnHeadLines(parsed)
	if err != nil {
		return "", Turn{}, err
	}
	return c.appendParsedItems(lines, parsed)
}

func (c *turnCtx) composeTakeoverBootstrap() (string, Turn, error) {
	task, ok := c.lookupTask(c.s.Task)
	if c.kind != ItemsTurn || c.s.Kind != TaskSession || !ok || task.Origin != "takeover" {
		return "", Turn{}, &Error{"bad_request", "items turn requires at least one item"}
	}
	block := itemBlock{
		why:       "assign",
		authority: "instruction",
		body:      "Your owner is about to work with you directly in the terminal.",
	}
	text := c.sessionHeader() + "\n" + renderBlock(1, 1, block)
	return text, Turn{Kind: ItemsTurn, Thread: c.s.Task}, nil
}

func (c *turnCtx) turnHeadLines(parsed []parsedItem) ([]string, error) {
	lines := []string{c.sessionHeader()}
	if c.kind != ReplacedTurn {
		return lines, nil
	}
	ctxBlock, err := c.replacedContext(parsed)
	if err != nil {
		return nil, err
	}
	if ctxBlock != "" {
		lines = append(lines, ctxBlock)
	}
	return lines, nil
}

func (c *turnCtx) appendParsedItems(lines []string, parsed []parsedItem) (string, Turn, error) {
	turn := Turn{Kind: c.kind}
	for i, p := range parsed {
		block, depth, err := c.buildItem(p, i == 0)
		if err != nil {
			return "", Turn{}, err
		}
		lines = append(lines, renderBlock(i+1, len(parsed), block))
		turn.Depth = max(turn.Depth, depth)
		turn.Jump = turn.Jump || bool(p.item.Jump)
		turn.Thread = itemThread(string(p.item.Task), p.msg)
	}
	return strings.Join(lines, "\n"), turn, nil
}

func (c *turnCtx) parseItems(rawItems []json.RawMessage) ([]parsedItem, error) {
	out := make([]parsedItem, 0, len(rawItems))
	for _, raw := range rawItems {
		p, task, err := parseOneItem(raw)
		if err != nil {
			return nil, err
		}
		if task != nil {
			c.itemTasks[task.ID] = *task
		}
		out = append(out, p)
	}
	return out, nil
}

func decodeInboxItem(wire proto.InboxItem) (proto.Item, error) {
	var it proto.Item
	err := proto.Decode([]byte(wire.Env.Payload), &it)
	return it, err
}

func parseOneItem(raw json.RawMessage) (parsedItem, *proto.TaskRow, error) {
	var wire proto.InboxItem
	if proto.Decode(raw, &wire) != nil {
		return parsedItem{}, nil, &Error{"bad_request", "invalid inbox item"}
	}
	it, err := decodeInboxItem(wire)
	if err != nil || it.Type != "item" {
		return parsedItem{}, nil, &Error{"bad_request", "invalid inbox item"}
	}
	msg, err := decodeOptionalMsg(wire.Msg)
	if err != nil {
		return parsedItem{}, nil, err
	}
	return parsedItem{item: it, msg: msg}, wire.Task, nil
}

func decodeOptionalMsg(raw json.RawMessage) (*proto.MsgRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var msg proto.MsgRow
	if proto.Decode(raw, &msg) != nil {
		return nil, &Error{"bad_request", "invalid item message"}
	}
	return &msg, nil
}

func msgMatchesItem(id proto.ID, msg *proto.MsgRow) bool {
	if msg == nil {
		return id == ""
	}
	return string(id) == msg.ID
}

func (c *turnCtx) buildItem(p parsedItem, first bool) (itemBlock, int, error) {
	build, ok := itemBuilders[p.item.Why]
	if !ok {
		return itemBlock{}, 0, &Error{"bad_request", "unsupported item why: " + p.item.Why}
	}
	return build(c, p, first)
}

func renderBlock(index, total int, b itemBlock) string {
	parts := []string{fmt.Sprintf("== %d of %d", index, total), b.why}
	if b.whoPlace != "" {
		parts = append(parts, b.whoPlace)
	}
	parts = append(parts, b.authority)
	if b.callID != "" {
		parts = append(parts, "call id "+b.callID)
	}
	head := strings.Join(parts, " · ")
	if b.body == "" {
		return head
	}
	return head + "\n" + b.body
}

func (c *turnCtx) sessionHeader() string {
	return fmt.Sprintf("[SoloDawn] group %q · you are %q · %s", c.groupName(), c.s.Name, c.sessionLabel())
}

func (c *turnCtx) groupName() string {
	raw, _ := c.h.Rows.Get(c.s.Group, "group", c.s.Group)
	var g proto.GroupRow
	if proto.Decode(raw, &g) == nil {
		return g.Name
	}
	return c.s.Group
}

func (c *turnCtx) sessionLabel() string {
	if c.s.Secretary {
		return "secretary session"
	}
	switch c.s.Kind {
	case TaskSession:
		return "task session " + c.taskRef(c.s.Task)
	case ReviewSession:
		return "review session " + c.taskRef(c.s.Task)
	default:
		return "group session"
	}
}

func (c *turnCtx) lookupTask(id string) (proto.TaskRow, bool) {
	raw, _ := c.h.Rows.Get(c.s.Group, "task", id)
	var row proto.TaskRow
	if proto.Decode(raw, &row) == nil {
		return row, true
	}
	row, ok := c.itemTasks[id]
	return row, ok
}

func (c *turnCtx) lookupMember(id string) (proto.MemberRow, bool) {
	raw, _ := c.h.Rows.Get(c.s.Group, "member", id)
	var m proto.MemberRow
	return m, proto.Decode(raw, &m) == nil
}

func (c *turnCtx) isOwnerMember(id string) bool {
	m, _ := c.lookupMember(id)
	return m.Kind == "human" && m.User != "" && m.User == c.owner
}

func (c *turnCtx) taskRef(id string) string {
	if row, ok := c.lookupTask(id); ok {
		return "T-" + strconv.Itoa(row.Num)
	}
	return id
}

func itemThread(taskID string, msg *proto.MsgRow) string {
	if taskID != "" {
		return taskID
	}
	if msg != nil && msg.Thread != nil {
		return *msg.Thread
	}
	return ""
}

func (c *turnCtx) place(thread string) string {
	if thread == "" {
		return "in main"
	}
	return "in " + c.taskRef(thread)
}

func (c *turnCtx) fromIn(memberID, thread string) string {
	return "from " + c.memberLabel(memberID) + " " + c.place(thread)
}

func (c *turnCtx) memberLabel(id string) string {
	if m, ok := c.lookupMember(id); ok {
		return formatSpeaker(m.Name, m.Kind == "ai")
	}
	return formatSpeaker(id, false)
}

func (c *turnCtx) stateActorLabel(state *proto.CardState) string {
	if by := strVal(state.By); by != "" {
		return c.memberLabel(by)
	}
	return "unknown"
}

func formatSpeaker(name string, ai bool) string {
	if ai {
		return fmt.Sprintf("%q (AI)", name)
	}
	return fmt.Sprintf("%q", name)
}

func authorityLabel(instruct bool) string {
	if instruct {
		return "instruction"
	}
	return "information"
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func hasCardState(p parsedItem) bool {
	return p.msg != nil && string(p.item.Msg) == p.msg.ID && p.msg.State != nil
}

func appendAttachedMsg(lines []string, msg *proto.MsgRow) (string, error) {
	if msg != nil && msg.Kind != "task" {
		extra, err := formatMsgContent(*msg)
		if err != nil {
			return "", err
		}
		lines = append(lines, extra)
	}
	return strings.Join(lines, "\n"), nil
}

func (c *turnCtx) buildMention(p parsedItem, _ bool) (itemBlock, int, error) {
	if p.msg == nil || string(p.item.Msg) != p.msg.ID {
		return itemBlock{}, 0, &Error{"bad_request", "mention item requires matching msg"}
	}
	callID, depth, err := parseCallInfo(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	auth := authorityLabel(c.isOwnerMember(p.msg.By))
	who := c.fromIn(p.msg.By, itemThread(string(p.item.Task), p.msg))
	return itemBlock{why: "mention", whoPlace: who, authority: auth, callID: callID, body: p.msg.Text}, depth, nil
}

func parseCallInfo(msg proto.MsgRow) (string, int, error) {
	if msg.Kind != "call" {
		return "", 0, nil
	}
	var call proto.CallBody
	if proto.Decode(msg.Body, &call) != nil {
		return "", 0, &Error{"bad_request", "invalid call body"}
	}
	return msg.ID, call.Depth, nil
}

func (c *turnCtx) buildReply(p parsedItem, _ bool) (itemBlock, int, error) {
	if p.msg == nil || string(p.item.Msg) != p.msg.ID {
		return itemBlock{}, 0, &Error{"bad_request", "reply item requires matching msg"}
	}
	who := c.fromIn(p.msg.By, itemThread(string(p.item.Task), p.msg))
	return itemBlock{why: "reply", whoPlace: who, authority: "information", body: p.msg.Text}, 0, nil
}

func (c *turnCtx) buildAssign(p parsedItem, first bool) (itemBlock, int, error) {
	task, ok := c.lookupTask(string(p.item.Task))
	if !ok || !msgMatchesItem(p.item.Msg, p.msg) {
		return itemBlock{}, 0, &Error{"bad_request", "assign item requires task and matching msg"}
	}
	body, err := c.assignBody(task, p.msg, first)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: "assign", authority: "instruction", body: body}, 0, nil
}

func (c *turnCtx) assignBody(task proto.TaskRow, msg *proto.MsgRow, first bool) (string, error) {
	if c.isFirstTaskTurn(first) {
		return c.firstAssignBody(task, msg)
	}
	if msg != nil && msg.Kind != "task" {
		return formatMsgContent(*msg)
	}
	return fmt.Sprintf("Task T-%d %q assigned to you. Continue the task.", task.Num, task.Title), nil
}

func (c *turnCtx) isFirstTaskTurn(first bool) bool {
	return first && c.kind == ItemsTurn && c.s.Kind == TaskSession && c.s.CLISession == ""
}

func (c *turnCtx) firstAssignBody(task proto.TaskRow, msg *proto.MsgRow) (string, error) {
	if isTakeoverBootstrap(task, msg) {
		return "Your owner is about to work with you directly in the terminal.", nil
	}
	lines, err := c.taskStartLines(task)
	if err != nil {
		return "", err
	}
	return appendAttachedMsg(lines, msg)
}

func isTakeoverBootstrap(task proto.TaskRow, msg *proto.MsgRow) bool {
	return task.Origin == "takeover" && task.Thread.Count == 0 && (msg == nil || msg.Kind == "task")
}

func (c *turnCtx) taskStartLines(task proto.TaskRow) ([]string, error) {
	lines := []string{
		fmt.Sprintf("Task T-%d %q", task.Num, task.Title),
		fmt.Sprintf("Branch: task/%d-%s (this folder is its worktree)", task.Num, task.Slug),
	}
	spec, err := c.taskSpecLines(task)
	if err != nil {
		return nil, err
	}
	lines = append(lines, spec...)
	lines = append(lines, fmt.Sprintf("Reviewer: %s   Pass score: %d", c.reviewerName(task.Reviewer), task.Pass))
	notes, err := c.fetchNotes()
	if err != nil {
		return nil, err
	}
	return appendNotes(lines, notes), nil
}

func appendNotes(lines, notes []string) []string {
	if len(notes) == 0 {
		return lines
	}
	lines = append(lines, "Notes of this group:")
	for _, note := range notes {
		lines = append(lines, "- "+note)
	}
	return lines
}

func (c *turnCtx) reviewerName(id string) string {
	if id == "" {
		return "none"
	}
	if m, ok := c.lookupMember(id); ok {
		return m.Name
	}
	return id
}

func (c *turnCtx) taskSpecLines(task proto.TaskRow) ([]string, error) {
	if task.Origin == "brief" {
		return c.briefSpecLines(task.Brief)
	}
	if task.Plan != "" {
		return []string{"Plan: " + task.Plan}, nil
	}
	return nil, nil
}

func (c *turnCtx) briefSpecLines(briefID string) ([]string, error) {
	if briefID == "" {
		return nil, &Error{"bad_request", "brief task has no brief id"}
	}
	brief, err := c.fetchBrief(briefID)
	if err != nil {
		return nil, err
	}
	lines := []string{"Goal: " + brief.Goal, "Acceptance criteria:"}
	for i, criterion := range brief.Criteria {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, criterion))
	}
	return lines, nil
}

func (c *turnCtx) buildReview(p parsedItem, _ bool) (itemBlock, int, error) {
	task, ok := c.lookupTask(string(p.item.Task))
	if !ok || p.item.Commit == "" {
		return itemBlock{}, 0, &Error{"bad_request", "review item requires task and commit"}
	}
	lines, err := c.reviewLines(task, p.item.Commit)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: "review", authority: "instruction", body: strings.Join(lines, "\n")}, 0, nil
}

func (c *turnCtx) reviewLines(task proto.TaskRow, commit string) ([]string, error) {
	lines := []string{
		fmt.Sprintf("Task T-%d %q", task.Num, task.Title),
		"Commit: " + commit,
		"Diff: .solodawn-review.diff",
	}
	spec, err := c.taskSpecLines(task)
	if err != nil {
		return nil, err
	}
	lines = append(lines, spec...)
	lines = append(lines, "Pass score: "+strconv.Itoa(task.Pass))
	return lines, nil
}

func (c *turnCtx) buildReviewed(p parsedItem, _ bool) (itemBlock, int, error) {
	if p.item.Task == "" || p.msg == nil || string(p.item.Msg) != p.msg.ID || p.msg.Kind != "review" {
		return itemBlock{}, 0, &Error{"bad_request", "reviewed item requires review msg and task"}
	}
	body, err := formatReviewMsg(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	who := c.fromIn(p.msg.By, string(p.item.Task))
	return itemBlock{why: "reviewed", whoPlace: who, authority: "instruction", body: body}, 0, nil
}

func (c *turnCtx) buildChecks(p parsedItem, _ bool) (itemBlock, int, error) {
	if p.item.Task == "" || p.msg == nil || string(p.item.Msg) != p.msg.ID || p.msg.Kind != "checks" {
		return itemBlock{}, 0, &Error{"bad_request", "checks item requires checks msg and task"}
	}
	body, err := formatChecksMsg(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: "checks", authority: "instruction", body: body}, 0, nil
}

func (c *turnCtx) buildResume(p parsedItem, _ bool) (itemBlock, int, error) {
	if p.item.Task == "" {
		return itemBlock{}, 0, &Error{"bad_request", "resume item requires task"}
	}
	body := fmt.Sprintf("Task %s resumed. Continue the task.", c.taskRef(string(p.item.Task)))
	return itemBlock{why: "resume", authority: "instruction", body: body}, 0, nil
}

func (c *turnCtx) buildDecided(p parsedItem, _ bool) (itemBlock, int, error) {
	if !hasCardState(p) {
		return itemBlock{}, 0, &Error{"bad_request", "decided item requires card msg with state"}
	}
	body, err := formatDecidedBody(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	thread := itemThread(string(p.item.Task), p.msg)
	auth := authorityLabel(thread != "" || c.isOwnerMember(strVal(p.msg.State.By)))
	who := "from " + c.stateActorLabel(p.msg.State) + " " + c.place(thread)
	return itemBlock{why: "decided", whoPlace: who, authority: auth, body: body}, 0, nil
}

func formatDecidedBody(msg proto.MsgRow) (string, error) {
	switch msg.Kind {
	case "approval":
		return formatDecidedApproval(msg)
	case "decision":
		return formatDecidedDecision(msg)
	default:
		return "", &Error{"bad_request", "decided item requires approval or decision card"}
	}
}

func formatDecidedApproval(msg proto.MsgRow) (string, error) {
	var body proto.ApprovalBody
	if proto.Decode(msg.Body, &body) != nil {
		return "", &Error{"bad_request", "invalid approval body"}
	}
	switch msg.State.Status {
	case "approved":
		return "Approved. Run exactly the same request again.", nil
	case "denied":
		return "Denied.", nil
	default:
		return "", &Error{"bad_request", "invalid approval status"}
	}
}

func formatDecidedDecision(msg proto.MsgRow) (string, error) {
	var body proto.DecisionBody
	if msg.State.Status != "decided" || proto.Decode(msg.Body, &body) != nil {
		return "", &Error{"bad_request", "invalid decision card"}
	}
	return decisionLines(body, strVal(msg.State.Choice), strVal(msg.State.Text))
}

func decisionLines(body proto.DecisionBody, choice, text string) (string, error) {
	if choice == "" && text == "" {
		return "", &Error{"bad_request", "empty decision answer"}
	}
	lines := []string{"Decision: " + body.Question}
	if choice != "" {
		lines = append(lines, "Choice: "+decisionOptionLabel(body.Options, choice))
	}
	if text != "" {
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n"), nil
}

func decisionOptionLabel(options []proto.DecisionOption, id string) string {
	for _, opt := range options {
		if opt.ID == id {
			return fmt.Sprintf("%s (%s)", id, opt.Label)
		}
	}
	return id
}

func (c *turnCtx) buildConfirmed(p parsedItem, _ bool) (itemBlock, int, error) {
	if !hasCardState(p) || p.msg.Kind != "brief" {
		return itemBlock{}, 0, &Error{"bad_request", "confirmed item requires brief msg with state"}
	}
	verb, body, err := formatConfirmedBrief(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	writer := c.memberLabel(p.msg.By)
	actor := c.stateActorLabel(p.msg.State)
	who := fmt.Sprintf("from %s, %s %s in main", writer, verb, actor)
	auth := authorityLabel(c.isOwnerMember(strVal(p.msg.State.By)))
	return itemBlock{why: "confirmed", whoPlace: who, authority: auth, body: body}, 0, nil
}

func formatConfirmedBrief(msg proto.MsgRow) (string, string, error) {
	var brief proto.BriefBody
	if proto.Decode(msg.Body, &brief) != nil {
		return "", "", &Error{"bad_request", "invalid brief body"}
	}
	switch msg.State.Status {
	case "confirmed":
		return "confirmed by", fmt.Sprintf("Brief %q (id %s) confirmed.", brief.Title, msg.ID), nil
	case "revise":
		body := fmt.Sprintf("Brief %q (id %s) sent back for revision.", brief.Title, msg.ID)
		if note := strVal(msg.State.Note); note != "" {
			body += "\n" + note
		}
		return "sent back by", body, nil
	default:
		return "", "", &Error{"bad_request", "invalid brief status"}
	}
}

func (c *turnCtx) buildNotice(p parsedItem, _ bool) (itemBlock, int, error) {
	if p.item.Code == "" || !msgMatchesItem(p.item.Msg, p.msg) {
		return itemBlock{}, 0, &Error{"bad_request", "notice item requires code and matching msg"}
	}
	thread := itemThread(string(p.item.Task), p.msg)
	body, err := c.noticeBody(p.item.Code, p.item.Detail, thread, p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: "notice", whoPlace: c.noticeWho(p.msg, thread), authority: "information", body: body}, 0, nil
}

func (c *turnCtx) noticeWho(msg *proto.MsgRow, thread string) string {
	if msg != nil && msg.By != "system" {
		return c.fromIn(msg.By, thread)
	}
	return c.place(thread)
}

func (c *turnCtx) noticeBody(code, detail, thread string, msg *proto.MsgRow) (string, error) {
	lines := []string{c.noticeHeadline(code, thread)}
	if detail != "" {
		lines = append(lines, detail)
	}
	return appendAttachedMsg(lines, msg)
}

func (c *turnCtx) noticeHeadline(code, thread string) string {
	if thread == "" {
		return fmt.Sprintf("Notice (%s)", code)
	}
	if task, ok := c.lookupTask(thread); ok {
		return fmt.Sprintf("Notice (%s) on task T-%d %q", code, task.Num, task.Title)
	}
	return fmt.Sprintf("Notice (%s) on task %s", code, thread)
}

func formatMsgContent(msg proto.MsgRow) (string, error) {
	if fn, ok := msgFormatters[msg.Kind]; ok {
		return fn(msg)
	}
	if msg.Text != "" {
		return msg.Text, nil
	}
	return "(" + msg.Kind + ")", nil
}

func formatReviewMsg(msg proto.MsgRow) (string, error) {
	var body proto.ReviewBody
	if proto.Decode(msg.Body, &body) != nil {
		return "", &Error{"bad_request", "invalid review body"}
	}
	lines := []string{fmt.Sprintf("Review round %d on commit %s: score %d/%d", body.Round, body.Commit, body.Score, body.Pass)}
	for _, it := range body.Items {
		lines = append(lines, formatReviewItem(it))
	}
	if msg.Text != "" {
		lines = append(lines, msg.Text)
	}
	return strings.Join(lines, "\n"), nil
}

func formatReviewItem(it proto.ReviewItem) string {
	prefix := "- "
	if it.Veto {
		prefix += "[veto] "
	}
	if loc := formatReviewLoc(it.File, it.Line); loc != "" {
		prefix += loc + ": "
	}
	return prefix + it.Text
}

func formatReviewLoc(file *string, line *int) string {
	if file == nil {
		return ""
	}
	if line == nil {
		return *file
	}
	return fmt.Sprintf("%s:%d", *file, *line)
}

func formatChecksMsg(msg proto.MsgRow) (string, error) {
	var body proto.ChecksBody
	if proto.Decode(msg.Body, &body) != nil {
		return "", &Error{"bad_request", "invalid checks body"}
	}
	outcome := "failed"
	if body.OK {
		outcome = "passed"
	}
	lines := []string{fmt.Sprintf("Checks (%s) on commit %s: %s", body.Stage, body.Commit, outcome)}
	for _, it := range body.Items {
		lines = append(lines, formatCheckItem(it))
	}
	return strings.Join(lines, "\n"), nil
}

func formatCheckItem(it proto.CheckItem) string {
	res := "fail"
	if it.OK {
		res = "pass"
	}
	line := fmt.Sprintf("- %s: %s", it.Name, res)
	if detail := strVal(it.Detail); detail != "" {
		line += "\n" + detail
	}
	return line
}

func formatHandbackMsg(msg proto.MsgRow) (string, error) {
	var body proto.HandbackBody
	if proto.Decode(msg.Body, &body) != nil {
		return "", &Error{"bad_request", "invalid handback body"}
	}
	lines := []string{fmt.Sprintf("Changes: +%d -%d (%d files, %d commits)", body.Added, body.Removed, len(body.Files), len(body.Commits))}
	for _, cm := range body.Commits {
		lines = append(lines, "- "+cm.SHA+" "+cm.Subject)
	}
	if len(body.Files) != 0 {
		lines = append(lines, "Files: "+strings.Join(body.Files, ", "))
	}
	if msg.Text != "" {
		lines = append(lines, msg.Text)
	}
	return strings.Join(lines, "\n"), nil
}

func formatSystemMsg(msg proto.MsgRow) (string, error) {
	var sys proto.SystemCode
	if proto.Decode(msg.Body, &sys) != nil {
		return "", &Error{"bad_request", "invalid system body"}
	}
	return "System: " + sys.Code, nil
}

func (c *turnCtx) replacedContext(parsed []parsedItem) (string, error) {
	lines, err := c.replacedSessionLines(parsed)
	if err != nil {
		return "", err
	}
	history, err := c.fetchHistory()
	if err != nil {
		return "", err
	}
	historyLines, err := c.formatHistory(history)
	if err != nil {
		return "", err
	}
	lines = append(lines, historyLines...)
	if len(lines) == 0 {
		return "", nil
	}
	return "== context · instruction\n" + strings.Join(lines, "\n"), nil
}

func (c *turnCtx) replacedSessionLines(parsed []parsedItem) ([]string, error) {
	switch c.s.Kind {
	case TaskSession:
		task, ok := c.lookupTask(c.s.Task)
		if !ok {
			return nil, &Error{"bad_request", "replaced task session requires task row"}
		}
		return c.taskStartLines(task)
	case ReviewSession:
		return c.replacedReviewLines(parsed)
	default:
		return nil, nil
	}
}

func (c *turnCtx) replacedReviewLines(parsed []parsedItem) ([]string, error) {
	for _, p := range parsed {
		if p.item.Why == "review" {
			return nil, nil
		}
	}
	task, ok := c.lookupTask(c.s.Task)
	if !ok {
		return nil, &Error{"bad_request", "replaced review session requires task row"}
	}
	return c.reviewLines(task, task.Commit)
}

func (c *turnCtx) formatHistory(msgs []proto.MsgRow) ([]string, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	lines := []string{"Recent messages:"}
	for _, m := range msgs {
		entry, err := c.formatHistoryEntry(m)
		if err != nil {
			return nil, err
		}
		lines = append(lines, entry)
	}
	return lines, nil
}

func (c *turnCtx) formatHistoryEntry(m proto.MsgRow) (string, error) {
	content, err := formatMsgContent(m)
	if err != nil {
		return "", err
	}
	thread := itemThread("", &m)
	who := c.place(thread)
	if m.By != "system" {
		who = c.fromIn(m.By, thread)
	}
	auth := authorityLabel(c.isOwnerMember(m.By))
	return fmt.Sprintf("-- %s · %s · %s\n%s", m.Kind, who, auth, content), nil
}

func (c *turnCtx) fetchPage(id, before int64, limit int) proto.FetchPage {
	return proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.s.Group, Before: before, Limit: limit}
}

func (c *turnCtx) fetch(build func(id int64) any) (proto.Fetched, error) {
	unreachable := &Error{"home_unreachable", "the home of that group is not connected"}
	home, ok := c.h.Home(c.s.Home)
	if !ok {
		return proto.Fetched{}, unreachable
	}
	raw, err := home.Request(context.Background(), build)
	if err != nil {
		return proto.Fetched{}, unreachable
	}
	var refused proto.ErrorResponse
	if proto.Decode(raw, &refused) == nil {
		return proto.Fetched{}, &Error{refused.Err.Code, refused.Err.Message}
	}
	var fetched proto.Fetched
	if proto.Decode(raw, &fetched) != nil {
		return proto.Fetched{}, &Error{"bad_request", "invalid fetch response"}
	}
	return fetched, nil
}

func (c *turnCtx) fetchBrief(briefID string) (proto.BriefBody, error) {
	build := func(id int64) any {
		return proto.FetchRow{FetchPage: c.fetchPage(id, 0, 1), Row: "msg", RowID: briefID}
	}
	fetched, err := c.fetch(build)
	if err != nil {
		return proto.BriefBody{}, err
	}
	if len(fetched.Rows) == 0 {
		return proto.BriefBody{}, &Error{"not_found", "brief not found"}
	}
	var msg proto.MessageRow[proto.BriefBody, *proto.CardState]
	if proto.Decode(fetched.Rows[0], &msg) != nil || msg.Kind != "brief" {
		return proto.BriefBody{}, &Error{"bad_request", "invalid brief row"}
	}
	return msg.Body, nil
}

func (c *turnCtx) fetchNotes() ([]string, error) {
	var notes []string
	var before int64
	used := 0
	for {
		page, more, err := c.fetchNotePage(before)
		if err != nil {
			return nil, err
		}
		var full bool
		notes, used, before, full = takeNotes(notes, page, used)
		if full || !more || len(page) == 0 {
			return notes, nil
		}
	}
}

func (c *turnCtx) fetchNotePage(before int64) ([]proto.NoteRow, bool, error) {
	build := func(id int64) any {
		return proto.FetchKind{FetchPage: c.fetchPage(id, before, notePageLimit), Kind: "note"}
	}
	fetched, err := c.fetch(build)
	if err != nil {
		return nil, false, err
	}
	rows := make([]proto.NoteRow, len(fetched.Rows))
	for i, raw := range fetched.Rows {
		if proto.Decode(raw, &rows[i]) != nil {
			return nil, false, &Error{"bad_request", "invalid note row"}
		}
	}
	return rows, fetched.More, nil
}

func takeNotes(notes []string, page []proto.NoteRow, used int) ([]string, int, int64, bool) {
	var before int64
	for _, n := range page {
		if used+len(n.Text) > maxNoteBytes {
			return notes, used, before, true
		}
		used += len(n.Text)
		notes = append(notes, n.Text)
		before = n.Pos
	}
	return notes, used, before, false
}

func (c *turnCtx) fetchHistory() ([]proto.MsgRow, error) {
	var thread *string
	if c.s.Task != "" {
		task := c.s.Task
		thread = &task
	}
	build := func(id int64) any {
		return proto.FetchThread{FetchPage: c.fetchPage(id, 0, historyLimit), Thread: thread}
	}
	fetched, err := c.fetch(build)
	if err != nil {
		return nil, err
	}
	msgs := make([]proto.MsgRow, len(fetched.Rows))
	for i, raw := range fetched.Rows {
		if proto.Decode(raw, &msgs[i]) != nil {
			return nil, &Error{"bad_request", "invalid history message"}
		}
	}
	slices.Reverse(msgs)
	return msgs, nil
}
