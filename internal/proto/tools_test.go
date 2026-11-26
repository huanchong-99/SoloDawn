package proto

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// The argument schemas of contracts/mcp-tools.md, sections 1 and 2.

func text() ToolArg               { return ToolArg{Type: "string"} }
func texts() ToolArg              { return ToolArg{Type: "array", Items: new(text())} }
func choice(of ...string) ToolArg { return ToolArg{Type: "string", Enum: of} }

func integer(minimum int, maximum ...int) ToolArg {
	arg := ToolArg{Type: "integer", Minimum: new(minimum)}
	if len(maximum) > 0 {
		arg.Maximum = new(maximum[0])
	}
	return arg
}

func boolean(def bool) ToolArg { return ToolArg{Type: "boolean", Default: def} }

// list is an array of objects, each with these members, of which required
// must be there.
func list(members map[string]ToolArg, required ...string) ToolArg {
	return ToolArg{Type: "array", Items: &ToolArg{Type: "object", Properties: members, Required: required, AdditionalProperties: new(false)}}
}

func tool(name string, args map[string]ToolArg, required ...string) Tool {
	if args == nil {
		args = map[string]ToolArg{}
	}
	return Tool{name, ToolInput{"object", args, required, false}}
}

// session is what tells the lists apart: the kinds a post may have, and
// whether the session is a group session, where no task is the session's own.
type session struct {
	posts []string
	group bool
}

// unless leaves out of a required list the task a session has of its own.
func (s session) unless(required ...string) []string {
	if s.group {
		return required
	}
	return slices.DeleteFunc(required, func(arg string) bool { return arg == "task" || arg == "to" })
}

func everyAI(s session, secretary bool) []Tool {
	post := map[string]ToolArg{
		"text":     text(),
		"kind":     {Type: "string", Enum: s.posts, Default: "text"},
		"mentions": texts(),
		"where":    choice("thread", "main"),
	}
	if secretary {
		post["task"] = text()
	}
	limit := integer(1, 100)
	limit.Default = 30
	kbLimit := integer(1, 20)
	kbLimit.Default = 3
	return []Tool{
		tool("post", post, "text"),
		tool("ask", map[string]ToolArg{"to": text(), "text": text(), "wait": boolean(true)}, "to", "text"),
		tool("reply", map[string]ToolArg{"call": text(), "text": text()}, "call", "text"),
		tool("read_thread", map[string]ToolArg{"task": text(), "limit": limit, "before": integer(1)}, s.unless("task")...),
		tool("list_members", nil),
		tool("list_tasks", map[string]ToolArg{"state": choice("queued", "active", "paused", "failed", "done", "cancelled")}),
		tool("note_put", map[string]ToolArg{"text": text(), "task": text()}, "text"),
		tool("note_list", map[string]ToolArg{"task": text(), "type": choice("preset")}),
		tool("kb_put", map[string]ToolArg{"name": text(), "text": text()}, "name", "text"),
		tool("kb_search", map[string]ToolArg{"query": text(), "limit": kbLimit}, "query"),
	}
}

func requestDecision(s session) Tool {
	return tool("request_decision", map[string]ToolArg{
		"question":    text(),
		"options":     list(map[string]ToolArg{"label": text(), "effect": text()}, "label", "effect"),
		"recommended": integer(0),
		"context":     text(),
		"to":          texts(),
	}, s.unless("question", "options", "to")...)
}

var netStatus = tool("net_status", nil)

func toolLists() ToolLists {
	// A takeover can reach every kind of session but an external AI's (contracts/mcp-tools.md section 5).
	taken := []string{"text", "progress", "question", "handback"}
	secretary := session{append(slices.Clone(taken), "summary"), true}
	group := session{taken, true}
	task := session{taken, false}
	depends := ToolArg{Type: "array", Items: new(integer(0)), Default: []any{}}
	brief := tool("propose_brief", map[string]ToolArg{
		"title": text(), "goal": text(), "criteria": texts(), "scope_in": texts(), "scope_out": texts(),
		"tasks": list(map[string]ToolArg{
			"title": text(), "assignee": text(), "reviewer": text(), "foundation": boolean(false), "depends": depends,
		}, "title", "assignee", "reviewer"),
		"requester": text(),
		"replaces":  text(),
		"preset":    text(),
	}, "title", "goal", "criteria", "scope_in", "scope_out", "tasks")
	setTask := tool("set_task", map[string]ToolArg{
		"task": text(), "assignee": text(), "reviewer": text(), "action": choice("pause", "resume", "cancel"),
	}, "task")
	startTask := tool("start_task", map[string]ToolArg{"title": text(), "plan": text()}, "title", "plan")
	review := tool("submit_review", map[string]ToolArg{
		"score": integer(0, 100),
		"items": list(map[string]ToolArg{"text": text(), "file": text(), "line": integer(1), "veto": boolean(false)}, "text"),
	}, "score", "items")
	submit := tool("submit_task", map[string]ToolArg{"summary": text()}, "summary")
	external := session{posts: []string{"text", "progress", "question"}}
	return ToolLists{
		Secretary: ToolList{slices.Concat(everyAI(secretary, true), []Tool{requestDecision(secretary), netStatus, brief, setTask})},
		Group:     ToolList{slices.Concat(everyAI(group, false), []Tool{startTask, requestDecision(group), netStatus})},
		Review:    ToolList{slices.Concat(everyAI(task, false), []Tool{review, requestDecision(task), netStatus})},
		Task:      ToolList{slices.Concat(everyAI(task, false), []Tool{submit, requestDecision(task), netStatus})},
		External: ToolList{slices.Concat(everyAI(external, false), []Tool{submit,
			startTask, review, requestDecision(external), netStatus,
			tool("next_item", nil),
			tool("done", map[string]ToolArg{"run": {Type: "integer"}}, "run"),
			tool("report_usage", map[string]ToolArg{
				"window_pct":    {Type: "number", Minimum: new(0), Maximum: new(100)},
				"window_resets": {Type: "integer"},
			}, "window_pct"),
		})},
	}
}

