//go:build acceptance

package features

import (
	"fmt"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func (w *s3DriverWorld) m4() *m4World {
	m := w.caps.m3
	return &m4World{f: w.f, cleanup: w.cleanup, cluster: m.cluster, activeHost: m.activeHost, ids: m.ids, groupID: proto.ID(m.groups["Website"]),
		b1: &m4Client{key: m.cluster.Owner, frames: m.browser, conn: m.browserConn, nextReq: m.nextReq, homeView: m.homeView}}
}

func (w *s3DriverWorld) pause() error {
	m, m4 := w.caps.m3, w.m4()
	return s3Run(func() error {
		return w.script(false, false, s3Turn{body: mcpPostSteps(1, "Working."), interrupt: true}, s3PostTurn(2, "Held mentions answered."))
	},
		func() error { return m4.postOwnerMention("Worker", "@Worker begin analysis") }, func() error { return m4.awaitAIPost("Worker", "Working.", 10*time.Second) },
		m4.whenOwnerPausesGroup,
		func() error { return m4.thenTurnInterruptedWithinTwoSeconds("Worker", "stop", "group_paused") },
		func() error { return m4.postOwnerMention("Worker", "@Worker first queued note") },
		func() error { return m4.postOwnerMention("Worker", "@Worker second queued note") },
		func() error { return awaitHomeInboxCount(m.node().Store, 2) }, m4.whenOwnerResumesGroup,
		func() error {
			return m4.thenResumedGroupDeliversMentions("Worker", "Held mentions answered.", "group_resumed")
		})
}

func (w *s3DriverWorld) hostSwitch(name string) error {
	return w.caps.send(name, w.caps.header(name))
}

func (w *s3DriverWorld) offline() error {
	m := w.caps.m3
	return s3Run(func() error { return w.script(false, true, s3PostTurn(1, "Waiting.")) }, w.mention,
		func() error { return w.posted("Waiting.") }, func() error { return w.hostSwitch("host.offline") },
		func() error { return m.thenAIHasState("Worker", "offline", "Website") },
		func() error { return w.script(true, true, s3PostTurn(1, "Online again.")) }, w.mention,
		func() error { return awaitHomeInboxCount(m.node().Store, 1) },
		func() error {
			m.node().StopHome()
			return w.hostSwitch("host.offline")
		},
		func() error {
			m.node().RestartHome()
			m.attachBrowser()
			return m.syncBrowserGroup(m.groups["Website"])
		},
		func() error { return w.hostSwitch("host.online") },
		func() error { return w.posted("Online again.") },
		func() error { return awaitHomeInboxCount(m.node().Store, 0) },
		w.offlineWhileStopped)
}

// offlineWhileStopped goes on from a waiting session whose process is open:
// the home stops first, then the owner sends host.offline.
func (w *s3DriverWorld) offlineWhileStopped() error {
	m := w.caps.m3
	return s3Run(func() error { return w.script(true, true, s3PostTurn(1, "Online once more.")) },
		func() error {
			m.node().StopHome()
			return w.hostSwitch("host.offline")
		},
		func() error { return sessionsDormant(m.activeHost) },
		func() error {
			m.node().RestartHome()
			m.attachBrowser()
			return m.syncBrowserGroup(m.groups["Website"])
		},
		func() error { return m.thenAIHasState("Worker", "offline", "Website") }, w.mention,
		func() error { return awaitHomeInboxCount(m.node().Store, 1) },
		func() error { return w.hostSwitch("host.online") },
		func() error { return w.posted("Online once more.") },
		func() error { return awaitHomeInboxCount(m.node().Store, 0) })
}

func (w *s3DriverWorld) timeout() error {
	m := w.caps.m3
	m4 := w.m4()
	var taskID string
	grp := m.group
	grp.Limits.TurnMinutes = 1
	grp.Limits.Failures = 1
	set := proto.GroupSet{GroupCommand: proto.GroupCommand{HomeCommand: proto.HomeCommand{Type: "group", Nonce: m.ids.next(), Timestamp: time.Now().UnixMilli(), Group: proto.ID(m.groups["Website"])}, Op: "set"}, Limits: &grp.Limits}
	return s3Run(func() error { return m.sendBrowserCmd(set) },
		func() error {
			return w.script(false, true, s3Turn{body: mcpPostSteps(1, "Long turn."), interrupt: true})
		},
		func() error {
			var err error
			taskID, err = m4.startTaskFor("Worker", "Long task", "Exceed the turn limit")
			return err
		},
		func() error { return m.thenAIPosts("Worker", "Long turn.", "Website") },
		func() error {
			return m.drainBrowserUntil(75*time.Second, func() bool {
				return viewHasInterruptedMsg(&m.homeView, m.aiByName["Worker"].ID, "timeout") && m.tasks[taskID].State == "failed"
			}, nil)
		})
}

func (w *s3DriverWorld) readOnly() error {
	a := newApproveAccWorld(w.cleanup)
	return s3Run(w.readOnlyEdit,
		func() error { return a.setupHost(w.agent, "task", 60) },
		func() error { return a.callSyncBash("touch marker", "", 1) },
		func() error { return a.assertDenyResult("Denied: this session is read only.") },
		func() error {
			if n := a.fakeHome.totalPostCount(); n != 0 {
				return fmt.Errorf("cards = %d, want 0", n)
			}
			return nil
		})
}

func (w *s3DriverWorld) readOnlyAsked() error {
	e := newEditWorld(w.caps.m3, w.agent)
	return s3Run(func() error { return useEditCLIs(w.f, w.cleanup) }, e.askReadOnly, e.unchanged)
}

// readOnlyEdit runs the member with a CLI that edits a file on every turn.
func (w *s3DriverWorld) readOnlyEdit() error {
	e := newEditWorld(w.caps.m3, w.agent)
	return s3Run(func() error { return useEditCLIs(w.f, w.cleanup) },
		e.tryReadOnly, e.unchanged, e.tryLevel3, e.changed)
}

func (w *s3DriverWorld) levelThree() error {
	m := w.caps.m3
	m4 := w.m4()
	saved := host.Drivers[w.agent]
	driver := &choiceDriver{Driver: saved}
	host.Drivers[w.agent] = driver
	w.cleanup(func() { host.Drivers[w.agent] = saved })
	return s3Run(func() error { return w.script(false, true, s3PostTurn(1, "Level three finished.")) },
		func() error {
			_, err := m4.startTaskFor("Worker", "Level three task", "Finish without asking permission")
			return err
		},
		func() error { return w.posted("Level three finished.") },
		func() error {
			launches := driver.snapshot()
			if len(launches) != 1 {
				return fmt.Errorf("launches = %+v, want one", launches)
			}
			if launches[0].Level != 3 {
				return fmt.Errorf("level = %d, want 3", launches[0].Level)
			}
			return nil
		},
		func() error {
			for _, row := range m.msgs {
				if row.Kind == "approval" {
					return fmt.Errorf("unexpected approval %s", row.ID)
				}
			}
			return nil
		})
}
