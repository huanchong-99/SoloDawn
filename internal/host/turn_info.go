package host

import (
	"fmt"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func (c *turnCtx) inOwnTask(thread string) bool {
	return c.s.Task != "" && c.s.Task == thread
}

func (c *turnCtx) humanUser(id string) string {
	m, _ := c.lookupMember(id)
	if m.Kind != "human" {
		return ""
	}
	return m.User
}

func (c *turnCtx) isOwnerMember(id string) bool {
	u := c.humanUser(id)
	return u != "" && u == c.owner
}

func isAllowAccount(account string) bool {
	return account == "team" || account == "api_key"
}

func (c *turnCtx) wordEnough(id string) bool {
	u := c.humanUser(id)
	if u == "" {
		return false
	}
	if u == c.owner {
		return true
	}
	raw, _ := c.h.Rows.Get(c.s.Group, "member", c.s.Member)
	var ai proto.AIMemberRow
	proto.Decode(raw, &ai)
	return isAllowAccount(ai.Account) && slices.Contains(ai.Allow, u)
}

func (c *turnCtx) msgInstruct(msg *proto.MsgRow, thread string, directOK bool) bool {
	if msg == nil {
		return false
	}
	if c.inOwnTask(thread) {
		return c.isOwnerMember(msg.By)
	}
	if directOK {
		return c.humanUser(msg.By) != ""
	}
	return c.wordEnough(msg.By)
}

func (c *turnCtx) decidedItemInstruct(msg *proto.MsgRow, thread string) bool {
	return c.inOwnTask(thread) || (msg != nil && msg.By == c.s.Member)
}

func (c *turnCtx) decisionInstruct(msg proto.MsgRow) bool {
	by := stateBy(msg.State)
	if c.wordEnough(by) {
		return true
	}
	if msg.By != c.s.Member {
		return false
	}
	var body proto.DecisionBody
	proto.Decode(msg.Body, &body)
	return slices.Contains(body.To, c.humanUser(by))
}

func (c *turnCtx) carriedMsgWho(msg proto.MsgRow) string {
	thread := strVal(msg.Thread)
	if msg.By == "system" {
		return "from the home " + c.place(thread)
	}
	return c.fromIn(msg.By, thread)
}

func (c *turnCtx) formatCarriedMsg(msg proto.MsgRow, directOK bool) (string, error) {
	content, err := formatMsgContent(msg)
	if err != nil {
		return "", err
	}
	auth := authorityLabel(c.msgInstruct(&msg, strVal(msg.Thread), directOK))
	return fmt.Sprintf("-- %s · %s\n%s", c.carriedMsgWho(msg), auth, content), nil
}

func (c *turnCtx) formatOptionalCarriedMsg(msg *proto.MsgRow) (string, error) {
	if msg == nil {
		return "", nil
	}
	return c.formatCarriedMsg(*msg, false)
}

func (c *turnCtx) appendCarriedMsg(lines []string, msg *proto.MsgRow, directOK bool) (string, error) {
	if msg != nil && msg.Kind != "task" {
		extra, err := c.formatCarriedMsg(*msg, directOK)
		if err != nil {
			return "", err
		}
		lines = append(lines, extra)
	}
	return strings.Join(lines, "\n"), nil
}

func (c *turnCtx) formatCarriedDecision(msg proto.MsgRow, content string) string {
	who := "from " + c.stateActorLabel(msg.State) + " " + c.place(strVal(msg.Thread))
	auth := authorityLabel(c.decisionInstruct(msg))
	return fmt.Sprintf("-- %s · %s\n%s", who, auth, content)
}
