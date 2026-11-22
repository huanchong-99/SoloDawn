//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"

	"github.com/cucumber/godog"
)

type s2ConfirmTool struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	Result string         `json:"result"`
}

func s2ConfirmScriptSteps(sc *godog.ScenarioContext, w *s2ConfirmWorld) {
	sc.Step(`^"([^"]+)" is scripted for confirmation tools:$`, w.script)
	sc.Step(`^"([^"]+)" is waiting after its confirmation turn$`, w.awaitIdle)
	sc.Step(`^"([^"]+)" has a task commit "([^"]+)"$`, w.commitWork)
	sc.Step(`^the secretary proposes a task "([^"]+)" for "([^"]+)"$`, w.proposeTask)
	sc.Step(`^"([^"]+)" proposes a confirmation task "([^"]+)" for "([^"]+)"$`, func(ai, title, assignee string) error {
		return w.propose(ai, title, assignee, "brief", "")
	})
	sc.Step(`^a confirmed personal task named "([^"]+)"$`, w.personalTask)
	sc.Step(`^the secretary proposes replacement task "([^"]+)"$`, w.replacementTask)
	sc.Step(`^"Q-personal" requests the Codex confirmation approval$`, w.codexApproval)
	sc.Step(`^a Q-only brief with assignment by "([^"]+)"$`, w.qOnlyBrief)
	sc.Step(`^"([^"]+)" gives the Q-only task to P-work$`, w.assignQTask)
	sc.Step(`^the task for P-work (waits for|does not ask) its owner's word$`, w.roleConfirmation)
}

func (w *s2ConfirmWorld) codexApproval() error {
	return w.cast.WriteScript(w.cast.Q1, s2QPersonalCLI, m5MustJSON(m6CodexHoldDoc()))
}

func (w *s2ConfirmWorld) proposeTask(title, assignee string) error {
	return w.propose(s2SecretaryName, title, assignee, "brief", "")
}

func (w *s2ConfirmWorld) propose(ai, title, assignee, name, replaces string) error {
	reviewer := s2PWorkName
	if ai == s2PWorkName || assignee == s2PWorkName {
		reviewer = "P-review"
	}
	return w.proposeWithReviewer(ai, title, assignee, reviewer, name, replaces)
}

func (w *s2ConfirmWorld) proposeWithReviewer(ai, title, assignee, reviewer, name, replaces string) error {
	args := map[string]any{
		"title": title, "goal": "Check the project path", "criteria": []string{"Path visible"},
		"scope_in": []string{"path"}, "scope_out": []string{}, "requester": "P",
		"tasks": []map[string]string{{"title": title, "assignee": assignee, "reviewer": reviewer}},
	}
	if replaces != "" {
		args["replaces"] = replaces
	}
	agent := w.ais[ai]
	script := m5ClaudeScript([]m5ToolCallSpec{{Name: "propose_brief", Args: args}}, nil)
	if err := w.cast.WriteScript(w.aiNode(ai), agent.CLI, script); err != nil {
		return err
	}
	if err := w.mention("P", ai, "main", "Plan "+title, "plan"); err != nil {
		return err
	}
	return w.awaitRow("msg", name, &godog.DocString{Content: m5MustJSON(map[string]any{"kind": "brief", "body": map[string]string{"title": title}})})
}

func (w *s2ConfirmWorld) personalTask(title string) error {
	if err := w.proposeTask(title, s2QPersonalName); err != nil {
		return err
	}
	if err := w.confirm("P", "brief"); err != nil {
		return err
	}
	if err := w.awaitRow("msg", "card", &godog.DocString{Content: `{"kind":"confirm","re":"${brief}","body":{"ai":"${Q-personal}"}}`}); err != nil {
		return err
	}
	if err := w.confirm("Q", "card"); err != nil {
		return err
	}
	if err := w.awaitRow("task", "task", &godog.DocString{Content: `{"brief":"${brief}","assignee":"${Q-personal}"}`}); err != nil {
		return err
	}
	return w.awaitTurn(s2QPersonalName, `^== .* · assign · instruction$`)
}

func (w *s2ConfirmWorld) confirm(actor, card string) error {
	if err := w.act(actor, "confirm", card); err != nil {
		return err
	}
	return w.answer("ok")
}

func (w *s2ConfirmWorld) replacementTask(title string) error {
	if err := w.observe(func() (bool, error) {
		ids, err := w.matchRows("member", map[string]any{"id": w.ais[s2SecretaryName].ID, "state": "dormant"})
		return len(ids) == 1, err
	}); err != nil {
		return err
	}
	return w.propose(s2SecretaryName, title, s2QPersonalName, "replacement", w.ids["brief"])
}

