//go:build acceptance

package features

import (
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func s3AgentSet(w *hostCapsWorld, agent, account string) proto.AgentSet {
	return proto.AgentSet{HostCommand: w.header("agent.set"), Agent: agent, Account: account}
}

type s3Turn struct {
	body      []map[string]any
	outside   string
	crash     bool
	interrupt bool
}

func s3PostTurn(id int, text string) s3Turn { return s3Turn{body: mcpPostSteps(id, text)} }

func (w *s3DriverWorld) script(resume, park bool, turns ...s3Turn) error {
	steps := []any{w.open(resume)}
	for _, step := range claudeInitMCP(0) {
		steps = append(steps, step)
	}
	for _, turn := range turns {
		steps = append(steps, w.turn(turn)...)
	}
	if park {
		steps = append(steps, serverHTTP("POST /never", map[string]any{}, nil, 200))
	}
	return w.caps.m3.setScript(w.agent, map[string]any{"session": "s3-session", "steps": steps})
}

func (w *s3DriverWorld) open(resume bool) any {
	call, result := "POST /session", map[string]string{"id": "s3-session"}
	if resume {
		call = "GET /session/s3-session"
	}
	var body any = map[string]any{}
	if w.agent == "qwen" {
		result = map[string]string{"sessionId": "s3-session"}
		if resume {
			call = "POST /session/s3-session/resume"
		}
	} else if resume {
		body = nil
	}
	return serverHTTP(call, body, result, 200)
}

func (w *s3DriverWorld) turn(turn s3Turn) []any {
	path := "/session/s3-session/prompt_async"
	if w.agent == "qwen" {
		path = "/session/s3-session/prompt"
	}
	steps := []any{serverHTTP("POST "+path, map[string]any{}, nil, 200)}
	if turn.crash {
		return steps
	}
	if turn.outside != "" {
		steps = append(steps, w.outsideEvent(turn.outside))
	}
	for _, step := range turn.body {
		steps = append(steps, step)
	}
	if turn.interrupt {
		steps = append(steps, w.interrupt()...)
	}
	return append(steps, w.endEvent(turn.interrupt))
}

func (w *s3DriverWorld) endEvent(interrupted bool) any {
	event := map[string]any{"type": "session.idle", "properties": map[string]string{"sessionID": "s3-session"}}
	if w.agent == "qwen" {
		reason := "end_turn"
		if interrupted {
			reason = "cancelled"
		}
		event = map[string]any{"type": "turn_complete", "data": map[string]string{"stopReason": reason}}
	}
	return map[string]any{"dir": "sse", "msg": event}
}

func (w *s3DriverWorld) outsideEvent(text string) any {
	event := map[string]any{"type": "message.part.updated", "properties": map[string]any{"part": map[string]string{"type": "text", "text": text, "sessionID": "s3-session", "messageID": "s3-message", "id": "s3-part"}}}
	if w.agent == "qwen" {
		event = map[string]any{"type": "session_update", "data": map[string]any{"update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}}}}
	}
	return map[string]any{"dir": "sse", "msg": event}
}

func (w *s3DriverWorld) interrupt() []any {
	if w.agent == "qwen" {
		return []any{serverHTTP("POST /session/s3-session/cancel", map[string]any{}, nil, 200)}
	}
	return []any{serverHTTP("POST /session/s3-session/abort", map[string]any{}, nil, 200), map[string]any{"dir": "sse", "msg": map[string]any{"type": "session.error", "properties": map[string]any{"sessionID": "s3-session", "error": map[string]string{"name": "MessageAbortedError"}}}}}
}
