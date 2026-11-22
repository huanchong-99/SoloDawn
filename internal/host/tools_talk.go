package host

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Schemas registered on Tools match internal/proto/testdata/tools.json.
// For post and read_thread, where tools.json narrows the schema by session,
// Tools holds the superset schema (secretary for post, group for read_thread)
// while the MCP server projects caller-specific schemas.
var (
	postSchema = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","enum":["text","progress","question","handback","summary"],"default":"text"},"mentions":{"type":"array","items":{"type":"string"}},"task":{"type":"string"},"text":{"type":"string"},"where":{"type":"string","enum":["thread","main"]}},"required":["text"],"additionalProperties":false}`)

	readThreadSchema = json.RawMessage(`{"type":"object","properties":{"before":{"type":"integer","minimum":1},"limit":{"type":"integer","default":30,"minimum":1,"maximum":100},"task":{"type":"string"}},"required":["task"],"additionalProperties":false}`)

	listMembersSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

	listTasksSchema = json.RawMessage(`{"type":"object","properties":{"state":{"type":"string","enum":["queued","active","paused","failed","merged","cancelled"]}},"additionalProperties":false}`)

	notePutSchema = json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"},"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`)

	noteListSchema = json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"},"type":{"type":"string","enum":["preset"]}},"additionalProperties":false}`)
)

func init() {
	Register(Tools, "post", Tool{
		Description: "Post a message to the group main line or a task thread.",
		Schema:      postSchema,
		Call:        callPost,
	})
	Register(Tools, "read_thread", Tool{
		Description: "Read messages from a task thread or the group main line.",
		Schema:      readThreadSchema,
		Call:        callReadThread,
	})
	Register(Tools, "list_members", Tool{
		Description: "List the current members of the group.",
		Schema:      listMembersSchema,
		Call:        callListMembers,
	})
	Register(Tools, "list_tasks", Tool{
		Description: "List tasks of the group, optionally filtered by state.",
		Schema:      listTasksSchema,
		Call:        callListTasks,
	})
	Register(Tools, "note_put", Tool{
		Description: "Write a durable project note for future tasks.",
		Schema:      notePutSchema,
		Call:        callNotePut,
	})
	Register(Tools, "note_list", Tool{
		Description: "List the durable notes of the group.",
		Schema:      noteListSchema,
		Call:        callNoteList,
	})
}

type postArgs struct {
	Text     string   `json:"text"`
	Kind     *string  `json:"kind,omitempty"`
	Mentions []string `json:"mentions,omitempty"`
	Where    *string  `json:"where,omitempty"`
	Task     *string  `json:"task,omitempty"`
}

type readThreadArgs struct {
	Task   *string `json:"task,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
	Before *int64  `json:"before,omitempty"`
}

type listTasksArgs struct {
	State *string `json:"state,omitempty"`
}

type notePutArgs struct {
	Text string  `json:"text"`
	Task *string `json:"task,omitempty"`
}

type noteListArgs struct {
	Task *string `json:"task,omitempty"`
	Type *string `json:"type,omitempty"`
}

type idResult struct {
	ID string `json:"id"`
}

type threadMsgItem struct {
	ID   string `json:"id"`
	Pos  int64  `json:"pos"`
	By   string `json:"by"`
	Kind string `json:"kind"`
	At   int64  `json:"at"`
	Text string `json:"text"`
}

type readThreadResult struct {
	Messages []threadMsgItem `json:"messages"`
	More     bool            `json:"more"`
}

type memberListItem struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"`
	State   string       `json:"state"`
	Host    string       `json:"host,omitempty"`
	Account string       `json:"account,omitempty"`
	Task    string       `json:"task,omitempty"`
	Usage   *proto.Usage `json:"usage,omitempty"`
}

type listMembersResult struct {
	Members []memberListItem `json:"members"`
}