func (w *s2ConfirmWorld) qOnlyBrief(assigner string) error {
	args := map[string]any{
		"title": "Q-only path", "goal": "Check the path", "criteria": []string{"Path visible"},
		"scope_in": []string{"path"}, "scope_out": []string{}, "requester": "P",
		"tasks": []map[string]string{{"title": "Q-only path", "assignee": s2QTeamName, "reviewer": s2QPersonalName}},
	}
	var assign []m5ToolCallSpec
	if assigner == s2SecretaryName {
		assign = []m5ToolCallSpec{{Name: "set_task", Args: map[string]string{"task": "T-1", "assignee": s2PWorkName}}}
	}
	script := m5ClaudeScript([]m5ToolCallSpec{{Name: "propose_brief", Args: args}}, nil, assign)
	if err := w.cast.WriteScript(w.cast.P1, s2SecretaryCLI, script); err != nil {
		return err
	}
	if err := w.mention("P", s2SecretaryName, "main", "Plan the Q-only path", "plan"); err != nil {
		return err
	}
	if err := w.awaitRow("msg", "brief", &godog.DocString{Content: `{"kind":"brief","body":{"title":"Q-only path"}}`}); err != nil {
		return err
	}
	if err := w.confirm("P", "brief"); err != nil {
		return err
	}
	return w.awaitRow("task", "task", &godog.DocString{Content: `{"brief":"${brief}","title":"Q-only path","num":1}`})
}

func (w *s2ConfirmWorld) assignQTask(assigner string) error {
	if assigner == s2SecretaryName {
		return w.mention("P", s2SecretaryName, "main", "Give the path to P-work", "assign")
	}
	if err := w.send(assigner, "assign", map[string]any{"t": "task", "op": "assign", "id": w.ids["task"], "assignee": w.ids[s2PWorkName]}); err != nil {
		return err
	}
	return w.answer("ok")
}

func (w *s2ConfirmWorld) roleConfirmation(outcome string) error {
	if err := w.awaitRow("task", "task", &godog.DocString{Content: `{"id":"${task}","assignee":"${P-work}"}`}); err != nil {
		return err
	}
	if outcome == "does not ask" {
		return w.countRows("msg", 0, &godog.DocString{Content: `{"kind":"confirm","body":{"ai":"${P-work}"}}`})
	}
	if err := w.awaitRow("msg", "role-card", &godog.DocString{Content: `{"kind":"confirm","re":"${brief}","body":{"ai":"${P-work}","to":["${P}"]},"state":{"status":"pending"}}`}); err != nil {
		return err
	}
	if err := w.countItems(s2PWorkName, 0, "assign"); err != nil {
		return err
	}
	return w.confirm("P", "role-card")
}

func (w *s2ConfirmWorld) script(ai string, doc *godog.DocString) error {
	input, err := w.expand(doc.Content)
	if err != nil {
		return err
	}
	var turns [][]s2ConfirmTool
	if err := json.Unmarshal([]byte(input), &turns); err != nil {
		return err
	}
	cli := w.ais[ai].CLI
	return w.cast.WriteScript(w.aiNode(ai), cli, s2ConfirmScript(cli, turns))
}

func s2ConfirmScript(cli string, turns [][]s2ConfirmTool) string {
	if cli == "claude" {
		converted := make([][]m5ToolCallSpec, len(turns))
		for i, calls := range turns {
			for _, call := range calls {
				converted[i] = append(converted[i], m5ToolCallSpec{Name: call.Name, Args: call.Args})
			}
		}
		return m5ClaudeScript(converted...)
	}
	steps := codexStartupSteps()
	id := 2
	for i, calls := range turns {
		steps = append(steps, codexToolTurn(fmt.Sprintf("confirm-%d", i+1), "", s2ConfirmCalls(calls, id))...)
		id += len(calls)
	}
	return m5MustJSON(map[string]any{"session": "s2-confirm-codex", "steps": steps})
}

func s2ConfirmCalls(calls []s2ConfirmTool, id int) []map[string]any {
	var steps []map[string]any
	for i, call := range calls {
		result := map[string]any{"isError": false}
		args := call.Args
		if args == nil {
			args = map[string]any{}
		}
		if call.Result != "" {
			result = m6ToolTextResult(false, call.Result)
		}
		steps = append(steps, mcpCallSteps(id+i, call.Name, args, result)...)
	}
	return steps
}