// readTools reads testdata/tools.json with encoding/json, refusing what the
// types do not know.
func readTools(t *testing.T, data []byte) ToolLists {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var lists ToolLists
	if err := d.Decode(&lists); err != nil {
		t.Fatal(err)
	}
	return lists
}

func TestToolListGolden(t *testing.T) {
	encoded := wireJSON(t, toolLists())
	path := filepath.Join("testdata", "tools.json")
	if *update {
		var pretty bytes.Buffer
		sure(json.Indent(&pretty, encoded, "", "  "))
		pretty.WriteByte('\n')
		if err := os.WriteFile(path, pretty.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data := example(t, "tools")
	if !reflect.DeepEqual(jsonValue(t, encoded), jsonValue(t, data)) {
		t.Fatalf("encoded %s\ngolden  %s", encoded, data)
	}
	if roundTrip := wireJSON(t, readTools(t, data)); !reflect.DeepEqual(jsonValue(t, roundTrip), jsonValue(t, data)) {
		t.Fatalf("round trip %s\nwant       %s", roundTrip, data)
	}
	var extra map[string]any
	sure(json.Unmarshal(data, &extra))
	extra["secretary"].(map[string]any)["tools"].([]any)[0].(map[string]any)["description"] = "Post a message."
	d := json.NewDecoder(bytes.NewReader(wireJSON(t, extra)))
	d.DisallowUnknownFields()
	if err := d.Decode(new(ToolLists)); err == nil {
		t.Error("a tool with a description is read")
	}
}

func names(list ToolList) []string {
	names := []string{}
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// Who sees which tool: the last column of the tables of contracts/mcp-tools.md,
// the notes of section 1, and section 2.
func TestToolListsFollowTheSessionKinds(t *testing.T) {
	lists := readTools(t, example(t, "tools"))
	everyone := []string{"post", "ask", "reply", "read_thread", "list_members", "list_tasks", "note_put", "note_list", "kb_put", "kb_search", "request_decision", "net_status"}
	for kind, c := range map[string]struct {
		list ToolList
		also []string
	}{
		"secretary": {lists.Secretary, []string{"propose_brief", "set_task"}},
		"group":     {lists.Group, []string{"start_task"}},
		"review":    {lists.Review, []string{"submit_review"}},
		"task":      {lists.Task, []string{"submit_task"}},
		"external":  {lists.External, []string{"start_task", "submit_review", "submit_task", "next_item", "done", "report_usage"}},
	} {
		got, want := names(c.list), slices.Concat(everyone, c.also)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s lists %q, want %q", kind, got, want)
		}
		for _, tool := range c.list.Tools {
			if input := tool.InputSchema; input.Type != "object" || input.AdditionalProperties {
				t.Errorf("%s: %s takes %s arguments, other arguments too: %v", kind, tool.Name, input.Type, input.AdditionalProperties)
			}
			for _, arg := range tool.InputSchema.Required {
				if _, there := tool.InputSchema.Properties[arg]; !there {
					t.Errorf("%s: %s requires %s, which it does not take", kind, tool.Name, arg)
				}
			}
		}
	}
}

// In a group session a tool is given the task it is about, and whom a
// decision is for; in a task or review session they default to the session's
// own (contracts/mcp-tools.md, conventions).
func TestAGroupSessionNamesWhatATaskSessionHas(t *testing.T) {
	lists := readTools(t, example(t, "tools"))
	required := func(list ToolList, name string) []string {
		i := slices.IndexFunc(list.Tools, func(tool Tool) bool { return tool.Name == name })
		return list.Tools[i].InputSchema.Required
	}
	for _, list := range []ToolList{lists.Secretary, lists.Group} {
		if !slices.Contains(required(list, "read_thread"), "task") || !slices.Contains(required(list, "request_decision"), "to") {
			t.Errorf("a group session may leave out the task or the people: %q", names(list))
		}
	}
	for _, list := range []ToolList{lists.Review, lists.Task} {
		if slices.Contains(required(list, "read_thread"), "task") || slices.Contains(required(list, "request_decision"), "to") {
			t.Errorf("a task or review session must name its own task or people: %q", names(list))
		}
	}
	kinds := func(list ToolList) []string { return list.Tools[0].InputSchema.Properties["kind"].Enum }
	// A takeover reaches every kind of session but an external AI's; only the secretary posts a summary.
	for _, list := range []ToolList{lists.Group, lists.Review, lists.Task} {
		if !slices.Contains(kinds(list), "handback") || slices.Contains(kinds(list), "summary") {
			t.Errorf("post kinds %q in %q", kinds(list), names(list))
		}
	}
	if !slices.Contains(kinds(lists.Secretary), "handback") || !slices.Contains(kinds(lists.Secretary), "summary") || slices.Contains(kinds(lists.External), "handback") {
		t.Errorf("post kinds: secretary %q, external %q", kinds(lists.Secretary), kinds(lists.External))
	}
}
