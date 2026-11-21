package host

import (
	stdcmp "cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"slices"
	str "strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	proposeBriefSchema = stdjson.RawMessage(`{"type":"object","properties":{"criteria":{"type":"array","items":{"type":"string"}},"goal":{"type":"string"},"replaces":{"type":"string"},"requester":{"type":"string"},"scope_in":{"type":"array","items":{"type":"string"}},"scope_out":{"type":"array","items":{"type":"string"}},"tasks":{"type":"array","items":{"type":"object","properties":{"assignee":{"type":"string"},"depends":{"type":"array","default":[],"items":{"type":"integer","minimum":0}},"foundation":{"type":"boolean","default":false},"reviewer":{"type":"string"},"title":{"type":"string"}},"required":["title","assignee","reviewer"],"additionalProperties":false}},"title":{"type":"string"}},"required":["title","goal","criteria","scope_in","scope_out","tasks"],"additionalProperties":false}`)

	setTaskSchema = stdjson.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["pause","resume","cancel"]},"assignee":{"type":"string"},"reviewer":{"type":"string"},"task":{"type":"string"}},"required":["task"],"additionalProperties":false}`)

	requestDecisionSchema = stdjson.RawMessage(`{"type":"object","properties":{"context":{"type":"string"},"options":{"type":"array","items":{"type":"object","properties":{"effect":{"type":"string"},"label":{"type":"string"}},"required":["label","effect"],"additionalProperties":false}},"question":{"type":"string"},"recommended":{"type":"integer","minimum":0},"to":{"type":"array","items":{"type":"string"}}},"required":["question","options","to"],"additionalProperties":false}`)

	setTaskActions = []string{"pause", "resume", "cancel"}
)

func init() {
	Register(Tools, "propose_brief", Tool{
		Description: "Propose a project brief for confirmation before work starts.",
		Schema:      proposeBriefSchema,
		For:         onlySecretary,
		Call:        callProposeBrief,
	})
	Register(Tools, "set_task", Tool{
		Description: "Assign a task's roles or pause, resume or cancel it.",
		Schema:      setTaskSchema,
		For:         onlySecretary,
		Call:        callSetTask,
	})
	Register(Tools, "request_decision", Tool{
		Description: "Raise a decision card for people in the group to choose.",
		Schema:      requestDecisionSchema,
		Call:        callRequestDecision,
	})
}

func onlySecretary(c Caller) bool { return c.Secretary }

type secIDReply struct {
	ID string `json:"id"`
}

type secOKReply struct {
	OK bool `json:"ok"`
}

type briefTaskInput struct {
	Title      string `json:"title"`
	Assignee   string `json:"assignee"`
	Reviewer   string `json:"reviewer"`
	Foundation *bool  `json:"foundation,omitempty"`
	Depends    *[]int `json:"depends,omitempty"`
}

type proposeBriefInput struct {
	Title     string           `json:"title"`
	Goal      string           `json:"goal"`
	Criteria  []string         `json:"criteria"`
	ScopeIn   []string         `json:"scope_in"`
	ScopeOut  []string         `json:"scope_out"`
	Tasks     []briefTaskInput `json:"tasks"`
	Requester *string          `json:"requester,omitempty"`
	Replaces  *string          `json:"replaces,omitempty"`
}

type setTaskInput struct {
	Task     string  `json:"task"`
	Assignee *string `json:"assignee,omitempty"`
	Reviewer *string `json:"reviewer,omitempty"`
	Action   *string `json:"action,omitempty"`
}

type decisionOptionInput struct {
	Label  string `json:"label"`
	Effect string `json:"effect"`
}

type requestDecisionInput struct {
	Question    string                `json:"question"`
	Options     []decisionOptionInput `json:"options"`
	Recommended *int                  `json:"recommended,omitempty"`
	Context     *string               `json:"context,omitempty"`
	To          *[]string             `json:"to,omitempty"`
}

func decodeSecInput[T any](raw stdjson.RawMessage) (T, error) {
	var dst T
	strict := stdjson.NewDecoder(str.NewReader(string(raw)))
	strict.DisallowUnknownFields()
	if strict.Decode(new(T)) != nil || proto.Decode(raw, &dst) != nil {
		return dst, &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	return dst, nil
}

func guardSecTexts(texts ...string) error {
	for _, s := range texts {
		if CheckOutboundSecret(s) != nil {
			return &Error{Code: "secret", Message: "this looks like a secret; remove it and send again"}
		}
	}
	return nil
}

func freshSecNonce() proto.ID {
	var raw [16]byte
	rand.Read(raw[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

func secHomeHeader(h *Host, c Caller, kind string) proto.HomeCommand {
	return proto.HomeCommand{
		Type:      kind,
		Nonce:     freshSecNonce(),
		Timestamp: h.Now().UnixMilli(),
		Group:     proto.ID(c.Group),
		As:        proto.ID(c.Member),
	}
}

func marshalSecWire(v any) string {
	var out str.Builder
	enc := stdjson.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return str.TrimSuffix(out.String(), "\n")
}

type secDirectory struct {
	byID    map[string]proto.MemberRow
	byName  map[string]proto.MemberRow
	members []string
	people  []string
}

func loadSecDirectory(rows *Rows, group string) secDirectory {
	dir := secDirectory{
		byID:   map[string]proto.MemberRow{},
		byName: map[string]proto.MemberRow{},
	}
	for _, raw := range rows.Of(group, "member") {
		var m proto.MemberRow
		if proto.Decode(raw, &m) == nil {
			dir.byID[m.ID] = m
			dir.byName[m.Name] = m
			dir.members = append(dir.members, m.Name)
			if m.Kind == "human" {
				dir.people = append(dir.people, m.Name)
			}
		}
	}
	slices.Sort(dir.members)
	slices.Sort(dir.people)
	return dir
}

func (d secDirectory) memberID(name string) (proto.ID, error) {
	if m, ok := d.byName[name]; ok {
		return proto.ID(m.ID), nil
	}
	return "", missingTargetError("member", name, d.members)
}

func (d secDirectory) person(name string) (proto.MemberRow, error) {
	if m, ok := d.byName[name]; ok && m.Kind == "human" {
		return m, nil
	}
	return proto.MemberRow{}, missingTargetError("member", name, d.people)
}

func missingTargetError(kind, given string, valid []string) error {
	names := "none"
	if len(valid) > 0 {
		names = str.Join(valid, ", ")
	}
	return &Error{
		Code:    "not_found",
		Message: fmt.Sprintf("unknown %s %q (valid: %s)", kind, given, names),
	}
}

func byTaskNumber(a, b proto.TaskRow) int {
	return stdcmp.Compare(a.Num, b.Num)
}

func loadGroupTasks(rows *Rows, group string) (map[string]proto.ID, []string) {
	var list []proto.TaskRow
	for _, raw := range rows.Of(group, "task") {
		var row proto.TaskRow
		if proto.Decode(raw, &row) == nil {
			list = append(list, row)
		}
	}
	slices.SortFunc(list, byTaskNumber)
	ids := make(map[string]proto.ID, len(list))
	labels := make([]string, len(list))
	for i, row := range list {
		tag := fmt.Sprintf("T-%d", row.Num)
		ids[tag], labels[i] = proto.ID(row.ID), tag
	}
	return ids, labels
}

func resolveSecTask(rows *Rows, group, label string) (proto.ID, error) {
	ids, valid := loadGroupTasks(rows, group)
	if id, ok := ids[label]; ok {
		return id, nil
	}
	return "", missingTargetError("task", label, valid)
}

func secFetchRows(ctx context.Context, h *Host, device string, build func(int64) any) ([]stdjson.RawMessage, error) {
	conn, ok := h.Home(device)
	if !ok {
		return nil, &Error{Code: "home_unreachable", Message: "the home of that group is not connected"}
	}
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return nil, &Error{Code: "home_unreachable", Message: "the home of that group is not connected"}
	}
	return parseSecFetched(raw)
}

func parseSecFetched(raw stdjson.RawMessage) ([]stdjson.RawMessage, error) {
	var failure proto.ErrorResponse
	if proto.Decode(raw, &failure) == nil {
		return nil, &Error{Code: failure.Err.Code, Message: failure.Err.Message}
	}
	var fetched proto.Fetched
	if proto.Decode(raw, &fetched) != nil {
		return nil, &Error{Code: "bad_request", Message: "invalid fetched response"}
	}
	return fetched.Rows, nil
}

func callProposeBrief(ctx context.Context, h *Host, c Caller, raw stdjson.RawMessage) (any, error) {
	in, err := decodeSecInput[proposeBriefInput](raw)
	if err != nil {
		return nil, err
	}
	if err := guardSecTexts(briefTexts(in)...); err != nil {
		return nil, err
	}
	body, requester, err := prepareBriefBody(ctx, h, c, in)
	if err != nil {
		return nil, err
	}
	hdr := secHomeHeader(h, c, "post")
	post := proto.Post[proto.BriefBody]{
		HomeCommand: hdr,
		Kind:        "brief",
		Thread:      nil,
		Main:        false,
		Text:        "",
		Mentions:    []proto.ID{requester},
		Re:          nil,
		Body:        body,
	}
	if _, err := h.Command(ctx, c.Home, marshalSecWire(post)); err != nil {
		return nil, err
	}
	return secIDReply{ID: string(hdr.Nonce)}, nil
}

func briefTexts(in proposeBriefInput) []string {
	texts := make([]string, 0, 3+len(in.Criteria)+len(in.ScopeIn)+len(in.ScopeOut)+len(in.Tasks))
	texts = append(texts, in.Title, in.Goal)
	texts = append(texts, in.Criteria...)
	texts = append(texts, in.ScopeIn...)
	texts = append(texts, in.ScopeOut...)
	for _, t := range in.Tasks {
		texts = append(texts, t.Title)
	}
	if in.Replaces != nil {
		texts = append(texts, *in.Replaces)
	}
	return texts
}

func prepareBriefBody(ctx context.Context, h *Host, c Caller, in proposeBriefInput) (proto.BriefBody, proto.ID, error) {
	pass, err := readGroupPass(h.Rows, c.Group)
	if err != nil {
		return proto.BriefBody{}, "", err
	}
	dir := loadSecDirectory(h.Rows, c.Group)
	tasks, err := buildBriefTasks(dir, in.Tasks)
	if err != nil {
		return proto.BriefBody{}, "", err
	}
	requester, err := resolveBriefRequester(ctx, h, c, dir, in.Requester)
	if err != nil {
		return proto.BriefBody{}, "", err
	}
	return proto.BriefBody{
		Title:     in.Title,
		Goal:      in.Goal,
		Criteria:  in.Criteria,
		Scope:     proto.BriefScope{In: in.ScopeIn, Out: in.ScopeOut},
		Tasks:     tasks,
		Pass:      pass,
		Requester: string(requester),
		Replaces:  in.Replaces,
	}, requester, nil
}

func readGroupPass(rows *Rows, group string) (int, error) {
	raw, ok := rows.Get(group, "group", group)
	if !ok {
		return 0, &Error{Code: "not_found", Message: "group state is not loaded"}
	}
	var grp proto.GroupRow
	if proto.Decode(raw, &grp) != nil {
		return 0, &Error{Code: "bad_request", Message: "invalid group row"}
	}
	return grp.Pass, nil
}

func buildBriefTasks(dir secDirectory, inputs []briefTaskInput) ([]proto.BriefTask, error) {
	out := make([]proto.BriefTask, len(inputs))
	for i, item := range inputs {
		task, err := buildOneBriefTask(dir, item)
		if err != nil {
			return nil, err
		}
		out[i] = task
	}
	return out, nil
}

func buildOneBriefTask(dir secDirectory, item briefTaskInput) (proto.BriefTask, error) {
	assignee, err := dir.memberID(item.Assignee)
	if err != nil {
		return proto.BriefTask{}, err
	}
	reviewer, err := dir.memberID(item.Reviewer)
	if err != nil {
		return proto.BriefTask{}, err
	}
	deps, err := cleanBriefDepends(item.Depends)
	if err != nil {
		return proto.BriefTask{}, err
	}
	var foundation bool
	if item.Foundation != nil {
		foundation = *item.Foundation
	}
	return proto.BriefTask{
		Title:      item.Title,
		Assignee:   string(assignee),
		Reviewer:   string(reviewer),
		Foundation: foundation,
		Depends:    deps,
	}, nil
}

func cleanBriefDepends(raw *[]int) ([]int, error) {
	if raw == nil {
		return []int{}, nil
	}
	for _, idx := range *raw {
		if idx < 0 {
			return nil, &Error{Code: "bad_request", Message: "depends index must be at least 0"}
		}
	}
	return append([]int{}, (*raw)...), nil
}

func resolveBriefRequester(ctx context.Context, h *Host, c Caller, dir secDirectory, explicit *string) (proto.ID, error) {
	if explicit != nil {
		return dir.memberID(*explicit)
	}
	if id, ok := pickRequesterFromMsgs(dir, c.Member, cachedPlaceMessages(h.Rows, c.Group, c.Turn.Thread)); ok {
		return id, nil
	}
	msgs, err := fetchPlaceMessages(ctx, h, c)
	if err != nil {
		return "", err
	}
	if id, ok := pickRequesterFromMsgs(dir, c.Member, msgs); ok {
		return id, nil
	}
	return "", &Error{Code: "bad_request", Message: "propose_brief requires requester when no request message is found"}
}

func cachedPlaceMessages(rows *Rows, group, thread string) []proto.MsgRow {
	var list []proto.MsgRow
	for _, raw := range rows.Of(group, "msg") {
		var m proto.MsgRow
		if proto.Decode(raw, &m) == nil && msgThreadID(m) == thread {
			list = append(list, m)
		}
	}
	slices.SortFunc(list, newestMsgFirst)
	return list
}

func msgThreadID(m proto.MsgRow) string {
	if m.Thread == nil {
		return ""
	}
	return *m.Thread
}

func newestMsgFirst(a, b proto.MsgRow) int {
	return stdcmp.Compare(b.Pos, a.Pos)
}

func fetchPlaceMessages(ctx context.Context, h *Host, c Caller) ([]proto.MsgRow, error) {
	var thread *string
	if c.Turn.Thread != "" {
		t := c.Turn.Thread
		thread = &t
	}
	build := func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: 0, Limit: 50},
			Thread:    thread,
		}
	}
	rawRows, err := secFetchRows(ctx, h, c.Home, build)
	if err != nil {
		return nil, err
	}
	return decodeMsgRows(rawRows)
}

func decodeMsgRows(rawRows []stdjson.RawMessage) ([]proto.MsgRow, error) {
	msgs := make([]proto.MsgRow, len(rawRows))
	for i, raw := range rawRows {
		if proto.Decode(raw, &msgs[i]) != nil {
			return nil, &Error{Code: "bad_request", Message: "invalid message row"}
		}
	}
	slices.SortFunc(msgs, newestMsgFirst)
	return msgs, nil
}

func pickRequesterFromMsgs(dir secDirectory, self string, msgs []proto.MsgRow) (proto.ID, bool) {
	for _, m := range msgs {
		if id, ok := msgHumanOrBriefRequester(dir, m); ok {
			return id, true
		}
	}
	return pickOtherMemberFromMsgs(dir, self, msgs)
}

func msgHumanOrBriefRequester(dir secDirectory, m proto.MsgRow) (proto.ID, bool) {
	if author, ok := dir.byID[m.By]; ok && author.Kind == "human" {
		return proto.ID(author.ID), true
	}
	if m.Kind != "brief" {
		return "", false
	}
	var body proto.BriefBody
	if proto.Decode(m.Body, &body) != nil {
		return "", false
	}
	req, ok := dir.byID[body.Requester]
	return proto.ID(req.ID), ok
}

func pickOtherMemberFromMsgs(dir secDirectory, self string, msgs []proto.MsgRow) (proto.ID, bool) {
	for _, m := range msgs {
		if author, ok := dir.byID[m.By]; ok && m.By != self {
			return proto.ID(author.ID), true
		}
	}
	return "", false
}

func callSetTask(ctx context.Context, h *Host, c Caller, raw stdjson.RawMessage) (any, error) {
	in, err := decodeSecInput[setTaskInput](raw)
	if err != nil {
		return nil, err
	}
	payloads, err := prepareSetTaskPayloads(h, c, in)
	if err != nil {
		return nil, err
	}
	for _, payload := range payloads {
		if _, err := h.Command(ctx, c.Home, payload); err != nil {
			return nil, err
		}
	}
	return secOKReply{OK: true}, nil
}

func prepareSetTaskPayloads(h *Host, c Caller, in setTaskInput) ([]string, error) {
	if err := validateSetTaskInput(in); err != nil {
		return nil, err
	}
	taskID, err := resolveSecTask(h.Rows, c.Group, in.Task)
	if err != nil {
		return nil, err
	}
	dir := loadSecDirectory(h.Rows, c.Group)
	assignee, reviewer, err := resolveSetTaskRoles(dir, in)
	if err != nil {
		return nil, err
	}
	return buildSetTaskPayloads(h, c, taskID, assignee, reviewer, in.Action), nil
}

func hasSetTaskChange(in setTaskInput) bool {
	return in.Assignee != nil || in.Reviewer != nil || in.Action != nil
}

func validateSetTaskInput(in setTaskInput) error {
	if !hasSetTaskChange(in) {
		return &Error{Code: "bad_request", Message: "set_task requires assignee, reviewer or action"}
	}
	if in.Action != nil && !slices.Contains(setTaskActions, *in.Action) {
		return &Error{Code: "bad_request", Message: "action must be pause, resume or cancel"}
	}
	return nil
}

func resolveSetTaskRoles(dir secDirectory, in setTaskInput) (proto.ID, proto.ID, error) {
	assignee, err := resolveOptionalMember(dir, in.Assignee)
	if err != nil {
		return "", "", err
	}
	reviewer, err := resolveOptionalMember(dir, in.Reviewer)
	if err != nil {
		return "", "", err
	}
	return assignee, reviewer, nil
}

func resolveOptionalMember(dir secDirectory, name *string) (proto.ID, error) {
	if name == nil {
		return "", nil
	}
	return dir.memberID(*name)
}

func buildSetTaskPayloads(h *Host, c Caller, taskID, assignee, reviewer proto.ID, action *string) []string {
	var payloads []string
	if assignee != "" || reviewer != "" {
		assign := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{HomeCommand: secHomeHeader(h, c, "task"), Op: "assign", ID: taskID},
			Assignee:    assignee,
			Reviewer:    reviewer,
		}
		payloads = append(payloads, marshalSecWire(assign))
	}
	if action != nil {
		cmd := proto.TaskCommand{HomeCommand: secHomeHeader(h, c, "task"), Op: *action, ID: taskID}
		payloads = append(payloads, marshalSecWire(cmd))
	}
	return payloads
}

func callRequestDecision(ctx context.Context, h *Host, c Caller, raw stdjson.RawMessage) (any, error) {
	in, err := decodeSecInput[requestDecisionInput](raw)
	if err != nil {
		return nil, err
	}
	if err := guardSecTexts(decisionTexts(in)...); err != nil {
		return nil, err
	}
	post, err := prepareDecisionPost(ctx, h, c, in)
	if err != nil {
		return nil, err
	}
	if _, err := h.Command(ctx, c.Home, marshalSecWire(post)); err != nil {
		return nil, err
	}
	return secIDReply{ID: string(post.Nonce)}, nil
}

func decisionTexts(in requestDecisionInput) []string {
	texts := make([]string, 0, 2+2*len(in.Options))
	texts = append(texts, in.Question)
	if in.Context != nil {
		texts = append(texts, *in.Context)
	}
	for _, opt := range in.Options {
		texts = append(texts, opt.Label, opt.Effect)
	}
	return texts
}

func prepareDecisionPost(ctx context.Context, h *Host, c Caller, in requestDecisionInput) (proto.Post[proto.DecisionBody], error) {
	opts, recID, err := buildDecisionOptions(in.Options, in.Recommended)
	if err != nil {
		return proto.Post[proto.DecisionBody]{}, err
	}
	dir := loadSecDirectory(h.Rows, c.Group)
	toUsers, mentions, err := resolveDecisionRecipients(ctx, h, c, dir, in.To)
	if err != nil {
		return proto.Post[proto.DecisionBody]{}, err
	}
	thread, task := decisionTaskFields(c)
	return proto.Post[proto.DecisionBody]{
		HomeCommand: secHomeHeader(h, c, "post"),
		Kind:        "decision",
		Thread:      thread,
		Main:        true,
		Text:        "",
		Mentions:    mentions,
		Re:          nil,
		Body: proto.DecisionBody{
			Task:        task,
			Question:    in.Question,
			Context:     nonEmptyPtr(in.Context),
			Options:     opts,
			Recommended: recID,
			To:          toUsers,
		},
	}, nil
}

func buildDecisionOptions(inputs []decisionOptionInput, rec *int) ([]proto.DecisionOption, *string, error) {
	opts := make([]proto.DecisionOption, len(inputs))
	for i, in := range inputs {
		opts[i] = proto.DecisionOption{ID: optionID(i), Label: in.Label, Effect: in.Effect}
	}
	if rec == nil {
		return opts, nil, nil
	}
	if *rec < 0 || *rec >= len(opts) {
		return nil, nil, &Error{Code: "bad_request", Message: "recommended index out of range"}
	}
	id := opts[*rec].ID
	return opts, &id, nil
}

func optionID(index int) string {
	var buf []byte
	for n := index; ; n = n/26 - 1 {
		buf = append(buf, byte('A'+n%26))
		if n < 26 {
			slices.Reverse(buf)
			return string(buf)
		}
	}
}

func resolveDecisionRecipients(ctx context.Context, h *Host, c Caller, dir secDirectory, explicit *[]string) ([]string, []proto.ID, error) {
	if explicit != nil {
		return resolveExplicitDecisionRecipients(dir, *explicit)
	}
	if c.Kind == GroupSession {
		return nil, nil, &Error{Code: "bad_request", Message: "request_decision requires to in a group session"}
	}
	return resolveDefaultDecisionRecipient(ctx, h, c, dir)
}

func resolveExplicitDecisionRecipients(dir secDirectory, names []string) ([]string, []proto.ID, error) {
	if len(names) == 0 {
		return nil, nil, &Error{Code: "bad_request", Message: "request_decision requires at least one recipient"}
	}
	var users []string
	var mentions []proto.ID
	for _, name := range names {
		p, err := dir.person(name)
		if err != nil {
			return nil, nil, err
		}
		users = appendUnique(users, p.User)
		mentions = appendUnique(mentions, proto.ID(p.ID))
	}
	return users, mentions, nil
}

func appendUnique[T comparable](list []T, v T) []T {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

func resolveDefaultDecisionRecipient(ctx context.Context, h *Host, c Caller, dir secDirectory) ([]string, []proto.ID, error) {
	if briefID := taskBriefID(h.Rows, c.Group, c.Task); briefID != "" {
		reqID, err := readBriefRequester(ctx, h, c, briefID)
		if err != nil {
			return nil, nil, err
		}
		if req, ok := dir.byID[reqID]; ok {
			return recipientForUser(dir, req.User)
		}
	}
	return recipientForUser(dir, fallbackOwnerUser(h, c, dir))
}

func taskBriefID(rows *Rows, group, taskID string) string {
	raw, ok := rows.Get(group, "task", taskID)
	if !ok {
		return ""
	}
	var task proto.TaskRow
	if proto.Decode(raw, &task) != nil || task.Origin != "brief" {
		return ""
	}
	return task.Brief
}

func readBriefRequester(ctx context.Context, h *Host, c Caller, briefID string) (string, error) {
	raw, ok := h.Rows.Get(c.Group, "msg", briefID)
	if !ok {
		fetched, err := fetchSingleMsg(ctx, h, c, briefID)
		if err != nil {
			return "", err
		}
		raw = fetched
	}
	return decodeBriefRequester(raw)
}

func fetchSingleMsg(ctx context.Context, h *Host, c Caller, msgID string) (stdjson.RawMessage, error) {
	build := func(id int64) any {
		return proto.FetchRow{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: 0, Limit: 1},
			Row:       "msg",
			RowID:     msgID,
		}
	}
	rows, err := secFetchRows(ctx, h, c.Home, build)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func decodeBriefRequester(raw stdjson.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var msg proto.MsgRow
	if proto.Decode(raw, &msg) != nil || msg.Kind != "brief" {
		return "", &Error{Code: "bad_request", Message: "invalid brief message row"}
	}
	var body proto.BriefBody
	if proto.Decode(msg.Body, &body) != nil {
		return "", &Error{Code: "bad_request", Message: "invalid brief body"}
	}
	return body.Requester, nil
}

func recipientForUser(dir secDirectory, user string) ([]string, []proto.ID, error) {
	if user == "" {
		return nil, nil, &Error{Code: "not_found", Message: "no recipient found for decision"}
	}
	mentions := []proto.ID{}
	for _, name := range dir.people {
		if m := dir.byName[name]; m.User == user {
			mentions = append(mentions, proto.ID(m.ID))
		}
	}
	return []string{user}, mentions, nil
}

func fallbackOwnerUser(h *Host, c Caller, dir secDirectory) string {
	if self, ok := dir.byID[c.Member]; ok {
		return self.User
	}
	if len(dir.people) > 0 {
		return dir.byName[dir.people[0]].User
	}
	return readStoreOwner(h.Store)
}

const selectOwnerSettingSQL = `SELECT coalesce((SELECT value FROM setting WHERE key = 'owner'), '""')`

func readStoreOwner(db *store.Store) string {
	var raw string
	scan := func(tx *store.Tx) error {
		return tx.QueryRow(selectOwnerSettingSQL).Scan(&raw)
	}
	db.Write(scan)
	var owner string
	stdjson.Unmarshal([]byte(raw), &owner)
	return owner
}

func decisionTaskFields(c Caller) (*proto.ID, *string) {
	if c.Kind == GroupSession {
		return nil, nil
	}
	tid := proto.ID(c.Task)
	task := c.Task
	return &tid, &task
}

func nonEmptyPtr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}
