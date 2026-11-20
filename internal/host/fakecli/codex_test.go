package fakecli

import (
	"fmt"
	"testing"
)

const thread = "01a1f63f-701c-78c1-bbf3-f1ac61c56b59"
const firstTurn = "01a1f63f-7034-7c60-a028-83d6e6401838"
const resumedTurn = "01a1f63f-cec1-7370-9b17-5a693c5b6580"

func initialize(t *testing.T, c *cli) {
	t.Helper()
	c.send(t, `{"method":"initialize","id":"hello-42","params":{"clientInfo":{"name":"test","version":"0"}}}`)
	c.next(t, "id", "hello-42", "result.platformOs", "macos")
	c.send(t, `{"method":"initialized"}`)
}

func userTurn(t *testing.T, c *cli, id, text string) {
	t.Helper()
	c.send(t, fmt.Sprintf(`{"method":"turn/start","id":42,"params":{"threadId":%q,"input":[{"type":"text","text":%q}]}}`, id, text))
}

func firstApproval(t *testing.T, c *cli) {
	t.Helper()
	initialize(t, c)
	c.send(t, `{"method":"thread/start","id":12,"params":{"cwd":"test"}}`)
	c.next(t, "id", 12.0, "result.thread.id", thread)
	c.next(t, "method", "thread/started", "params.thread.sessionId", thread)
	userTurn(t, c, thread, "Try the write.")
	c.next(t, "id", 42.0, "result.turn.id", firstTurn)
	c.next(t, "method", "turn/started", "params.turn.id", firstTurn)
	c.next(t, "method", "account/updated", "params.authMode", "chatgpt", "params.planType", "plus")
	c.next(t, "method", "item/started", "params.item.type", "commandExecution")
	c.next(t, "method", "item/commandExecution/requestApproval", "id", 0.0, "params.turnId", firstTurn, "params.threadId", thread)
}

func codexTurns(t *testing.T) {
	c := start(t, "codex", "testdata/codex-approval.json", "tok-first", "app-server")
	firstApproval(t, c)
	c.send(t, `{"id":0,"result":{"decision":"decline"}}`)
	c.next(t, "method", "serverRequest/resolved", "params.requestId", 0.0)
	c.next(t, "method", "item/completed", "params.item.status", "declined")
	c.next(t, "method", "thread/tokenUsage/updated", "params.tokenUsage.total.totalTokens", 17960.0)
	c.next(t, "method", "account/rateLimits/updated", "params.rateLimits.primary.windowDurationMins", 300.0)
	c.next(t, "method", "turn/completed", "params.turn.id", firstTurn)
	userTurn(t, c, thread, "Approved. Retry.")
	const secondTurn = "01a1f63f-965e-7fc2-9513-6460299fd68d"
	c.next(t, "id", 42.0, "result.turn.id", secondTurn)
	c.next(t, "method", "turn/started", "params.turn.id", secondTurn)
	c.next(t, "method", "item/started", "params.item.type", "commandExecution")
	c.next(t, "method", "item/commandExecution/requestApproval", "id", 1.0, "params.turnId", secondTurn)
	c.send(t, `{"id":1,"result":{"decision":"accept"}}`)
	c.next(t, "method", "serverRequest/resolved", "params.requestId", 1.0)
	c.next(t, "method", "item/completed", "params.item.status", "completed")
	c.next(t, "method", "turn/completed", "params.turn.id", secondTurn)
	c.stdin.Close()
	c.end(t, 0)
}

func resume(t *testing.T) {
	c := start(t, "codex", "testdata/codex-resume.json", "tok-resumed", "app-server")
	initialize(t, c)
	c.send(t, `{"method":"thread/resume","id":24,"params":{"threadId":"`+thread+`"}}`)
	m := c.mcp(t, "2025-06-18")
	c.next(t, "id", 24.0, "result.thread.id", thread)
	userTurn(t, c, thread, "The owner decided after exit. Check the token.")
	c.next(t, "id", 42.0, "result.turn.id", resumedTurn)
	c.next(t, "method", "turn/started", "params.turn.id", resumedTurn)
	c.next(t, "method", "item/started", "params.item.type", "mcpToolCall")
	m.next(t, "id", 2.0, "method", "tools/call", "params.name", "whoami", "params._meta.threadId", thread, "params._meta.x-codex-turn-metadata.turn_id", resumedTurn)
	m.result(t, 2, `{"token": "tok-resumed"}`)
	c.next(t, "method", "item/completed", "params.item.status", "completed")
	c.next(t, "method", "turn/completed", "params.turn.id", resumedTurn, "params.threadId", thread)
	c.stdin.Close()
	c.end(t, 0)
}

func exitDuringApproval(t *testing.T) {
	c := start(t, "codex", "testdata/codex-exit-mid-turn.json", "tok-first", "app-server")
	firstApproval(t, c)
	c.end(t, 17)
	if _, err := fmt.Fprintln(c.in, `{"id":0,"result":{"decision":"accept"}}`); err == nil {
		t.Fatal("exited process accepted a decision")
	}
	resume(t)
}

func lateReply(t *testing.T) {
	const sid = "01a1f63e-dcce-7a02-a6cc-fc0d4c34bc1c"
	const turn = "01a1f63e-dcec-7621-b000-b2f5ffc70b34"
	c := start(t, "codex", "testdata/codex-late-call.json", "tok-checks", "app-server")
	initialize(t, c)
	c.send(t, `{"method":"thread/start","id":3,"params":{}}`)
	c.next(t, "id", 3.0, "result.thread.id", sid)
	userTurn(t, c, sid, "Run checks.")
	c.next(t, "id", 42.0, "result.turn.id", turn)
	m := c.mcp(t, "2025-06-18")
	c.next(t, "method", "turn/started", "params.turn.id", turn)
	c.next(t, "method", "item/started", "params.item.tool", "run_checks")
	m.next(t, "id", 2.0, "params.name", "run_checks", "params._meta.threadId", sid)
	c.next(t, "method", "item/completed", "params.item.status", "failed", "params.item.durationMs", 300005.0)
	c.next(t, "method", "turn/completed", "params.turn.id", turn)
	m.result(t, 2, `{"ok": true, "commit": "0000000", "items": []}`)
	userTurn(t, c, sid, "Late reply: checks passed.")
	c.stdin.Close()
	c.end(t, 0)
}