type taskListItem struct {
	Task        string  `json:"task"`
	Title       string  `json:"title"`
	State       string  `json:"state"`
	Step        string  `json:"step"`
	Assignee    string  `json:"assignee"`
	Reviewer    string  `json:"reviewer,omitempty"`
	Wait        *string `json:"wait"`
	IdleMinutes int     `json:"idle_minutes"`
}

type listTasksResult struct {
	Tasks []taskListItem `json:"tasks"`
}

type noteListItem struct {
	ID   string  `json:"id"`
	Task *string `json:"task"`
	Name string  `json:"name,omitempty"`
	Text string  `json:"text"`
	By   string  `json:"by"`
	At   int64   `json:"at"`
}

type noteListResult struct {
	Notes []noteListItem `json:"notes"`
}

type preparedPost struct {
	kind     string
	text     string
	thread   *proto.ID
	main     bool
	mentions []proto.ID
}

func badTalkRequest(msg string) error {
	return &Error{Code: "bad_request", Message: msg}
}

func decodeTalkArgs(raw json.RawMessage, dst any) error {
	if proto.Decode(raw, dst) != nil {
		return badTalkRequest("invalid tool arguments")
	}
	return nil
}

func callPost(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a postArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(a.Text); err != nil {
		return nil, err
	}
	p, err := preparePost(h, c, a)
	if err != nil {
		return nil, err
	}
	return sendPreparedPost(ctx, h, c, p)
}

func preparePost(h *Host, c Caller, a postArgs) (preparedPost, error) {
	kind := "text"
	if a.Kind != nil {
		kind = *a.Kind
	}
	if err := checkPostKind(c, kind); err != nil {
		return preparedPost{}, err
	}
	thread, mainLine, err := resolvePostPlace(h, c, kind, a.Where, a.Task)
	if err != nil {
		return preparedPost{}, err
	}
	mentions, err := resolveMentions(h.Rows, c.Group, a.Mentions)
	if err != nil {
		return preparedPost{}, err
	}
	return preparedPost{kind: kind, text: a.Text, thread: thread, main: mainLine, mentions: mentions}, nil
}

// checkPostKind allows handback only in a hand-back turn: a tool list is
// fixed per session (contracts/mcp-tools.md section 5).
func checkPostKind(c Caller, kind string) error {
	switch kind {
	case "text", "progress", "question", "summary":
		return nil
	case "handback":
		if c.Turn.Kind == HandbackTurn {
			return nil
		}
		return badTalkRequest("handback is posted only in a hand-back turn")
	default:
		return badTalkRequest("unsupported post kind " + kind)
	}
}

func resolvePostPlace(h *Host, c Caller, kind string, whereArg, taskArg *string) (*proto.ID, bool, error) {
	if kind == "summary" {
		return resolveSummaryPlace(h, c.Group, taskArg)
	}
	thread, err := resolveOrdinaryThread(h, c, whereArg, taskArg)
	return thread, false, err
}

func resolveSummaryPlace(h *Host, group string, taskArg *string) (*proto.ID, bool, error) {
	if taskArg == nil {
		return nil, false, badTalkRequest("summary post requires task")
	}
	tid, err := resolveTaskID(h, group, *taskArg)
	if err != nil {
		return nil, false, err
	}
	return &tid, true, nil
}

func resolveOrdinaryThread(h *Host, c Caller, whereArg, taskArg *string) (*proto.ID, error) {
	var where string
	if whereArg != nil {
		where = *whereArg
	}
	if taskArg != nil {
		if where == "main" {
			return nil, badTalkRequest("task cannot be combined with where=main")
		}
		tid, err := resolveTaskID(h, c.Group, *taskArg)
		if err != nil {
			return nil, err
		}
		return &tid, nil
	}
	return turnThreadPlace(c, where)
}

func turnThreadPlace(c Caller, where string) (*proto.ID, error) {
	if where == "main" {
		return nil, nil
	}
	if c.Turn.Thread == "" {
		if where == "thread" {
			return nil, badTalkRequest("no task thread for where=thread")
		}
		return nil, nil
	}
	id := proto.ID(c.Turn.Thread)
	return &id, nil
}

