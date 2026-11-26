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
	h               *Host
	s               Session
	kind            TurnKind
	owner           string
	itemTasks       map[string]proto.TaskRow
	confirmations   []proto.MsgRow
	confirmsFetched bool
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
	"reviewed":  (*turnCtx).buildCarried,
	"resume":    (*turnCtx).buildResume,
	"decided":   (*turnCtx).buildDecided,
	"confirmed": (*turnCtx).buildConfirmed,
	"notice":    (*turnCtx).buildNotice,
}

var msgFormatters = map[string]func(proto.MsgRow) (string, error){
	"brief":    formatBriefMsg,
	"review":   formatReviewMsg,
	"system":   formatSystemMsg,
	"decision": formatDecisionMsg,
	"approval": formatApprovalMsg,
}

func composeTurn(h *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
	c, err := newTurnCtx(h, s, kind)
	if err != nil {
		return "", Turn{}, err
	}
	switch kind {
	case ContinueTurn, HandbackTurn, ReplacedTurn, TakeoverTurn:
		return c.composeNoItemsTurn(items)
	case ItemsTurn:
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

func (c *turnCtx) composeNoItemsTurn(items []json.RawMessage) (string, Turn, error) {
	if len(items) != 0 {
		return "", Turn{}, &Error{"bad_request", string(c.kind) + " turn takes no items"}
	}
	if c.kind == ReplacedTurn {
		return c.composeReplaced()
	}
	block := itemBlock{why: string(c.kind), authority: "instruction", body: c.ownTurnBody()}
	text := c.sessionHeader() + "\n" + renderBlock(1, 1, block)
	return text, Turn{Kind: c.kind, Thread: c.s.Task}, nil
}

func (c *turnCtx) ownTurnBody() string {
	if c.kind == TakeoverTurn {
		return "Your owner is about to work with you directly in the terminal."
	}
	if c.kind == ContinueTurn {
		if c.s.Kind == TaskSession {
			return "You were interrupted by a direct request from your owner. Continue the task."
		}
		return "You were interrupted by a direct request from your owner. Continue."
	}
	const base = "Your owner worked with you directly in the terminal. Post a short account of what was asked and what was done (`post`, kind `handback`)."
	if c.s.Kind == TaskSession {
		return base + " Then continue the task: if the work is complete, submit; otherwise post what remains."
	}
	return base
}

func (c *turnCtx) composeReplaced() (string, Turn, error) {
	ctx, err := c.replacedContext()
	if err != nil {
		return "", Turn{}, err
	}
	return c.sessionHeader() + "\n" + ctx, Turn{Kind: ReplacedTurn, Thread: c.s.Task}, nil
}

func (c *turnCtx) composeItemsTurn(rawItems []json.RawMessage) (string, Turn, error) {
	if len(rawItems) == 0 {
		return "", Turn{}, &Error{"bad_request", "items turn requires at least one item"}
	}
	parsed, err := c.parseItems(rawItems)
	if err != nil {
		return "", Turn{}, err
	}
	return c.renderItems(parsed)
}

func (c *turnCtx) renderItems(parsed []parsedItem) (string, Turn, error) {
	lines := []string{c.sessionHeader()}
	turn := Turn{Kind: ItemsTurn}
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

func parseOneItem(raw json.RawMessage) (parsedItem, *proto.TaskRow, error) {
	var wire proto.InboxItem
	if proto.Decode(raw, &wire) != nil {
		return parsedItem{}, nil, &Error{"bad_request", "invalid inbox item"}
	}
	msg, err := decodeOptionalMsg(wire.Msg)
	if err != nil {
		return parsedItem{}, nil, err
	}
	return parsedItem{item: wire.Item, msg: msg}, wire.Task, nil
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
	header := fmt.Sprintf("[SoloDawn] group %q · you are %q · %s", c.groupName(), c.s.Name, c.sessionLabel())
	if c.s.Secretary {
		return header + "\n" + c.profileRoster()
	}
	return header
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

func (c *turnCtx) msgWho(msg *proto.MsgRow, thread string) string {
	if msg != nil {
		return c.fromIn(msg.By, thread)
	}
	return c.place(thread)
}

func (c *turnCtx) memberLabel(id string) string {
	switch id {
	case "":
		return "unknown"
	case "system":
		return "the home"
	}
	if m, ok := c.lookupMember(id); ok {
		return formatSpeaker(m.Name, m.Kind == "ai")
	}
	return formatSpeaker(id, false)
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

func msgText(msg *proto.MsgRow) string {
	if msg == nil {
		return ""
	}
	return msg.Text
}

func cardStatus(state *proto.CardState) string {
	if state == nil {
		return ""
	}
	return state.Status
}

func stateBy(state *proto.CardState) string {
	if state == nil {
		return ""
	}
	return strVal(state.By)
}

func (c *turnCtx) buildMention(p parsedItem, _ bool) (itemBlock, int, error) {
	callID, depth, err := parseCallInfo(p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	thread := itemThread(string(p.item.Task), p.msg)
	auth := authorityLabel(c.mentionInstruct(p.msg, thread))
	return itemBlock{why: "mention", whoPlace: c.msgWho(p.msg, thread), authority: auth, callID: callID, body: msgText(p.msg)}, depth, nil
}

func parseCallInfo(msg *proto.MsgRow) (string, int, error) {
	if msg == nil || msg.Kind != "call" {
		return "", 0, nil
	}
	var call proto.CallBody
	if proto.Decode(msg.Body, &call) != nil {
		return "", 0, &Error{"bad_request", "invalid call body"}
	}
	return msg.ID, call.Depth, nil
}

func (c *turnCtx) buildReply(p parsedItem, _ bool) (itemBlock, int, error) {
	thread := itemThread(string(p.item.Task), p.msg)
	return itemBlock{why: "reply", whoPlace: c.msgWho(p.msg, thread), authority: "information", body: c.replyBody(p.msg)}, 0, nil
}

// replyBody is the text of a reply, or what the confirm card says by which
// the callee's owner denied the call.
func (c *turnCtx) replyBody(msg *proto.MsgRow) string {
	if msg == nil || msg.Kind != "confirm" {
		return msgText(msg)
	}
	return fmt.Sprintf("Your call (id %s) was denied by %s.", strVal(msg.Re), c.memberLabel(stateBy(msg.State)))
}

func (c *turnCtx) buildAssign(p parsedItem, first bool) (itemBlock, int, error) {
	task, ok := c.lookupTask(string(p.item.Task))
	if !ok {
		return itemBlock{}, 0, &Error{"bad_request", "assign item requires task"}
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
		return c.formatCarriedMsg(*msg)
	}
	return fmt.Sprintf("Task T-%d %q assigned to you. Continue the task.", task.Num, task.Title), nil
}

func (c *turnCtx) isFirstTaskTurn(first bool) bool {
	return first && c.s.Kind == TaskSession && c.s.CLISession == ""
}

func (c *turnCtx) firstAssignBody(task proto.TaskRow, msg *proto.MsgRow) (string, error) {
	lines, err := c.taskStartLines(task)
	if err != nil {
		return "", err
	}
	if msg != nil && msg.Kind == "task" {
		msg = nil
	}
	return c.appendCarriedMsg(lines, msg)
}

func (c *turnCtx) taskStartLines(task proto.TaskRow) ([]string, error) {
	lines := []string{
		fmt.Sprintf("Task T-%d %q", task.Num, task.Title),
		fmt.Sprintf("Folder: %s (your workspace in this group)", c.s.Folder),
	}
	spec, preset, err := c.taskSpecLines(task)
	if err != nil {
		return nil, err
	}
	lines = append(lines, spec...)
	lines = append(lines, fmt.Sprintf("Reviewer: %s   Pass score: %d", c.reviewerName(task.Reviewer), task.Pass))
	lines, err = c.appendTaskPreset(lines, preset)
	if err != nil {
		return nil, err
	}
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

func (c *turnCtx) taskSpecLines(task proto.TaskRow) ([]string, string, error) {
	if task.Origin == "brief" {
		return c.briefSpecLines(task.Brief)
	}
	if task.Plan != "" {
		return []string{"Plan: " + task.Plan}, "", nil
	}
	return nil, "", nil
}

func (c *turnCtx) briefSpecLines(briefID string) ([]string, string, error) {
	brief, err := c.fetchBrief(briefID)
	if err != nil {
		return nil, "", err
	}
	return briefLines(brief), brief.Preset, nil
}

func briefLines(brief proto.BriefBody) []string {
	lines := []string{"Goal: " + brief.Goal, "Acceptance criteria:"}
	for i, criterion := range brief.Criteria {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, criterion))
	}
	lines = appendScope(lines, "In scope: ", brief.Scope.In)
	return appendScope(lines, "Out of scope: ", brief.Scope.Out)
}

func appendScope(lines []string, label string, scope []string) []string {
	if len(scope) == 0 {
		return lines
	}
	return append(lines, label+strings.Join(scope, "; "))
}

func formatBriefMsg(msg proto.MsgRow) (string, error) {
	var brief proto.BriefBody
	if proto.Decode(msg.Body, &brief) != nil {
		return "", &Error{"bad_request", "invalid brief body"}
	}
	lines := []string{fmt.Sprintf("Brief %q (id %s): %s", brief.Title, msg.ID, cardStatus(msg.State))}
	lines = append(append(lines, briefLines(brief)...), "Tasks:")
	for i, task := range brief.Tasks {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, task.Title))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *turnCtx) buildReview(p parsedItem, _ bool) (itemBlock, int, error) {
	task, ok := c.lookupTask(string(p.item.Task))
	if !ok || p.msg == nil {
		return itemBlock{}, 0, &Error{"bad_request", "review item requires task and report"}
	}
	lines, err := c.reviewLines(task, p.msg.By)
	if err != nil {
		return itemBlock{}, 0, err
	}
	body, err := c.appendCarriedMsg(lines, p.msg)
	return itemBlock{why: "review", authority: "instruction", body: body}, 0, err
}

// reviewLines is what a review turn says before the report: the task's
// request, the pass score, and where the work of the report's author is.
func (c *turnCtx) reviewLines(task proto.TaskRow, author string) ([]string, error) {
	lines := []string{fmt.Sprintf("Task T-%d %q", task.Num, task.Title)}
	spec, preset, err := c.taskSpecLines(task)
	if err != nil {
		return nil, err
	}
	lines = append(lines, spec...)
	lines = append(lines, "Pass score: "+strconv.Itoa(task.Pass))
	return append(appendReviewPreset(lines, preset), c.workLines(author)...), nil
}

// workLines says where the work under review is: the name, the computer and
// the folder of the member who wrote the report, read from that member's row,
// and how that folder lies for this session (contracts/drivers.md section 3).
// The author may be another AI than the task's assignee is now. A member who
// has left the group has no row, and no line.
func (c *turnCtx) workLines(author string) []string {
	ai, ok := lookupRow[proto.AIMemberRow](c.h, c.s.Group, "member", author)
	if !ok {
		return nil
	}
	return []string{fmt.Sprintf("Work: by %q on %s, in %s", ai.Name, ai.HostName, ai.Workspace), c.workPlace(ai)}
}

func (c *turnCtx) workPlace(ai proto.AIMemberRow) string {
	switch {
	case ai.Host != c.h.Key.ID():
		return "The work is on another computer. Get it the way the report says it is shared (with Git and a remote: fetch the branch the report names), and read it without changing what this folder holds."
	case ai.Workspace != c.s.Folder:
		return "The work is in another folder on this computer. Read it there; change nothing in it."
	}
	return "The work is in this folder."
}

func (c *turnCtx) buildCarried(p parsedItem, _ bool) (itemBlock, int, error) {
	body, err := c.appendCarriedMsg(nil, p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: p.item.Why, authority: "instruction", body: c.appendEscalation(body, p)}, 0, nil
}

func (c *turnCtx) buildResume(p parsedItem, _ bool) (itemBlock, int, error) {
	body := fmt.Sprintf("Task %s resumed. Continue the task.", c.taskRef(string(p.item.Task)))
	return itemBlock{why: "resume", authority: "instruction", body: body}, 0, nil
}

func (c *turnCtx) buildDecided(p parsedItem, _ bool) (itemBlock, int, error) {
	thread := itemThread(string(p.item.Task), p.msg)
	auth := authorityLabel(c.takesPart(thread) || (p.msg != nil && p.msg.By == c.s.Member))
	if p.msg == nil {
		return itemBlock{why: "decided", authority: auth}, 0, nil
	}
	content, err := formatMsgContent(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: "decided", authority: auth, body: c.formatCarriedDecision(*p.msg, content)}, 0, nil
}

func formatApprovalMsg(msg proto.MsgRow) (string, error) {
	switch cardStatus(msg.State) {
	case "approved":
		return "Approved. Run exactly the same request again.", nil
	case "denied":
		return "Denied.", nil
	default:
		return "(approval)", nil
	}
}

func formatDecisionMsg(msg proto.MsgRow) (string, error) {
	var body proto.DecisionBody
	if proto.Decode(msg.Body, &body) != nil {
		return "", &Error{"bad_request", "invalid decision card"}
	}
	lines := []string{fmt.Sprintf("Decision (id %s): %s", msg.ID, body.Question)}
	if cardStatus(msg.State) != "decided" {
		return strings.Join(appendOptions(lines, body.Options), "\n"), nil
	}
	if choice := strVal(msg.State.Choice); choice != "" {
		lines = append(lines, "Choice: "+decisionOptionLabel(body.Options, choice))
	}
	if text := strVal(msg.State.Text); text != "" {
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n"), nil
}

func appendOptions(lines []string, options []proto.DecisionOption) []string {
	for _, opt := range options {
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", opt.ID, opt.Label, opt.Effect))
	}
	return lines
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
	if p.msg == nil {
		return itemBlock{why: "confirmed", authority: "information"}, 0, nil
	}
	content, err := formatConfirmedBrief(*p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	auth := authorityLabel(p.msg.By == c.s.Member)
	return itemBlock{why: "confirmed", authority: auth, body: c.formatCarriedDecision(*p.msg, content)}, 0, nil
}

func formatConfirmedBrief(msg proto.MsgRow) (string, error) {
	var brief proto.BriefBody
	if proto.Decode(msg.Body, &brief) != nil {
		return "", &Error{"bad_request", "invalid brief body"}
	}
	if cardStatus(msg.State) == "revise" {
		body := fmt.Sprintf("Brief %q (id %s) sent back for revision.", brief.Title, msg.ID)
		if note := strVal(msg.State.Note); note != "" {
			body += "\n" + note
		}
		return body, nil
	}
	return fmt.Sprintf("Brief %q (id %s) confirmed.", brief.Title, msg.ID), nil
}

func (c *turnCtx) buildNotice(p parsedItem, _ bool) (itemBlock, int, error) {
	thread := itemThread(string(p.item.Task), p.msg)
	body, err := c.noticeBody(p.item.Code, p.item.Detail, thread, p.msg)
	if err != nil {
		return itemBlock{}, 0, err
	}
	return itemBlock{why: "notice", authority: "information", body: body}, 0, nil
}

func (c *turnCtx) noticeBody(code, detail, thread string, msg *proto.MsgRow) (string, error) {
	lines := []string{c.noticeHeadline(code, thread)}
	if detail != "" {
		lines = append(lines, detail)
	}
	return c.appendCarriedMsg(lines, msg)
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
	lines := []string{fmt.Sprintf("Review round %d: score %d/%d", body.Round, body.Score, body.Pass)}
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

func formatSystemMsg(msg proto.MsgRow) (string, error) {
	var sys proto.SystemCode
	if proto.Decode(msg.Body, &sys) != nil {
		return "", &Error{"bad_request", "invalid system body"}
	}
	return "System: " + sys.Code, nil
}

func (c *turnCtx) replacedContext() (string, error) {
	lines, err := c.replacedSessionLines()
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
		return "== context · instruction", nil
	}
	return "== context · instruction\n" + strings.Join(lines, "\n"), nil
}

func (c *turnCtx) replacedSessionLines() ([]string, error) {
	switch c.s.Kind {
	case TaskSession:
		task, ok := c.lookupTask(c.s.Task)
		if !ok {
			return nil, &Error{"bad_request", "replaced task session requires task row"}
		}
		return c.taskStartLines(task)
	case ReviewSession:
		task, ok := c.lookupTask(c.s.Task)
		if !ok {
			return nil, &Error{"bad_request", "replaced review session requires task row"}
		}
		report, err := fetchMsg[proto.EmptyBody](c, c.s.Report, "report")
		if err != nil {
			return nil, err
		}
		return c.reviewLines(task, report.By)
	default:
		return nil, nil
	}
}

func (c *turnCtx) formatHistory(msgs []proto.MsgRow) ([]string, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	lines := []string{"Recent messages:"}
	c.confirmations = append(c.confirmations, msgs...)
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
	content, err := historyContent(m)
	if err != nil {
		return "", err
	}
	if resolvedOutcome(m) {
		return "-- " + m.Kind + " · " + strings.TrimPrefix(c.formatCarriedDecision(m, content), "-- "), nil
	}
	instruct, err := c.msgInstruct(m)
	return fmt.Sprintf("-- %s · %s · %s\n%s", m.Kind, c.fromIn(m.By, strVal(m.Thread)), authorityLabel(instruct), content), err
}

func historyContent(m proto.MsgRow) (string, error) {
	if m.Kind == "brief" && resolvedOutcome(m) {
		return formatConfirmedBrief(m)
	}
	return formatMsgContent(m)
}

func resolvedOutcome(m proto.MsgRow) bool {
	return slices.Contains([]string{"approved", "denied", "decided", "confirmed", "revise"}, cardStatus(m.State))
}

func (c *turnCtx) fetchPage(id, before int64, limit int) proto.FetchPage {
	return proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.s.Group, Before: before, Limit: limit}
}

func fetchFrom(ctx context.Context, h *Host, device string, build func(id int64) any) (proto.Fetched, error) {
	home, ok := h.Home(device)
	if !ok {
		return proto.Fetched{}, errHomeUnreachable
	}
	raw, err := home.Request(ctx, build)
	if err != nil {
		return proto.Fetched{}, errHomeUnreachable
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

// fetchMsg fetches one message of the group by its id; what names its kind
// in an error.
func fetchMsg[B any](c *turnCtx, id, what string) (msg proto.MessageRow[B, *proto.CardState], err error) {
	build := func(n int64) any {
		return proto.FetchRow{FetchPage: c.fetchPage(n, 0, 1), Row: "msg", RowID: id}
	}
	fetched, err := fetchFrom(context.Background(), c.h, c.s.Home, build)
	if err != nil {
		return msg, err
	}
	if len(fetched.Rows) == 0 {
		return msg, &Error{"not_found", what + " not found"}
	}
	if proto.Decode(fetched.Rows[0], &msg) != nil {
		return msg, &Error{"bad_request", "invalid " + what + " row"}
	}
	return msg, nil
}

func (c *turnCtx) fetchBrief(briefID string) (proto.BriefBody, error) {
	msg, err := fetchMsg[proto.BriefBody](c, briefID, "brief")
	return msg.Body, err
}

func (c *turnCtx) fetchNotes() ([]string, error) {
	build := func(id int64) any {
		return proto.FetchKind{FetchPage: c.fetchPage(id, 0, notePageLimit), Kind: "note"}
	}
	fetched, err := fetchFrom(context.Background(), c.h, c.s.Home, build)
	if err != nil {
		return nil, err
	}
	return decodeNotes(fetched.Rows)
}

func decodeNotes(rawRows []json.RawMessage) ([]string, error) {
	var notes []string
	used := 0
	for _, raw := range rawRows {
		var n proto.NoteRow
		if proto.Decode(raw, &n) != nil {
			return nil, &Error{"bad_request", "invalid note row"}
		}
		if used+len(n.Text) > maxNoteBytes {
			break
		}
		used += len(n.Text)
		notes = append(notes, n.Text)
	}
	return notes, nil
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
	fetched, err := fetchFrom(context.Background(), c.h, c.s.Home, build)
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
