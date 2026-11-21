package host

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// DefaultLevel is the permission level every AI session defaults to when
// neither perm nor default_level overrides it (owner's decision of
// 2026-11-21; contracts/drivers.md section 1, contracts/commands.md
// section 2).
const DefaultLevel = 3

const (
	lvlUpsertSQL = `INSERT OR REPLACE INTO perm(agent, g, level, repo_config) VALUES(?, ?, ?, ?)`

	lvlEffectiveSQL = `SELECT
		coalesce((SELECT json_extract(value, '$') FROM setting WHERE key = 'all_level3'), 0),
		coalesce(
			(SELECT level FROM perm WHERE agent = ? AND g = ?),
			(SELECT json_extract(value, '$') FROM setting WHERE key = 'default_level'),
			?
		),
		coalesce((SELECT repo_config FROM perm WHERE agent = ? AND g = ?), 0)`
)

func init() {
	Register(Surface, "perm.set", lvlPermSet)
}

// TaskToolsAllowed reports whether a task session at level is offered the
// node's task submission tools: push_branch, run_checks and submit_task
// (contracts/drivers.md section 2).
func TaskToolsAllowed(level int) bool {
	return level > 1
}

// EffectivePerm returns the effective permission level and repository
// configuration flag stored for agent in group (contracts/commands.md
// section 2, contracts/drivers.md section 1). Group sessions never load
// repository configuration even when repoConfig is true (contracts/drivers.md
// sections 1.1 and 1.2).
func (h *Host) EffectivePerm(agent, group string) (level int, repoConfig bool, err error) {
	var allLevel3 bool
	read := func(tx *store.Tx) error {
		return tx.QueryRow(lvlEffectiveSQL, agent, group, DefaultLevel, agent, group).Scan(&allLevel3, &level, &repoConfig)
	}
	if err := h.Store.Write(read); err != nil {
		return 0, false, err
	}
	if allLevel3 {
		level = DefaultLevel
	}
	return level, repoConfig, nil
}

func lvlPermSet(_ context.Context, h *Host, _ Owner, by string, data json.RawMessage) (any, error) {
	return nil, lvlSetPerm(h, by, data, runtime.GOOS)
}

func lvlSetPerm(h *Host, by string, data json.RawMessage, goos string) error {
	cmd, err := lvlDecodePermSet(data)
	if err != nil {
		return err
	}
	if goos == "windows" && cmd.Agent == "claude" && cmd.Level == 2 {
		return &Error{Code: "conflict", Message: "Claude Code has no sandbox on Windows; choose level 1 or 3"}
	}
	return lvlWritePerm(h, cmd, by)
}

func lvlDecodePermSet(data json.RawMessage) (proto.PermSet, error) {
	var cmd proto.PermSet
	if err := proto.Decode(data, &cmd); err != nil {
		return proto.PermSet{}, &Error{Code: "bad_request", Message: err.Error()}
	}
	if !lvlValidAgent(cmd.Agent) {
		return proto.PermSet{}, &Error{Code: "bad_request", Message: "unknown agent: " + cmd.Agent}
	}
	if cmd.Level < 1 || cmd.Level > 3 {
		return proto.PermSet{}, &Error{Code: "bad_request", Message: fmt.Sprintf("invalid level: %d", cmd.Level)}
	}
	return cmd, nil
}

func lvlValidAgent(agent string) bool {
	return agent == "claude" || agent == "codex"
}

func lvlWritePerm(h *Host, cmd proto.PermSet, by string) error {
	write := func(tx *store.Tx) error {
		if _, err := tx.Exec(lvlUpsertSQL, cmd.Agent, string(cmd.Group), cmd.Level, cmd.RepoConfig); err != nil {
			return err
		}
		_, err := h.Audit(tx, AuditRow{
			Group:  string(cmd.Group),
			Agent:  cmd.Agent,
			What:   AuditSetting,
			Detail: fmt.Sprintf("level %d repo_config %t", cmd.Level, cmd.RepoConfig),
			By:     by,
		})
		return err
	}
	return h.Store.Write(write)
}
