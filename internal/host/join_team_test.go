package host

import (
	"os"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestJoinTeamRefusesBeforeWorkspaceOrAdmission(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-1")
	group := makeGroupRow(joinTestGroup1, "home-1", "Team", "", "")
	group.Kind = "team"
	fh.putGroup(group)
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "AI"}), "conflict")
	joinAssertEqual(t, "sessions", len(rig.h.Sessions.All()), 0)
	fh.mu.Lock()
	joinAssertEqual(t, "AI admissions", len(fh.addAIs), 0)
	joinAssertEqual(t, "state-only syncs", len(fh.syncs), 1)
	joinAssertEqual(t, "state only", fh.syncs[0].StateOnly, true)
	fh.mu.Unlock()
	_, err := os.Stat(projectDir(rig.h, string(joinTestGroup1)))
	joinAssertEqual(t, "no workspace", os.IsNotExist(err), true)
}
