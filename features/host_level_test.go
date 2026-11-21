//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_level"] = func(sc *godog.ScenarioContext, _ fixtures) {
		hlvlSteps(sc)
	}
}

const (
	hlvlGroup = "rZNvy-1jH6Z-BcPqA5U5BQ"
	hlvlNonce = "btrr6I_yrHUCko3ynHVPlw"
)

type hlvlRig struct {
	db     *store.Store
	h      *host.Host
	caller host.Caller
	last   error
}

func hlvlSteps(sc *godog.ScenarioContext) {
	cleanup := afterScenario(sc)
	r := &hlvlRig{}
	sc.Step(`^a fresh host for permission levels$`, func() error { return r.open(cleanup) })
	sc.Step(`^effective permission for "([^"]+)" in the project is level (\d+) with repo_config "(true|false)"$`, r.checkEffective)
	sc.Step(`^a "(task|group|review)" session admitted at its effective level is (offered|not offered) the task submission tools$`, r.admitEffectiveAndCheck)
	sc.Step(`^a "(task|group|review)" session is admitted at level (\d+)$`, r.admitAtLevel)
	sc.Step(`^the task submission tools are (offered|not offered) to that session$`, r.checkToolsOffered)
	sc.Step(`^owner device "([^"]+)" sends perm\.set for "([^"]*)" with level (-?\d+) and repo_config "(true|false)"$`, r.sendPermSet)
	sc.Step(`^perm\.set succeeds$`, r.checkOK)
	sc.Step(`^perm\.set is refused with "([^"]+)"$`, r.checkRefused)
	sc.Step(`^the last audit row records a setting for "([^"]+)" by "([^"]+)" with detail "([^"]+)"$`, r.checkLastAudit)
	sc.Step(`^host setting "([^"]+)" is "([^"]+)"$`, r.setSetting)
	sc.Step(`^the audit record holds (\d+) setting rows$`, r.checkAuditCount)
}

func (r *hlvlRig) open(cleanup func(func())) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, time.Now)
	if err != nil {
		return err
	}
	before := maps.Clone(host.Tools)
	cleanup(func() { host.Tools = before })
	forTask := func(c host.Caller) bool { return c.Kind == host.TaskSession && host.TaskToolsAllowed(c.Level) }
	host.Tools["post"] = host.Tool{Description: "every session"}
	host.Tools["push_branch"] = host.Tool{Description: "push branch", For: forTask}
	host.Tools["run_checks"] = host.Tool{Description: "run checks", For: forTask}
	host.Tools["submit_task"] = host.Tool{Description: "submit task", For: forTask}
	r.db, r.h, r.last = db, h, nil
	return nil
}

func (r *hlvlRig) checkEffective(agent string, wantLevel int, wantRepo string) error {
	level, repo, err := r.h.EffectivePerm(agent, hlvlGroup)
	if err != nil {
		return err
	}
	if level != wantLevel || repo != (wantRepo == "true") {
		return fmt.Errorf("got level %d repo_config %t, want %d %s", level, repo, wantLevel, wantRepo)
	}
	return nil
}

func (r *hlvlRig) admitAtLevel(kind string, level int) error {
	task := "t12"
	if kind == "group" {
		task = ""
	}
	key := host.SessionKey{Group: hlvlGroup, Member: "m7", Task: task}
	if _, exists := r.h.Sessions.Get(key); !exists {
		s := host.Session{SessionKey: key, Kind: host.Kind(kind), Folder: "repo", Agent: "claude", Home: "home-dev", Name: "Claude Code"}
		if err := r.h.Sessions.Create(s); err != nil {
			return err
		}
	}
	caller, ok := r.h.Sessions.Caller(r.h.Sessions.Admit(key, level, false))
	if !ok {
		return errors.New("admitted token did not resolve to a caller")
	}
	r.caller = caller
	return nil
}

func (r *hlvlRig) admitEffectiveAndCheck(kind, want string) error {
	level, _, err := r.h.EffectivePerm("claude", hlvlGroup)
	if err != nil {
		return err
	}
	if err := r.admitAtLevel(kind, level); err != nil {
		return err
	}
	return r.checkToolsOffered(want)
}

func (r *hlvlRig) checkToolsOffered(want string) error {
	listed := host.ToolsFor(r.caller)
	wantListed := want == "offered"
	for _, name := range []string{"push_branch", "run_checks", "submit_task"} {
		if _, ok := listed[name]; ok != wantListed {
			return fmt.Errorf("tool %s listed=%t, want %t", name, ok, wantListed)
		}
	}
	if _, ok := listed["post"]; !ok {
		return errors.New("base tool post was not listed")
	}
	return nil
}

func (r *hlvlRig) sendPermSet(by, agent string, level int, repo string) error {
	cmd := proto.PermSet{
		HostCommand: proto.HostCommand{
			Type:      "perm.set",
			Nonce:     hlvlNonce,
			Timestamp: r.h.Now().UnixMilli(),
			To:        r.h.Key.ID(),
		},
		Agent:      agent,
		Group:      hlvlGroup,
		Level:      level,
		RepoConfig: repo == "true",
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	ans, callErr := host.Surface["perm.set"](context.Background(), r.h, nil, by, raw)
	if ans != nil {
		return fmt.Errorf("unexpected perm.set result: %v", ans)
	}
	r.last = callErr
	return nil
}

func (r *hlvlRig) checkOK() error {
	return r.last
}

func (r *hlvlRig) checkRefused(wantCode string) error {
	var hostErr *host.Error
	if !errors.As(r.last, &hostErr) || hostErr.Code != wantCode {
		return fmt.Errorf("got %v, want code %s", r.last, wantCode)
	}
	return nil
}

func (r *hlvlRig) checkLastAudit(wantAgent, wantBy, wantDetail string) error {
	var g, agent, what, detail, by string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT coalesce(g, ''), coalesce(agent, ''), what, detail, coalesce(by, '')
			FROM audit ORDER BY id DESC LIMIT 1`).Scan(&g, &agent, &what, &detail, &by)
	}
	if err := r.db.Write(read); err != nil {
		return err
	}
	if g != hlvlGroup || agent != wantAgent || what != host.AuditSetting || detail != wantDetail || by != wantBy {
		return fmt.Errorf("audit = (%q, %q, %q, %q, %q), want (%q, %q, %q, %q, %q)",
			g, agent, what, detail, by, hlvlGroup, wantAgent, host.AuditSetting, wantDetail, wantBy)
	}
	return nil
}

func (r *hlvlRig) setSetting(key, val string) error {
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)", key, val)
		return err
	}
	return r.db.Write(write)
}

func (r *hlvlRig) checkAuditCount(want int) error {
	var got int
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM audit WHERE what = ?", host.AuditSetting).Scan(&got)
	}
	if err := r.db.Write(read); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("audit setting count = %d, want %d", got, want)
	}
	return nil
}