func sendPreparedPost(ctx context.Context, h *Host, c Caller, p preparedPost) (any, error) {
	hdr := secHomeHeader(h, c, "post")
	var body any = proto.EmptyBody{}
	if p.kind == "handback" {
		body = takeoverFacts(h, c.SessionKey)
	}
	payload := marshalWire(proto.Post[any]{
		HomeCommand: hdr,
		Kind:        p.kind,
		Thread:      p.thread,
		Main:        p.main,
		Text:        p.text,
		Mentions:    p.mentions,
		Re:          nil,
		Body:        body,
	})
	if _, err := h.Command(ctx, c.Home, payload); err != nil {
		return nil, err
	}
	return idResult{ID: string(hdr.Nonce)}, nil
}

func callReadThread(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a readThreadArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	limit, before := threadPaging(a.Limit, a.Before)
	thread, err := resolveThreadTarget(h, c, a.Task)
	if err != nil {
		return nil, err
	}
	return fetchThreadMessages(ctx, h, c, thread, before, limit)
}

func threadPaging(limitArg *int, beforeArg *int64) (int, int64) {
	limit := 30
	if limitArg != nil {
		limit = *limitArg
	}
	var before int64
	if beforeArg != nil {
		before = *beforeArg
	}
	return limit, before
}

func resolveThreadTarget(h *Host, c Caller, taskArg *string) (*string, error) {
	if taskArg == nil {
		own := c.Task
		return &own, nil
	}
	if *taskArg == "main" {
		return nil, nil
	}
	tid, err := resolveTaskID(h, c.Group, *taskArg)
	if err != nil {
		return nil, err
	}
	s := string(tid)
	return &s, nil
}

func fetchThreadMessages(ctx context.Context, h *Host, c Caller, thread *string, before int64, limit int) (any, error) {
	req := func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: before, Limit: limit},
			Thread:    thread,
		}
	}
	page, err := fetchFrom(ctx, h, c.Home, req)
	if err != nil {
		return nil, err
	}
	names := memberNamesByID(h, c.Group)
	msgs := make([]threadMsgItem, len(page.Rows))
	for i, rawRow := range page.Rows {
		var row proto.MsgRow
		proto.Decode(rawRow, &row)
		msgs[i] = threadMsgItem{
			ID:   row.ID,
			Pos:  row.Pos,
			By:   cmp.Or(names[row.By], row.By),
			Kind: row.Kind,
			At:   row.Timestamp,
			Text: row.Text,
		}
	}
	return readThreadResult{Messages: msgs, More: page.More}, nil
}

