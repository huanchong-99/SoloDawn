//go:build !windows

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Making a symbolic link needs no privilege on a POSIX system. Two spellings
// of one folder are one workspace: the link is resolved before the folder is
// kept and before it is compared with those of other groups.
func TestHostJoinResolvesSymbolicLinks(t *testing.T) {
	rig := newJoinRig(t)
	rig.setAgent("claude", "2.1.311", joinStr("personal"))
	fh := rig.addHome("home-1")
	fh.putGroup(makeGroupRow(joinTestGroup1, "home-1", "Website"))
	fh.putGroup(makeGroupRow(joinTestGroup2, "home-1", "Backend"))
	real := rig.folder("website")
	link := filepath.Join(rig.root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	rig.mustJoin(proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Claude", Workspace: link})
	s, _ := rig.h.Sessions.Get(SessionKey{Group: string(joinTestGroup1), Member: string(fh.addAIs[0].ID)})
	joinAssertEqual(t, "add_ai workspace", fh.addAIs[0].Workspace, real)
	joinAssertEqual(t, "session folder", s.Folder, real)
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup2, Home: "home-1", Agent: "claude", Name: "Claude", Workspace: real}), "conflict")

	if err := os.Remove(real); err != nil {
		t.Fatal(err)
	}
	seamCode(t, rig.callJoin(t.Context(), proto.HostJoin{Group: joinTestGroup1, Home: "home-1", Agent: "claude", Name: "Again", Workspace: link}), "not_found")
}
