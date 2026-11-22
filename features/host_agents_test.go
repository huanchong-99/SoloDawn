//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_agents"] = hostAgentsSteps
}

func hostAgentsExe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func hostAgentsIsolatedPath(dir string) string {
	if runtime.GOOS == "windows" {
		if sysRoot := os.Getenv("SystemRoot"); sysRoot != "" {
			return dir + string(os.PathListSeparator) + filepath.Join(sysRoot, "System32")
		}
	}
	return dir
}

func hostAgentsLinkOrCopy(src, dst string) error {
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0700)
}

func hostAgentsSetBins(fakeCLIs, list string) (string, error) {
	dir, err := os.MkdirTemp("", "solodawn-agents-*")
	if err != nil {
		return "", err
	}
	for _, raw := range strings.Split(list, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		bin := hostAgentsExe(name)
		if err := hostAgentsLinkOrCopy(filepath.Join(fakeCLIs, bin), filepath.Join(dir, bin)); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

func hostAgentsSteps(sc *godog.ScenarioContext, f fixtures) {
	var (
		db      *store.Store
		h       *host.Host
		now     time.Time
		got     []proto.HostAgent
		setErr  error
		sessKey = host.SessionKey{Group: "g1", Member: "m1"}
	)
	cleanup := afterScenario(sc)

	useBins := func(list string) error {
		dir, err := hostAgentsSetBins(f.fakeCLIs, list)
		if err != nil {
			return err
		}
		cleanup(func() { os.RemoveAll(dir) })
		prev, had := os.LookupEnv("PATH")
		if err := os.Setenv("PATH", hostAgentsIsolatedPath(dir)); err != nil {
			return err
		}
		cleanup(func() { restoreVar("PATH", prev, had) })
		return nil
	}

	sc.Step(`^a host with fake CLIs "([^"]*)" on PATH$`, func(list string) error {
		if err := useBins(list); err != nil {
			return err
		}
		var err error
		db, err = store.Open(":memory:")
		if err != nil {
			return err
		}
		cleanup(func() { db.Close() })
		now = time.UnixMilli(1_763_600_000_000)
		key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
		h, _, err = host.New(db, key, func() time.Time { return now })
		return err
	})

	sc.Step(`^fake CLIs on PATH change to "([^"]*)" and (\d+) seconds pass$`, func(list string, sec int) error {
		if err := useBins(list); err != nil {
			return err
		}
		now = now.Add(time.Duration(sec) * time.Second)
		return nil
	})

	sc.Step(`^(\d+) more second passes$`, func(sec int) {
		now = now.Add(time.Duration(sec) * time.Second)
	})

	sc.Step(`^the host reads its agents$`, func() error {
		var err error
		got, err = h.Agents(context.Background(), false)
		return err
	})

	sc.Step(`^the host reports (\d+) agents$`, func(want int) error {
		if got == nil || len(got) != want {
			return fmt.Errorf("got %+v (nil=%v), want %d agents", got, got == nil, want)
		}
		return nil
	})

	sc.Step(`^the host reports agents:$`, func(table *godog.Table) error {
		rows := table.Rows[1:]
		if len(got) != len(rows) {
			return fmt.Errorf("got %d agents (%+v), want %d", len(got), got, len(rows))
		}
		for i, row := range rows {
			acct := "-"
			if got[i].Account != nil {
				acct = *got[i].Account
			}
			line := fmt.Sprintf("%s|%s|%s|%t", got[i].Agent, got[i].CLIVersion, acct, got[i].LoginOK)
			want := fmt.Sprintf("%s|%s|%s|%s", row.Cells[0].Value, row.Cells[1].Value, row.Cells[2].Value, row.Cells[3].Value)
			if line != want {
				return fmt.Errorf("row %d: got %s, want %s", i, line, want)
			}
		}
		return nil
	})

	sc.Step(`^owner device "([^"]+)" sends agent\.set for "([^"]*)" with account "([^"]*)"$`, func(by, agent, account string) {
		payload := fmt.Sprintf(`{"t":"agent.set","n":"STwCAvPRTFkRNCvXYEgsMw","ts":1763600000000,"to":"host-device","agent":%q,"account":%q}`, agent, account)
		_, setErr = host.Surface["agent.set"](context.Background(), h, nil, by, json.RawMessage(payload))
	})

	sc.Step(`^agent\.set ends with "([^"]*)"$`, func(want string) error {
		actual := "ok"
		if setErr != nil {
			actual = setErr.Error()
		}
		if actual != want {
			return fmt.Errorf("got %q, want %q", actual, want)
		}
		return nil
	})

	sc.Step(`^the setting audit rows are "([^"]*)"$`, func(want string) error {
		var actual string
		err := db.Write(func(tx *store.Tx) error {
			return tx.QueryRow(`SELECT coalesce(group_concat(concat_ws('|', coalesce(agent, '-'), detail, coalesce(by, '-')), ';'), '')
				FROM (SELECT agent, detail, by FROM audit WHERE what = 'setting' ORDER BY id)`).Scan(&actual)
		})
		if err != nil || actual != want {
			return fmt.Errorf("audit = %q (%v), want %q", actual, err, want)
		}
		return nil
	})

	sc.Step(`^a group session on that host for "([^"]+)"$`, func(agent string) error {
		return h.Sessions.Create(host.Session{
			SessionKey: sessKey,
			Kind:       host.GroupSession,
			Folder:     "repo",
			Agent:      agent,
			Home:       "home-1",
			Name:       "AI",
		})
	})

	sc.Step(`^that session reports "([^"]+)"$`, func(kind string) {
		h.Report(sessKey, host.Event{Kind: host.EventKind(kind)})
	})

	sc.Step(`^that session ends a turn with status "([^"]+)"$`, func(status string) {
		h.Report(sessKey, host.Event{Kind: host.TurnEnded, Status: status})
	})

	sc.Step(`^agent "([^"]+)" has login_ok "(true|false)"$`, func(agent, want string) error {
		for _, a := range got {
			if a.Agent == agent {
				if fmt.Sprintf("%t", a.LoginOK) != want {
					return fmt.Errorf("%s login_ok = %t, want %s", agent, a.LoginOK, want)
				}
				return nil
			}
		}
		return fmt.Errorf("agent %s not in %+v", agent, got)
	})
}