func callListMembers(_ context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	if err := decodeTalkArgs(raw, &struct{}{}); err != nil {
		return nil, err
	}
	tasks := taskLabelsByID(h, c.Group)
	items := []memberListItem{}
	for _, rawRow := range h.Rows.Of(c.Group, "member") {
		if item, ok := decodeMemberItem(rawRow, tasks); ok {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, compareMemberName)
	return listMembersResult{Members: items}, nil
}

func compareMemberName(a, b memberListItem) int {
	return cmp.Compare(a.Name, b.Name)
}

func decodeMemberItem(raw json.RawMessage, tasks map[string]string) (memberListItem, bool) {
	var base proto.MemberRow
	if proto.Decode(raw, &base) != nil {
		return memberListItem{}, false
	}
	if base.Kind == "ai" {
		return decodeAIMemberItem(raw, tasks)
	}
	return decodeHumanMemberItem(raw)
}

func decodeAIMemberItem(raw json.RawMessage, tasks map[string]string) (memberListItem, bool) {
	var ai proto.AIMemberRow
	if proto.Decode(raw, &ai) != nil {
		return memberListItem{}, false
	}
	usage := ai.Usage
	return memberListItem{
		Name:    ai.Name,
		Kind:    ai.Kind,
		State:   ai.State,
		Host:    cmp.Or(ai.HostName, ai.Host),
		Account: ai.Account,
		Task:    cmp.Or(tasks[ai.Task], ai.Task),
		Usage:   &usage,
	}, true
}

func decodeHumanMemberItem(raw json.RawMessage) (memberListItem, bool) {
	var hum proto.HumanMemberRow
	if proto.Decode(raw, &hum) != nil {
		return memberListItem{}, false
	}
	state := "offline"
	if hum.Online {
		state = "online"
	}
	return memberListItem{Name: hum.Name, Kind: hum.Kind, State: state}, true
}

func callListTasks(_ context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a listTasksArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	names := memberNamesByID(h, c.Group)
	nowMS := h.Now().UnixMilli()
	items := []taskListItem{}
	for _, row := range liveTasks(h, c.Group) {
		if taskMatchesState(row.State, a.State) {
			items = append(items, buildTaskListItem(row, names, nowMS))
		}
	}
	return listTasksResult{Tasks: items}, nil
}

func taskMatchesState(rowState string, filter *string) bool {
	return filter == nil || rowState == *filter
}

func buildTaskListItem(row proto.TaskRow, names map[string]string, nowMS int64) taskListItem {
	var wait *string
	if row.Wait != nil {
		kind := row.Wait.Kind
		wait = &kind
	}
	return taskListItem{
		Task:        taskLabel(row.Num),
		Title:       row.Title,
		State:       row.State,
		Step:        row.Step,
		Assignee:    cmp.Or(names[row.Assignee], row.Assignee),
		Reviewer:    cmp.Or(names[row.Reviewer], row.Reviewer),
		Wait:        wait,
		IdleMinutes: taskIdleMinutes(row, nowMS),
	}
}

func taskIdleMinutes(row proto.TaskRow, nowMS int64) int {
	if row.Stats.Ended != nil || row.State == "merged" || row.State == "cancelled" {
		return 0
	}
	last := cmp.Or(row.Stats.Started, row.Created)
	if row.Thread.Last != nil {
		last = row.Thread.Last.Timestamp
	}
	return max(0, int((nowMS-last)/60_000))
}

func callNotePut(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a notePutArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(a.Text); err != nil {
		return nil, err
	}
	task, err := resolveNotePutTask(h, c, a.Task)
	if err != nil {
		return nil, err
	}
	hdr := secHomeHeader(h, c, "note")
	noteID := proto.NewNonce()
	cmd := proto.Note{HomeCommand: hdr, ID: noteID, Task: task, Text: a.Text}
	if _, err := h.Command(ctx, c.Home, marshalWire(cmd)); err != nil {
		return nil, err
	}
	return idResult{ID: string(noteID)}, nil
}

func resolveNotePutTask(h *Host, c Caller, taskArg *string) (*proto.ID, error) {
	if taskArg != nil {
		tid, err := resolveTaskID(h, c.Group, *taskArg)
		if err != nil {
			return nil, err
		}
		return &tid, nil
	}
	if c.Kind == GroupSession {
		return nil, nil
	}
	own := proto.ID(c.Task)
	return &own, nil
}

const noteFetchLimit = 100

func callNoteList(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a noteListArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	kind, err := resolveNoteListKind(a.Type)
	if err != nil {
		return nil, err
	}
	var filter string
	if a.Task != nil {
		tid, err := resolveTaskID(h, c.Group, *a.Task)
		if err != nil {
			return nil, err
		}
		filter = string(tid)
	}
	return fetchAllNotes(ctx, h, c, kind, filter)
}

func resolveNoteListKind(typeArg *string) (string, error) {
	if typeArg == nil {
		return "note", nil
	}
	if *typeArg == "preset" {
		return "preset", nil
	}
	return "", badTalkRequest("unsupported note type " + *typeArg)
}

func fetchAllNotes(ctx context.Context, h *Host, c Caller, kind, filter string) (any, error) {
	names := memberNamesByID(h, c.Group)
	tasks := taskLabelsByID(h, c.Group)
	notes := []noteListItem{}
	var before int64
	for {
		page, err := fetchNotesPage(ctx, h, c, kind, before)
		if err != nil {
			return nil, err
		}
		notes, before = appendNoteRows(notes, page.Rows, filter, names, tasks)
		if !page.More || len(page.Rows) == 0 {
			return noteListResult{Notes: notes}, nil
		}
	}
}

func fetchNotesPage(ctx context.Context, h *Host, c Caller, kind string, before int64) (proto.Fetched, error) {
	req := func(id int64) any {
		return proto.FetchKind{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: before, Limit: noteFetchLimit},
			Kind:      kind,
		}
	}
	return fetchFrom(ctx, h, c.Home, req)
}

