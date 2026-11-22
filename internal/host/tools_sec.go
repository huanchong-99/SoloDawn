package host

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"slices"
	str "strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var (
	proposeBriefSchema = stdjson.RawMessage(`{"type":"object","properties":{"criteria":{"type":"array","items":{"type":"string"}},"goal":{"type":"string"},"replaces":{"type":"string"},"requester":{"type":"string"},"scope_in":{"type":"array","items":{"type":"string"}},"scope_out":{"type":"array","items":{"type":"string"}},"tasks":{"type":"array","items":{"type":"object","properties":{"assignee":{"type":"string"},"depends":{"type":"array","default":[],"items":{"type":"integer","minimum":0}},"foundation":{"type":"boolean","default":false},"reviewer":{"type":"string"},"title":{"type":"string"}},"required":["title","assignee","reviewer"],"additionalProperties":false}},"title":{"type":"string"}},"required":["title","goal","criteria","scope_in","scope_out","tasks"],"additionalProperties":false}`)

	setTaskSchema = stdjson.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["pause","resume","cancel"]},"assignee":{"type":"string"},"reviewer":{"type":"string"},"task":{"type":"string"}},"required":["task"],"additionalProperties":false}`)

	requestDecisionSchema = stdjson.RawMessage(`{"type":"object","properties":{"context":{"type":"string"},"options":{"type":"array","items":{"type":"object","properties":{"effect":{"type":"string"},"label":{"type":"string"}},"required":["label","effect"],"additionalProperties":false}},"question":{"type":"string"},"recommended":{"type":"integer","minimum":0},"to":{"type":"array","items":{"type":"string"}}},"required":["question","options","to"],"additionalProperties":false}`)
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
	if proto.Decode(raw, &dst) != nil {
		return dst, &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	return dst, nil
}

func secHomeHeader(h *Host, c Caller, kind string) proto.HomeCommand {
	return homeHeader(h, c.Group, c.Member, kind)
}

type secDirectory struct {
	byName  map[string]proto.MemberRow
	members []string
	people  []string
}

func loadSecDirectory(rows *Rows, group string) secDirectory {
	dir := secDirectory{byName: map[string]proto.MemberRow{}}
	for _, m := range liveMembers(rows, group) {
		dir.byName[m.Name] = m
		dir.members = append(dir.members, m.Name)
		if m.Kind == "human" {
			dir.people = append(dir.people, m.Name)
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
	if len(valid) != 0 {
		names = str.Join(valid, ", ")
	}
	return &Error{
		Code:    "not_found",
		Message: fmt.Sprintf("unknown %s %q (valid: %s)", kind, given, names),
	}
}

func callProposeBrief(ctx context.Context, h *Host, c Caller, raw stdjson.RawMessage) (any, error) {
	in, err := decodeSecInput[proposeBriefInput](raw)
	if err != nil {
		return nil, err
	}
	if err := checkBriefSecrets(in); err != nil {
		return nil, err
	}
	body, requester, err := prepareBriefBody(h.Rows, c.Group, in)
	if err != nil {
		return nil, err
	}
	hdr := secHomeHeader(h, c, "post")
	post := proto.Post[proto.BriefBody]{
		HomeCommand: hdr,
		Kind:        "brief",
		Mentions:    []proto.ID{requester},
		Body:        body,
	}
	if _, err := h.Command(ctx, c.Home, marshalWire(post)); err != nil {
		return nil, err
	}
	return secIDReply{ID: string(hdr.Nonce)}, nil
}

func checkBriefSecrets(in proposeBriefInput) error {
	for _, s := range slices.Concat([]string{in.Title, in.Goal}, in.Criteria, in.ScopeIn, in.ScopeOut) {
		if err := CheckOutboundSecret(s); err != nil {
			return err
		}
	}
	for _, t := range in.Tasks {
		if err := CheckOutboundSecret(t.Title); err != nil {
			return err
		}
	}
	return nil
}

func prepareBriefBody(rows *Rows, group string, in proposeBriefInput) (proto.BriefBody, proto.ID, error) {
	pass, err := readGroupPass(rows, group)
	if err != nil {
		return proto.BriefBody{}, "", err
	}
	dir := loadSecDirectory(rows, group)
	tasks, err := buildBriefTasks(dir, in.Tasks)
	if err != nil {
		return proto.BriefBody{}, "", err
	}
	requester, err := resolveBriefRequester(dir, in.Requester)
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
	var foundation bool
	if item.Foundation != nil {
		foundation = *item.Foundation
	}
	return proto.BriefTask{
		Title:      item.Title,
		Assignee:   string(assignee),
		Reviewer:   string(reviewer),
		Foundation: foundation,
		Depends:    cleanBriefDepends(item.Depends),
	}, nil
}

func cleanBriefDepends(raw *[]int) []int {
	if raw == nil {
		return []int{}
	}
	return *raw
}

func resolveBriefRequester(dir secDirectory, explicit *string) (proto.ID, error) {
	if explicit != nil {
		return dir.memberID(*explicit)
	}
	switch len(dir.people) {
	case 1:
		return proto.ID(dir.byName[dir.people[0]].ID), nil
	case 0:
		return "", &Error{Code: "bad_request", Message: "propose_brief requires requester when the group has no person"}
	default:
		return "", &Error{
			Code:    "bad_request",
			Message: fmt.Sprintf("propose_brief requires requester when the group has more than one person (%s)", str.Join(dir.people, ", ")),
		}
	}
}

type setTaskPlan struct {
	assign string
	action string
	op     string
}

func callSetTask(ctx context.Context, h *Host, c Caller, raw stdjson.RawMessage) (any, error) {
	in, err := decodeSecInput[setTaskInput](raw)
	if err != nil {
		return nil, err
	}
	plan, err := prepareSetTask(h, c, in)
	if err != nil {
		return nil, err
	}
	if err := runSetTask(ctx, h, c.Home, plan); err != nil {
		return nil, err
	}
	return secOKReply{OK: true}, nil
}

func prepareSetTask(h *Host, c Caller, in setTaskInput) (setTaskPlan, error) {
	if !hasSetTaskChange(in) {
		return setTaskPlan{}, &Error{Code: "bad_request", Message: "set_task requires assignee, reviewer or action"}
	}
	taskID, err := resolveTaskID(h, c.Group, in.Task)
	if err != nil {
		return setTaskPlan{}, err
	}
	dir := loadSecDirectory(h.Rows, c.Group)
	assignee, reviewer, err := resolveSetTaskRoles(dir, in)
	if err != nil {
		return setTaskPlan{}, err
	}
	return buildSetTaskPlan(h, c, taskID, assignee, reviewer, in.Action), nil
}

func hasSetTaskChange(in setTaskInput) bool {
	return in.Assignee != nil || in.Reviewer != nil || in.Action != nil
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

func buildSetTaskPlan(h *Host, c Caller, taskID, assignee, reviewer proto.ID, action *string) setTaskPlan {
	var plan setTaskPlan
	if assignee != "" || reviewer != "" {
		assign := proto.TaskAssign{
			TaskCommand: proto.TaskCommand{HomeCommand: secHomeHeader(h, c, "task"), Op: "assign", ID: taskID},
			Assignee:    assignee,
			Reviewer:    reviewer,
		}
		plan.assign = marshalWire(assign)
	}
	if action != nil {
		cmd := proto.TaskCommand{HomeCommand: secHomeHeader(h, c, "task"), Op: *action, ID: taskID}
		plan.action = marshalWire(cmd)
		plan.op = *action
	}
	return plan
}

func runSetTask(ctx context.Context, h *Host, home string, plan setTaskPlan) error {
	if plan.assign != "" {
		if _, err := h.Command(ctx, home, plan.assign); err != nil {
			return err
		}
	}
	if plan.action == "" {
		return nil
	}
	if _, err := h.Command(ctx, home, plan.action); err != nil {
		return afterAssignError(plan.assign != "", plan.op, err)
	}
	return nil
}

func afterAssignError(assigned bool, op string, err error) error {
	if !assigned {
		return err
	}
	he := err.(*Error)
	return &Error{
		Code:    he.Code,
		Message: fmt.Sprintf("task assign succeeded before task %s failed: %s", op, he.Message),
	}
}

func callRequestDecision(ctx context.Context, h *Host, c Caller, raw stdjson.RawMessage) (any, error) {
	in, err := decodeSecInput[requestDecisionInput](raw)
	if err != nil {
		return nil, err
	}
	if err := checkDecisionSecrets(in); err != nil {
		return nil, err
	}
	post, err := prepareDecisionPost(h, c, in)
	if err != nil {
		return nil, err
	}
	if _, err := h.Command(ctx, c.Home, marshalWire(post)); err != nil {
		return nil, err
	}
	return secIDReply{ID: string(post.Nonce)}, nil
}

func checkDecisionSecrets(in requestDecisionInput) error {
	texts := []string{in.Question}
	if in.Context != nil {
		texts = append(texts, *in.Context)
	}
	for _, opt := range in.Options {
		texts = append(texts, opt.Label, opt.Effect)
	}
	for _, s := range texts {
		if err := CheckOutboundSecret(s); err != nil {
			return err
		}
	}
	return nil
}

func prepareDecisionPost(h *Host, c Caller, in requestDecisionInput) (proto.Post[proto.DecisionBody], error) {
	opts, recID, err := buildDecisionOptions(in.Options, in.Recommended)
	if err != nil {
		return proto.Post[proto.DecisionBody]{}, err
	}
	dir := loadSecDirectory(h.Rows, c.Group)
	toUsers, mentions, err := resolveDecisionRecipients(dir, in.To)
	if err != nil {
		return proto.Post[proto.DecisionBody]{}, err
	}
	thread, task := decisionTaskFields(c)
	return proto.Post[proto.DecisionBody]{
		HomeCommand: secHomeHeader(h, c, "post"),
		Kind:        "decision",
		Thread:      thread,
		Mentions:    mentions,
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
		opts[i] = decisionOption(i, in.Label, in.Effect)
	}
	if rec == nil {
		return opts, nil, nil
	}
	if *rec >= len(opts) {
		return nil, nil, &Error{Code: "bad_request", Message: "recommended index out of range"}
	}
	id := opts[*rec].ID
	return opts, &id, nil
}

func decisionOption(i int, label, effect string) proto.DecisionOption {
	return proto.DecisionOption{ID: string(rune('A' + i)), Label: label, Effect: effect}
}

func resolveDecisionRecipients(dir secDirectory, explicit *[]string) ([]string, []proto.ID, error) {
	names := dir.people
	if explicit != nil {
		names = *explicit
	}
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
