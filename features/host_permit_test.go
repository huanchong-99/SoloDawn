//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_permit"] = hostPermitSteps
}

func hostPermitSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	jw := &joinScenarioWorld{}
	var (
		lastErr    error
		lastFolder string

		// A11.6 session state
		db         *store.Store
		h          *host.Host
		rh         *hrHome
		drv        *hrMemDriver
		sessFolder string
		taskKey    host.SessionKey
	)

	sc.Step(`^a paired host for permit checks with declared agent "([^"]+)" and remote group "([^"]+)" on "([^"]+)"$`, func(agent, group, remote string) error {
		if err := jw.initHostOnDisk(agent, "codex", remote, cleanup); err != nil {
			return err
		}
		jw.homes[remote] = newJoinScenarioHome(jw.h)
		return jw.createRemoteGroup(remote, group)
	})
	sc.Step(`^macOS refuses access to folder "([^"]+)"$`, func(string) {
		jw.h.Access = func(folder string) error {
			return &host.Error{Code: "permission", Message: "macOS does not allow SoloDawn to use " + folder}
		}
	})
	sc.Step(`^macOS does not answer within 2 seconds for folder "([^"]+)"$`, func(string) {
		jw.h.Access = func(folder string) error {
			return &host.Error{Code: "permission", Message: "macOS is waiting for an answer about " + folder + " on this computer"}
		}
	})
	sc.Step(`^macOS allows access to folder "([^"]+)"$`, func(string) {
		jw.h.Access = func(string) error { return nil }
	})
	sc.Step(`^the owner tries to join "([^"]+)" as "([^"]+)" to "([^"]+)" on "([^"]+)" in folder "([^"]+)"$`, func(agent, name, group, home, dir string) error {
		var err error
		lastFolder, err = aiWorkspace(jw.root, joinGroupIDs[group], dir)
		if err != nil {
			return err
		}
		lastErr = jw.joinIn(agent, name, false, group, home, lastFolder)
		return nil
	})
	sc.Step(`^the permit join fails with code "([^"]+)" naming folder "([^"]+)" and adds no member$`, func(code, _ string) error {
		var wireErr *host.Error
		if !errors.As(lastErr, &wireErr) || wireErr.Code != code || !strings.Contains(wireErr.Message, lastFolder) {
			return fmt.Errorf("got err=%v, want code %q naming %q", lastErr, code, lastFolder)
		}
		if got := len(jw.h.Sessions.All()); got != 0 {
			return fmt.Errorf("sessions = %d, want 0", got)
		}
		return nil
	})
	sc.Step(`^the permit join fails with code "([^"]+)" saying macOS is waiting and adds no member$`, func(code string) error {
		var wireErr *host.Error
		if !errors.As(lastErr, &wireErr) || wireErr.Code != code || !strings.Contains(wireErr.Message, "waiting for an answer about "+lastFolder) {
			return fmt.Errorf("got err=%v, want code %q waiting for %q", lastErr, code, lastFolder)
		}
		if got := len(jw.h.Sessions.All()); got != 0 {
			return fmt.Errorf("sessions = %d, want 0", got)
		}
		return nil
	})
	sc.Step(`^the permit join succeeds with (\d+) session in folder "([^"]+)"$`, func(count int, _ string) error {
		if lastErr != nil {
			return fmt.Errorf("join failed: %v", lastErr)
		}
		got := jw.h.Sessions.All()
		if len(got) != count || got[0].Folder != lastFolder {
			return fmt.Errorf("sessions = %+v, want %d in %q", got, count, lastFolder)
		}
		return nil
	})

	// A11.6 steps
	sc.Step(`^a task session in folder "([^"]+)" on a host whose folder access macOS refuses$`, func(string) error {
		var err error
		db, err = store.Open(":memory:")
		if err != nil {
			return err
		}
		cleanup(func() { db.Close() })
		h, _, err = host.New(db, identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))), time.Now)
		if err != nil {
			return err
		}
		cleanup(h.Settle)
		sessFolder, err = filepath.EvalSymlinks(f.t.TempDir())
		if err != nil {
			return err
		}
		h.Access = func(folder string) error {
			return &host.Error{Code: "permission", Message: "macOS does not allow SoloDawn to use " + folder}
		}
		rh = newHRHome()
		h.Attach("home-permit", rh)

		origDrivers, origCompose, origInst := host.Drivers, host.Compose, host.Instructions
		cleanup(func() {
			host.Drivers = origDrivers
			host.Compose = origCompose
			host.Instructions = origInst
		})
		drv = &hrMemDriver{autoEnd: true}
		host.Drivers = map[string]host.Driver{"claude": drv}
		host.Compose = func(_ *host.Host, _ host.Session, kind host.TurnKind, _ []json.RawMessage) (string, host.Turn, error) {
			return "go", host.Turn{Kind: kind}, nil
		}
		host.Instructions = func(host.Session) string { return "inst" }

		taskKey = host.SessionKey{Group: "rZNvy-1jH6Z-BcPqA5U5BQ", Member: "1PC8WineBrUQ-apCjx7tug", Task: "DrtCn6htSBwmMPrFPbHJHA"}
		return h.Sessions.Create(host.Session{
			SessionKey: taskKey, Kind: host.TaskSession, Folder: sessFolder, Agent: "claude", Home: "home-permit", Name: "Claude",
		})
	})
	sc.Step(`^that task session receives a turn$`, func() error {
		lastErr = host.Runner.Turn(context.Background(), h, taskKey, "go", host.Turn{Kind: host.ItemsTurn, Thread: "DrtCn6htSBwmMPrFPbHJHA"})
		return nil
	})
	sc.Step(`^no CLI started and the host reports turn_interrupted "([^"]+)" naming the folder in detail and task fail "([^"]+)"$`, func(wantReason, wantFail string) error {
		if lastErr == nil || !strings.Contains(lastErr.Error(), sessFolder) {
			return fmt.Errorf("turn err = %v, want folder %q", lastErr, sessFolder)
		}
		if starts := len(drv.launched()); starts != 0 {
			return fmt.Errorf("CLI starts = %d, want 0", starts)
		}
		rh.mu.Lock()
		cmds := slices.Clone(rh.rawCmds)
		rh.mu.Unlock()
		if len(cmds) != 2 {
			return fmt.Errorf("home commands = %d (%v), want 2", len(cmds), cmds)
		}
		var st proto.MemberStatus[proto.TurnInterruptedEvent]
		if err := proto.Decode([]byte(cmds[0]), &st); err != nil || st.Event == nil || st.Event.Reason != wantReason || !strings.Contains(st.Event.Detail, sessFolder) {
			return fmt.Errorf("turn_interrupted = %+v (%v), want reason %q and folder %q", st, err, wantReason, sessFolder)
		}
		var fail proto.TaskFail
		if err := proto.Decode([]byte(cmds[1]), &fail); err != nil || fail.Op != "fail" || fail.Reason != wantFail || !strings.Contains(fail.Detail, sessFolder) {
			return fmt.Errorf("task fail = %+v (%v), want reason %q and folder %q", fail, err, wantFail, sessFolder)
		}
		return nil
	})
	sc.Step(`^macOS allows access to that task folder and the session receives a turn$`, func() error {
		h.Access = func(string) error { return nil }
		return host.Runner.Turn(context.Background(), h, taskKey, "go", host.Turn{Kind: host.ItemsTurn, Thread: "DrtCn6htSBwmMPrFPbHJHA"})
	})
	sc.Step(`^the CLI started once in that folder$`, func() error {
		launches := drv.launched()
		if len(launches) != 1 || launches[0].Folder != sessFolder {
			return fmt.Errorf("launches = %+v, want 1 in %q", launches, sessFolder)
		}
		return nil
	})
}