func appendNoteRows(dst []noteListItem, rows []json.RawMessage, filter string, names, tasks map[string]string) ([]noteListItem, int64) {
	var before int64
	for _, rawRow := range rows {
		var row proto.NoteRow
		proto.Decode(rawRow, &row)
		before = row.Pos
		if noteMatchesTask(row.Task, filter) {
			dst = append(dst, noteListItem{
				ID:   row.ID,
				Task: formatNoteTask(tasks, row.Task),
				Name: row.Name,
				Text: row.Text,
				By:   cmp.Or(names[row.By], row.By),
				At:   row.Timestamp,
			})
		}
	}
	return dst, before
}

func noteMatchesTask(rowTask *string, filter string) bool {
	if filter == "" {
		return true
	}
	return rowTask != nil && *rowTask == filter
}

func formatNoteTask(tasks map[string]string, rowTask *string) *string {
	if rowTask == nil {
		return nil
	}
	label := cmp.Or(tasks[*rowTask], *rowTask)
	return &label
}

func liveMembers(rows *Rows, group string) []proto.MemberRow {
	var list []proto.MemberRow
	for _, raw := range rows.Of(group, "member") {
		var m proto.MemberRow
		if proto.Decode(raw, &m) == nil {
			list = append(list, m)
		}
	}
	return list
}

func memberNamesByID(h *Host, group string) map[string]string {
	names := map[string]string{}
	for _, m := range liveMembers(h.Rows, group) {
		names[m.ID] = m.Name
	}
	return names
}

func resolveMentions(rows *Rows, group string, names []string) ([]proto.ID, error) {
	dir := loadSecDirectory(rows, group)
	ids := []proto.ID{}
	for _, name := range names {
		id, err := dir.memberID(name)
		if err != nil {
			return nil, err
		}
		ids = appendUnique(ids, id)
	}
	return ids, nil
}

func liveTasks(h *Host, group string) []proto.TaskRow {
	var tasks []proto.TaskRow
	for _, raw := range h.Rows.Of(group, "task") {
		var row proto.TaskRow
		if proto.Decode(raw, &row) == nil {
			tasks = append(tasks, row)
		}
	}
	slices.SortFunc(tasks, compareTaskNum)
	return tasks
}

func compareTaskNum(a, b proto.TaskRow) int {
	return cmp.Compare(a.Num, b.Num)
}

func taskLabel(num int) string {
	return "T-" + strconv.Itoa(num)
}

func taskLabelsByID(h *Host, group string) map[string]string {
	labels := map[string]string{}
	for _, t := range liveTasks(h, group) {
		labels[t.ID] = taskLabel(t.Num)
	}
	return labels
}

func resolveTaskID(h *Host, group, ref string) (proto.ID, error) {
	tasks := liveTasks(h, group)
	valid := make([]string, len(tasks))
	for i, t := range tasks {
		valid[i] = taskLabel(t.Num)
		if valid[i] == ref {
			return proto.ID(t.ID), nil
		}
	}
	return "", missingTargetError("task", ref, valid)
}
