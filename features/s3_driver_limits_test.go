//go:build acceptance

package features

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
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
		func() error { return w.waitInitialTimeoutPost(taskID) },
		func() error {
			return m.drainBrowserUntil(75*time.Second, func() bool {
				return viewHasInterruptedMsg(&m.homeView, m.aiByName["Worker"].ID, "timeout") && m.tasks[taskID].State == "failed"
			}, nil)
		})
}

type s3MCPInput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (p *s3MCPInput) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Write(b)
}

func (p *s3MCPInput) receivedPost(text string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for line := range bytes.SplitSeq(p.buf.Bytes(), []byte{'\n'}) {
		if bytes.Contains(line, []byte(`"name":"post"`)) && bytes.Contains(line, []byte(text)) {
			return true
		}
	}
	return false
}

func (w *s3DriverWorld) waitInitialTimeoutPost(taskID string) error {
	m := w.caps.m3
	const text = "Long turn."
	err := m.thenAIPosts("Worker", text, "Website")
	if err == nil {
		return nil
	}
	received := w.mcp.receivedPost(text)
	log, logErr := os.ReadFile(m.cluster.LogPath)
	gid := m.groups["Website"]
	var member, task string
	var posts int
	readErr := m.node().Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT
			coalesce((SELECT data->>'state' FROM row WHERE g=? AND r='member' AND id=?), ''),
			coalesce((SELECT data->>'state' FROM row WHERE g=? AND r='task' AND id=?), ''),
			(SELECT count(*) FROM row WHERE g=? AND r='msg' AND gone=0 AND data->>'by'=? AND data->>'text'=?)
		`, gid, m.aiByName["Worker"].ID, gid, taskID, gid, m.aiByName["Worker"].ID, text).Scan(&member, &task, &posts)
	})
	return fmt.Errorf("%w; host MCP receipt=%t; home member=%q task=%q posts=%d db=%v; fake CLI log err=%v:\n%s",
		err, received, member, task, posts, readErr, logErr, log)
}

func (w *s3DriverWorld) readOnly() error {
	a := newApproveAccWorld(w.cleanup)
	return s3Run(func() error { return a.setupHost(w.agent, "task", 60) },
		func() error { return a.callSyncBash("touch marker", "", 1) },
		func() error { return a.assertDenyResult("Denied: this session is read only.") },
		func() error {
			if n := a.fakeHome.totalPostCount(); n != 0 {
				return fmt.Errorf("cards = %d, want 0", n)
			}
			return nil
		})
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
