//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["s2_watch"] = s2WatchSteps }

type s2WatchWorld struct {
	f       fixtures
	cleanup func(func())
	c       *s2Cast
}

func s2WatchSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s2WatchWorld{f: f, cleanup: afterScenario(sc)}
	team := rewritten("AC-3", "the team kind and bulletins are removed; A9.22 is a group without AIs, A9.23 goes (MODEL-1126 plan 3.3)")
	for _, step := range []string{
		`^P creates a team group on P2 without a repository$`,
		`^its group row has no repository$`,
		`^the team group refuses task start, brief posts, host\.join, member add_ai and check settings$`,
		`^no node holds a clone for the team group$`,
		`^the secretary's node knows the team group$`,
		`^the secretary posts a notice to the team group using the post tool$`,
		`^the team main line shows a bulletin naming the secretary and project$`,
		`^the working AI's call to the same tool is refused$`,
		`^P posts a team message naming the working AI$`,
		`^the team message creates no inbox item$`,
	} {
		sc.Step(step, team)
	}
	s2RegisterTerminal(sc, w)
	s2RegisterForge(sc, w)
}

func (w *s2WatchWorld) start(timeout time.Duration) error {
	c, err := startS2Cast(w.f.t, w.cleanup, w.f, s2CastConfig{Timeout: timeout})
	w.c = c
	return err
}

func s2WatchCmd(c *s2Cast, p *s2Person, conn *peer.Connection, fields map[string]any, want string) error {
	fields["ts"] = time.Now().UnixMilli()
	if _, ok := fields["n"]; !ok {
		fields["n"] = c.NextID()
	}
	if want == "" {
		_, err := c.SendCmd(p, conn, fields)
		return err
	}
	raw, err := sendSignedCmdRaw(c.Ctx, conn, p.Key, fields)
	if err != nil {
		return err
	}
	return s2WatchReply(raw, want)
}

func s2WatchReply(raw json.RawMessage, want string) error {
	if want == "" {
		if err := proto.Decode(raw, new(proto.Success)); err != nil {
			return fmt.Errorf("expected a successful reply, got %s: %w", raw, err)
		}
		return nil
	}
	return checkWireError(raw, want)
}
