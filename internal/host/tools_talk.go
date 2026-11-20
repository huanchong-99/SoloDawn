package host

import (
	"cmp"
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Schemas registered on Tools match internal/proto/testdata/tools.json.
// For post and read_thread, where tools.json narrows the schema by session,
// Tools holds the superset schema (secretary for post, group for read_thread)
// while the MCP server projects caller-specific schemas on tools/list and Call
// enforces the caller rules below.
var (
	postSchema = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","enum":["text","progress","question","handback","summary"],"default":"text"},"mentions":{"type":"array","items":{"type":"string"}},"task":{"type":"string"},"text":{"type":"string"},"where":{"type":"string","enum":["thread","main"]}},"required":["text"],"additionalProperties":false}`)

	readThreadSchema = json.RawMessage(`{"type":"object","properties":{"before":{"type":"integer","minimum":1},"limit":{"type":"integer","default":30,"minimum":1,"maximum":100},"task":{"type":"string"}},"required":["task"],"additionalProperties":false}`)

	listMembersSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

	listTasksSchema = json.RawMessage(`{"type":"object","properties":{"state":{"type":"string","enum":["queued","active","paused","failed","merged","cancelled"]}},"additionalProperties":false}`)

	notePutSchema = json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"},"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`)

	noteListSchema = json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"}},"additionalProperties":false}`)
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
	Text     string    `json:"text"`
	Kind     *string   `json:"kind,omitempty"`
	Mentions *[]string `json:"mentions,omitempty"`
	Where    *string   `json:"where,omitempty"`
	Task     *string   `json:"task,omitempty"`
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

func errHomeUnreachable() error {
	return &Error{Code: "home_unreachable", Message: "the home of that group is not connected"}
}

func decodeTalkArgs(raw json.RawMessage, dst any, allowed ...string) error {
	if proto.Decode(raw, dst) != nil {
		return badTalkRequest("invalid tool arguments")
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	for name := range fields {
		if !slices.Contains(allowed, name) {
			return badTalkRequest("unknown argument " + name)
		}
	}
	return nil
}

func checkTalkSecret(text string) error {
	if err := CheckOutboundSecret(text); err != nil {
		return &Error{Code: "secret", Message: "this looks like a secret; remove it and send again"}
	}
	return nil
}

func newTalkID() proto.ID {
	var b [16]byte
	crand.Read(b[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(b[:]))
}

func talkCommandHeader(h *Host, c Caller, cmdType string) proto.HomeCommand {
	return proto.HomeCommand{
		Type:      cmdType,
		Nonce:     newTalkID(),
		Timestamp: h.Now().UnixMilli(),
		Group:     proto.ID(c.Group),
		As:        proto.ID(c.Member),
	}
}

func encodeTalkJSON(v any) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

func encodePostPayload[B any](hdr proto.HomeCommand, p preparedPost, body B) string {
	return encodeTalkJSON(proto.Post[B]{
		HomeCommand: hdr,
		Kind:        p.kind,
		Thread:      p.thread,
		Main:        p.main,
		Text:        p.text,
		Mentions:    p.mentions,
		Re:          nil,
		Body:        body,
	})
}

func callPost(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a postArgs
	if err := decodeTalkArgs(raw, &a, postAllowedFields(c.Secretary)...); err != nil {
		return nil, err
	}
	if err := checkTalkSecret(a.Text); err != nil {
		return nil, err
	}
	p, err := preparePost(h, c, a)
	if err != nil {
		return nil, err
	}
	return sendPreparedPost(ctx, h, c, p)
}

func postAllowedFields(secretary bool) []string {
	if secretary {
		return []string{"text", "kind", "mentions", "where", "task"}
	}
	return []string{"text", "kind", "mentions", "where"}
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
	mentions, err := resolveMentions(h, c.Group, a.Mentions)
	if err != nil {
		return preparedPost{}, err
	}
	return preparedPost{kind: kind, text: a.Text, thread: thread, main: mainLine, mentions: mentions}, nil
}

func checkPostKind(c Caller, kind string) error {
	switch kind {
	case "text", "progress", "question":
		return nil
	case "summary":
		if !c.Secretary {
			return badTalkRequest("only the secretary may post a summary")
		}
		return nil
	case "handback":
		return checkHandbackCaller(c)
	default:
		return badTalkRequest("unsupported post kind " + kind)
	}
}

func checkHandbackCaller(c Caller) error {
	if c.Interactive || c.Turn.Kind != HandbackTurn {
		return badTalkRequest("handback is allowed only in a hand-back turn")
	}
	if c.Kind == GroupSession && !c.Secretary {
		return badTalkRequest("group sessions of working AIs have no takeover")
	}
	return nil
}

func resolvePostPlace(h *Host, c Caller, kind string, whereArg, taskArg *string) (*proto.ID, bool, error) {
	where, err := postWhere(whereArg)
	if err != nil {
		return nil, false, err
	}
	if kind == "summary" {
		return resolveSummaryPlace(h, c.Group, taskArg)
	}
	thread, err := resolveOrdinaryThread(h, c, where, taskArg)
	if err != nil {
		return nil, false, err
	}
	return thread, false, checkKindPlace(c, kind, thread)
}

func postWhere(whereArg *string) (string, error) {
	if whereArg == nil {
		return "", nil
	}
	if *whereArg != "thread" && *whereArg != "main" {
		return "", badTalkRequest("where must be thread or main")
	}
	return *whereArg, nil
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

func resolveOrdinaryThread(h *Host, c Caller, where string, taskArg *string) (*proto.ID, error) {
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
	tid := cmp.Or(c.Turn.Thread, c.Task)
	if tid == "" {
		if where == "thread" {
			return nil, badTalkRequest("no task thread for where=thread")
		}
		return nil, nil
	}
	id := proto.ID(tid)
	return &id, nil
}

func checkKindPlace(c Caller, kind string, thread *proto.ID) error {
	if kind == "progress" && thread == nil {
		return badTalkRequest("progress post requires a task thread")
	}
	if kind == "handback" && (thread == nil) != c.Secretary {
		return badTalkRequest("handback post is not in the session's place")
	}
	return nil
}

func sendPreparedPost(ctx context.Context, h *Host, c Caller, p preparedPost) (any, error) {
	hdr := talkCommandHeader(h, c, "post")
	payload := encodePostPayload(hdr, p, proto.EmptyBody{})
	if p.kind == "handback" {
		body, err := handbackFacts(ctx, h, c)
		if err != nil {
			return nil, err
		}
		payload = encodePostPayload(hdr, p, body)
	}
	if _, err := h.Command(ctx, c.Home, payload); err != nil {
		return nil, err
	}
	return idResult{ID: string(hdr.Nonce)}, nil
}

func handbackFacts(ctx context.Context, h *Host, c Caller) (proto.HandbackBody, error) {
	empty := proto.HandbackBody{Files: []string{}, Commits: []proto.CommitSubject{}}
	if c.Kind != TaskSession || c.TakeoverAt == "" {
		return empty, nil
	}
	return readGitHandback(ctx, sessionWorkDir(h, c.Group, c.Folder), c.TakeoverAt)
}

const mainDBFileSQL = `SELECT coalesce((SELECT file FROM pragma_database_list WHERE name = 'main'), '')`

func sessionWorkDir(h *Host, group, folder string) string {
	if filepath.IsAbs(folder) {
		return folder
	}
	var dbFile string
	scan := func(tx *store.Tx) error {
		return tx.QueryRow(mainDBFileSQL).Scan(&dbFile)
	}
	h.Store.Write(scan)
	if dbFile == "" {
		return folder
	}
	return filepath.Join(filepath.Dir(dbFile), "projects", group, folder)
}

func readGitHandback(ctx context.Context, dir, start string) (proto.HandbackBody, error) {
	outs, err := runGitLines(ctx, dir,
		[]string{"rev-parse", "HEAD"},
		[]string{"status", "--porcelain", "--untracked-files=all"},
		[]string{"log", start + "..HEAD", "--format=%h %s"},
		[]string{"diff", "--shortstat", start},
		[]string{"diff", "--name-only", start},
	)
	if err != nil {
		return proto.HandbackBody{}, err
	}
	added, removed := parseShortstat(outs[3])
	return proto.HandbackBody{
		Files:   collectHandbackFiles(outs[4], outs[1]),
		Commits: parseHandbackCommits(outs[2]),
		Added:   added,
		Removed: removed,
	}, nil
}

func runGitLines(ctx context.Context, dir string, commands ...[]string) ([]string, error) {
	outs := make([]string, len(commands))
	for i, args := range commands {
		out, err := workspaceGit(ctx, dir, args...)
		if err != nil {
			return nil, err
		}
		outs[i] = out
	}
	return outs, nil
}

func parseHandbackCommits(logOut string) []proto.CommitSubject {
	trimmed := strings.TrimRight(logOut, "\r\n")
	if trimmed == "" {
		return []proto.CommitSubject{}
	}
	lines := strings.Split(trimmed, "\n")
	commits := make([]proto.CommitSubject, len(lines))
	for i, line := range lines {
		sha, subject, _ := strings.Cut(strings.TrimRight(line, "\r"), " ")
		commits[i] = proto.CommitSubject{SHA: sha, Subject: subject}
	}
	return commits
}

func parseShortstat(statOut string) (added, removed int) {
	for part := range strings.SplitSeq(statOut, ",") {
		fields := strings.Fields(part)
		if len(fields) >= 2 {
			n, _ := strconv.Atoi(fields[0])
			applyShortstatField(fields[1], n, &added, &removed)
		}
	}
	return added, removed
}

func applyShortstatField(label string, n int, added, removed *int) {
	if strings.HasPrefix(label, "insertion") {
		*added = n
	} else if strings.HasPrefix(label, "deletion") {
		*removed = n
	}
}

func collectHandbackFiles(namesOut, statusOut string) []string {
	files := []string{}
	for line := range strings.SplitSeq(namesOut, "\n") {
		files = appendUniqueFile(files, strings.TrimRight(line, "\r"))
	}
	for line := range strings.SplitSeq(statusOut, "\n") {
		if untracked, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "?? "); ok {
			files = appendUniqueFile(files, untracked)
		}
	}
	return files
}

func appendUniqueFile(files []string, path string) []string {
	if path != "" && !slices.Contains(files, path) {
		return append(files, path)
	}
	return files
}

func callReadThread(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a readThreadArgs
	if err := decodeTalkArgs(raw, &a, "task", "limit", "before"); err != nil {
		return nil, err
	}
	limit, before, err := checkThreadPaging(a.Limit, a.Before)
	if err != nil {
		return nil, err
	}
	thread, err := resolveThreadTarget(h, c, a.Task)
	if err != nil {
		return nil, err
	}
	return fetchThreadMessages(ctx, h, c, thread, before, limit)
}

func checkThreadPaging(limitArg *int, beforeArg *int64) (int, int64, error) {
	limit := 30
	if limitArg != nil {
		if *limitArg < 1 || *limitArg > 100 {
			return 0, 0, badTalkRequest("limit must be between 1 and 100")
		}
		limit = *limitArg
	}
	var before int64
	if beforeArg != nil {
		if *beforeArg < 1 {
			return 0, 0, badTalkRequest("before must be at least 1")
		}
		before = *beforeArg
	}
	return limit, before, nil
}

func resolveThreadTarget(h *Host, c Caller, taskArg *string) (*string, error) {
	if taskArg == nil {
		if c.Kind == GroupSession {
			return nil, badTalkRequest("read_thread requires task in a group session")
		}
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
	page, err := requestFetched(ctx, h, c.Home, req)
	if err != nil {
		return nil, err
	}
	names := memberNamesByID(h, c.Group)
	msgs := make([]threadMsgItem, len(page.Rows))
	for i, rawRow := range page.Rows {
		var row proto.MsgRow
		if proto.Decode(rawRow, &row) != nil {
			return nil, badTalkRequest("invalid message row")
		}
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

func requestFetched(ctx context.Context, h *Host, device string, build func(int64) any) (proto.Fetched, error) {
	conn, ok := h.Home(device)
	if !ok {
		return proto.Fetched{}, errHomeUnreachable()
	}
	reply, err := conn.Request(ctx, build)
	if err != nil {
		return proto.Fetched{}, errHomeUnreachable()
	}
	return decodeFetchedReply(reply)
}

func decodeFetchedReply(reply json.RawMessage) (proto.Fetched, error) {
	var wireErr proto.ErrorResponse
	if proto.Decode(reply, &wireErr) == nil {
		return proto.Fetched{}, &Error{Code: wireErr.Err.Code, Message: wireErr.Err.Message}
	}
	var page proto.Fetched
	if proto.Decode(reply, &page) != nil {
		return proto.Fetched{}, badTalkRequest("invalid fetched frame")
	}
	return page, nil
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

var validTaskStates = []string{"queued", "active", "paused", "failed", "merged", "cancelled"}

func callListTasks(_ context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a listTasksArgs
	if err := decodeTalkArgs(raw, &a, "state"); err != nil {
		return nil, err
	}
	if err := checkTaskStateFilter(a.State); err != nil {
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

func checkTaskStateFilter(state *string) error {
	if state != nil && !slices.Contains(validTaskStates, *state) {
		return badTalkRequest("invalid task state " + *state)
	}
	return nil
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
	if err := decodeTalkArgs(raw, &a, "text", "task"); err != nil {
		return nil, err
	}
	if err := checkTalkSecret(a.Text); err != nil {
		return nil, err
	}
	task, err := resolveNotePutTask(h, c, a.Task)
	if err != nil {
		return nil, err
	}
	hdr := talkCommandHeader(h, c, "note")
	noteID := newTalkID()
	cmd := proto.Note{HomeCommand: hdr, ID: noteID, Task: task, Text: a.Text}
	if _, err := h.Command(ctx, c.Home, encodeTalkJSON(cmd)); err != nil {
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
	if err := decodeTalkArgs(raw, &a, "task"); err != nil {
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
	return fetchAllNotes(ctx, h, c, filter)
}

func fetchAllNotes(ctx context.Context, h *Host, c Caller, filter string) (any, error) {
	names := memberNamesByID(h, c.Group)
	tasks := taskLabelsByID(h, c.Group)
	notes := []noteListItem{}
	var before int64
	for {
		page, err := fetchNotesPage(ctx, h, c, before)
		if err != nil {
			return nil, err
		}
		notes, before, err = appendNoteRows(notes, page.Rows, filter, names, tasks)
		if err != nil {
			return nil, err
		}
		if !page.More || len(page.Rows) == 0 {
			return noteListResult{Notes: notes}, nil
		}
	}
}

func fetchNotesPage(ctx context.Context, h *Host, c Caller, before int64) (proto.Fetched, error) {
	req := func(id int64) any {
		return proto.FetchKind{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: before, Limit: noteFetchLimit},
			Kind:      "note",
		}
	}
	return requestFetched(ctx, h, c.Home, req)
}

func appendNoteRows(dst []noteListItem, rows []json.RawMessage, filter string, names, tasks map[string]string) ([]noteListItem, int64, error) {
	var before int64
	for _, rawRow := range rows {
		var row proto.NoteRow
		if proto.Decode(rawRow, &row) != nil {
			return nil, 0, badTalkRequest("invalid note row")
		}
		before = row.Pos
		if noteMatchesTask(row.Task, filter) {
			dst = append(dst, noteListItem{
				ID:   row.ID,
				Task: formatNoteTask(tasks, row.Task),
				Text: row.Text,
				By:   cmp.Or(names[row.By], row.By),
				At:   row.Timestamp,
			})
		}
	}
	return dst, before, nil
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

func liveMembers(h *Host, group string) []proto.MemberRow {
	var list []proto.MemberRow
	for _, raw := range h.Rows.Of(group, "member") {
		var m proto.MemberRow
		if proto.Decode(raw, &m) == nil {
			list = append(list, m)
		}
	}
	return list
}

func memberNamesByID(h *Host, group string) map[string]string {
	names := map[string]string{}
	for _, m := range liveMembers(h, group) {
		names[m.ID] = m.Name
	}
	return names
}

func memberIDsByName(h *Host, group string) (map[string]proto.ID, []string) {
	byName := map[string]proto.ID{}
	for _, m := range liveMembers(h, group) {
		byName[m.Name] = proto.ID(m.ID)
	}
	return byName, slices.Sorted(maps.Keys(byName))
}

func resolveMentions(h *Host, group string, raw *[]string) ([]proto.ID, error) {
	if raw == nil {
		return []proto.ID{}, nil
	}
	byName, valid := memberIDsByName(h, group)
	ids := make([]proto.ID, len(*raw))
	for i, name := range *raw {
		id, ok := byName[name]
		if !ok {
			return nil, unknownNameError("member", name, valid)
		}
		ids[i] = id
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
	return "", unknownNameError("task", ref, valid)
}

func unknownNameError(kind, name string, valid []string) error {
	listed := "none"
	if len(valid) > 0 {
		listed = strings.Join(valid, ", ")
	}
	return &Error{Code: "not_found", Message: fmt.Sprintf("unknown %s %q; valid %ss: %s", kind, name, kind, listed)}
}
