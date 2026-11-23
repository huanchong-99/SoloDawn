//go:build acceptance

package features

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func serverProfileFixtures(f fixtures, cleanup func(func())) (fixtures, error) {
	dir, err := os.MkdirTemp("", "solodawn-server-profiles-*")
	if err != nil {
		return f, err
	}
	cleanup(func() { os.RemoveAll(dir) })
	if out, err := exec.Command("go", "build", "-o", dir+string(os.PathSeparator), "../internal/host/fakecli/opencode", "../internal/host/fakecli/qwen").CombinedOutput(); err != nil {
		return f, fmt.Errorf("build server fake CLIs: %s: %w", out, err)
	}
	f.fakeCLIs = dir
	return f, nil
}

func (w *hostModelsWorld) serverListed() error {
	want := [][]proto.Model{
		{{Model: "anthropic/claude-opus-5", Name: "Opus", Efforts: []string{"low", "high"}, Tier: "strong"}, {Model: "openai/gpt-6-mini", Name: "Mini", Efforts: []string{}, Tier: "light"}},
		{},
	}
	return hostModelsEqual([]any{w.state.Agents[0].Agent, w.state.Agents[1].Agent, w.state.Agents[0].Models, w.state.Agents[1].Models}, []any{"opencode", "qwen", want[0], want[1]})
}

func serverChoiceScript(s host.Session, p proto.Profile, refused bool) string {
	if s.Agent == "opencode" {
		return openCodeChoiceScript(s, p, refused)
	}
	return qwenChoiceScript(s, p, refused)
}

func serverHTTP(call string, body, res any, status int) any {
	return map[string]any{"dir": "http", "msg": map[string]any{"call": call, "body": body, "res": res, "status": status}}
}

func serverTurnScript(s host.Session, id string, steps []any, refused bool) string {
	if !refused {
		prompt := serverHTTP("POST /session/"+id+"/prompt_async", map[string]any{}, nil, 200)
		event := map[string]any{"type": "session.idle", "properties": map[string]string{"sessionID": id}}
		if s.Agent == "qwen" {
			prompt = serverHTTP("POST /session/"+id+"/prompt", map[string]any{}, nil, 200)
			event = map[string]any{"type": "turn_complete", "data": map[string]string{"stopReason": "end_turn"}}
		}
		steps = append(steps, prompt, map[string]any{"dir": "sse", "msg": event})
	}
	steps = append(steps, serverHTTP("POST /never", map[string]any{}, nil, 200))
	return accJSON(map[string]any{"session": id, "steps": steps})
}

func openCodeChoiceScript(s host.Session, p proto.Profile, refused bool) string {
	id := s.CLISession
	var open any
	if id == "" {
		id = "server-session"
		open = serverHTTP("POST /session", map[string]any{}, map[string]string{"id": id}, 200)
	} else {
		open = serverHTTP("GET /session/"+id, nil, map[string]string{"id": id}, 200)
	}
	steps := []any{open}
	if p.Model != "" {
		provider, model, _ := strings.Cut(p.Model, "/")
		choice := map[string]string{"providerID": provider, "id": model}
		if p.Effort != "" {
			choice["variant"] = p.Effort
		}
		status := 200
		if refused {
			status = 400
		}
		steps = append(steps, serverHTTP("POST /api/session/"+id+"/model", map[string]any{"model": choice}, nil, status))
	}
	return serverTurnScript(s, id, steps, refused)
}

func qwenChoiceScript(s host.Session, p proto.Profile, refused bool) string {
	id, call := "server-session", "POST /session"
	config := map[string]string{}
	if p.Model != "" {
		config["modelServiceId"] = p.Model
	}
	if p.Effort != "" {
		config["reasoningEffort"] = p.Effort
	}
	body := map[string]any{}
	if len(config) != 0 {
		body["startupConfig"] = config
	}
	if s.CLISession != "" {
		id = s.CLISession
		call += "/" + id + "/resume"
		body = map[string]any{}
	}
	status := 200
	if refused {
		status = 400
	}
	steps := []any{serverHTTP(call, body, map[string]string{"sessionId": id}, status)}
	return serverTurnScript(s, id, steps, refused)
}
