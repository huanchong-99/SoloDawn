package host

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func (c *turnCtx) takesPart(thread string) bool {
	task, _ := c.lookupTask(thread)
	return task.Assignee == c.s.Member || task.Reviewer == c.s.Member
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

func (c *turnCtx) wordEnough(u string) bool {
	if u == "" {
		return false
	}
	if u == c.owner {
		return true
	}
	raw, _ := c.h.Rows.Get(c.s.Group, "member", c.s.Member)
	var ai proto.AIMemberRow
	proto.Decode(raw, &ai)
	return (ai.Account == "team" || ai.Account == "api_key") && slices.Contains(ai.Allow, u)
}

func (c *turnCtx) mentionInstruct(msg *proto.MsgRow, thread string) bool {
	if msg == nil {
		return false
	}
	if c.takesPart(thread) {
		return c.isOwnerMember(msg.By)
	}
	return c.humanUser(msg.By) != ""
}

func (c *turnCtx) msgInstruct(msg proto.MsgRow) (bool, error) {
	u := c.humanUser(msg.By)
	if u == "" {
		return false, nil
	}
	if c.takesPart(strVal(msg.Thread)) {
		return u == c.owner, nil
	}
	if c.wordEnough(u) {
		return true, nil
	}
	return c.confirmedDirect(msg.ID)
}

func (c *turnCtx) confirmedDirect(id string) (bool, error) {
	if c.hasDirect(id) {
		return true, nil
	}
	if !c.confirmsFetched {
		c.confirmsFetched = true
		build := func(n int64) any {
			return proto.FetchKind{FetchPage: c.fetchPage(n, 0, 100), Kind: "confirm"}
		}
		fetched, err := fetchFrom(context.Background(), c.h, c.s.Home, build)
		if err != nil {
			return false, err
		}
		if err := c.readConfirmations(fetched.Rows); err != nil {
			return false, err
		}
	}
	return c.hasDirect(id), nil
}

func (c *turnCtx) readConfirmations(rows []json.RawMessage) error {
	for _, raw := range rows {
		var msg proto.MsgRow
		if proto.Decode(raw, &msg) != nil {
			return &Error{"bad_request", "invalid confirmation message"}
		}
		c.confirmations = append(c.confirmations, msg)
	}
	return nil
}

func (c *turnCtx) hasDirect(id string) bool {
	for _, msg := range c.confirmations {
		if c.coversDirect(msg, id) {
			return true
		}
	}
	return false
}

func (c *turnCtx) coversDirect(msg proto.MsgRow, id string) bool {
	if msg.Kind != "confirm" || strVal(msg.Re) != id || cardStatus(msg.State) != "confirmed" {
		return false
	}
	var body struct {
		What string `json:"what"`
		AI   string `json:"ai"`
	}
	proto.Decode(msg.Body, &body)
	return body.What == "direct" && body.AI == c.s.Member
}

func (c *turnCtx) decisionInstruct(msg proto.MsgRow) bool {
	by := c.humanUser(stateBy(msg.State))
	if c.wordEnough(by) {
		return true
	}
	if msg.Kind != "decision" || msg.By != c.s.Member {
		return false
	}
	var body proto.DecisionBody
	proto.Decode(msg.Body, &body)
	return slices.Contains(body.To, by)
}

func (c *turnCtx) formatCarriedMsg(msg proto.MsgRow) (string, error) {
	content, err := formatMsgContent(msg)
	if err != nil {
		return "", err
	}
	instruct, err := c.msgInstruct(msg)
	return fmt.Sprintf("-- %s · %s\n%s", c.fromIn(msg.By, strVal(msg.Thread)), authorityLabel(instruct), content), err
}

func (c *turnCtx) appendCarriedMsg(lines []string, msg *proto.MsgRow) (string, error) {
	if msg != nil {
		extra, err := c.formatCarriedMsg(*msg)
		if err != nil {
			return "", err
		}
		lines = append(lines, extra)
	}
	return strings.Join(lines, "\n"), nil
}

func (c *turnCtx) formatCarriedDecision(msg proto.MsgRow, content string) string {
	who := c.fromIn(stateBy(msg.State), strVal(msg.Thread))
	auth := authorityLabel(c.decisionInstruct(msg))
	return fmt.Sprintf("-- %s · %s\n%s", who, auth, content)
}
