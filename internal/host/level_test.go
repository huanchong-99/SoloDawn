package host

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	lvlGroup1 = "rZNvy-1jH6Z-BcPqA5U5BQ"
	lvlGroup2 = "AAAAAAAAAAAAAAAAAAAAAQ"
	lvlNonce  = "btrr6I_yrHUCko3ynHVPlw"
)

func lvlPayload(t *testing.T, h *Host, agent, group string, level int, repoConfig bool) json.RawMessage {
	t.Helper()
	cmd := proto.PermSet{
		HostCommand: proto.HostCommand{
			Type:      "perm.set",
			Nonce:     proto.ID(lvlNonce),
			Timestamp: h.Now().UnixMilli(),
			To:        h.Key.ID(),
		},
		Agent:      agent,
		Group:      proto.ID(group),
		Level:      level,
		RepoConfig: repoConfig,
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func lvlPermCount(t *testing.T, h *Host) int {
	t.Helper()
	var n int
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM perm").Scan(&n)
	}
	if err := h.Store.Write(read); err != nil {
		t.Fatal(err)
	}
	return n
}

func lvlAssertPerm(t *testing.T, h *Host, agent, group string, wantLevel int, wantRepo bool) {
	t.Helper()
	gotLevel, gotRepo, err := h.EffectivePerm(agent, group)
	if err != nil || gotLevel != wantLevel || gotRepo != wantRepo {
		t.Fatalf("EffectivePerm(%q, %q) = (%d, %t, %v), want (%d, %t, nil)",
			agent, group, gotLevel, gotRepo, err, wantLevel, wantRepo)
	}
}

func TestDefaultLevelAndEffectivePermPrecedence(t *testing.T) {
	if DefaultLevel != 3 {
		t.Fatalf("DefaultLevel = %d, want 3", DefaultLevel)
	}
	h, _ := seamHost(t, seamStore(t))
	lvlAssertPerm(t, h, "claude", lvlGroup1, 3, false)
	lvlAssertPerm(t, h, "codex", lvlGroup1, 3, false)

	seamSQL(t, h.Store, `INSERT INTO setting(key, value) VALUES('default_level', '2')`)
	lvlAssertPerm(t, h, "claude", lvlGroup1, 2, false)
	seamSQL(t, h.Store, `INSERT OR REPLACE INTO setting(key, value) VALUES('default_level', '1')`)
	lvlAssertPerm(t, h, "claude", lvlGroup1, 1, false)

	// all_level3 overrides default_level when no perm row exists.
	seamSQL(t, h.Store, `INSERT INTO setting(key, value) VALUES('all_level3', 'true')`)
	lvlAssertPerm(t, h, "claude", lvlGroup1, 3, false)
	seamSQL(t, h.Store, `INSERT OR REPLACE INTO setting(key, value) VALUES('all_level3', 'false')`)
	lvlAssertPerm(t, h, "claude", lvlGroup1, 1, false)

	// A perm row overrides default_level for its own (agent, g) only.
	seamSQL(t, h.Store, `INSERT OR REPLACE INTO setting(key, value) VALUES('default_level', '2')`)
	if err := lvlSetPerm(h, "owner-dev", lvlPayload(t, h, "claude", lvlGroup1, 1, true), "linux"); err != nil {
		t.Fatal(err)
	}
	lvlAssertPerm(t, h, "claude", lvlGroup1, 1, true)
	lvlAssertPerm(t, h, "codex", lvlGroup1, 2, false)
	lvlAssertPerm(t, h, "claude", lvlGroup2, 2, false)

	// all_level3 forces level 3 while keeping the stored repo_config flag.
	seamSQL(t, h.Store, `INSERT OR REPLACE INTO setting(key, value) VALUES('all_level3', 'true')`)
	lvlAssertPerm(t, h, "claude", lvlGroup1, 3, true)
	lvlAssertPerm(t, h, "codex", lvlGroup1, 3, false)
}

func TestPermSetSurfaceAndAudit(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	golden, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "perm-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	ans, err := Surface["perm.set"](t.Context(), h, nil, "owner-browser", golden)
	if err != nil || ans != nil {
		t.Fatalf("Surface perm.set = (%v, %v), want (nil, nil)", ans, err)
	}
	lvlAssertPerm(t, h, "claude", lvlGroup1, 3, true)

	// Updating the same (agent, g) replaces the row rather than duplicating it.
	ans, err = Surface["perm.set"](t.Context(), h, nil, "owner-laptop", lvlPayload(t, h, "claude", lvlGroup1, 1, false))
	if err != nil || ans != nil {
		t.Fatalf("Surface perm.set update = (%v, %v), want (nil, nil)", ans, err)
	}
	lvlAssertPerm(t, h, "claude", lvlGroup1, 1, false)
	if got := lvlPermCount(t, h); got != 1 {
		t.Fatalf("perm count = %d, want 1", got)
	}

	// Codex accepts level 2 on every operating system.
	ans, err = Surface["perm.set"](t.Context(), h, nil, "owner-laptop", lvlPayload(t, h, "codex", lvlGroup1, 2, true))
	if err != nil || ans != nil {
		t.Fatalf("Surface perm.set codex = (%v, %v), want (nil, nil)", ans, err)
	}
	lvlAssertPerm(t, h, "codex", lvlGroup1, 2, true)
	if got := lvlPermCount(t, h); got != 2 {
		t.Fatalf("perm count = %d, want 2", got)
	}

	wantAudit := strings.Join([]string{
		"1763600000000|" + lvlGroup1 + "|claude|setting|-|-|level 3 repo_config true|owner-browser|-",
		"1763600002000|" + lvlGroup1 + "|claude|setting|-|-|level 1 repo_config false|owner-laptop|-",
		"1763600004000|" + lvlGroup1 + "|codex|setting|-|-|level 2 repo_config true|owner-laptop|-",
	}, "\n")
	if got := seamRecord(t, h); got != wantAudit {
		t.Fatalf("audit =\n%s\nwant\n%s", got, wantAudit)
	}
}

func TestPermSetWindowsClaudeLevel2Conflict(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))

	err := lvlSetPerm(h, "owner-dev", lvlPayload(t, h, "claude", lvlGroup1, 2, false), "windows")
	if err == nil || err.Error() != "conflict: Claude Code has no sandbox on Windows; choose level 1 or 3" {
		t.Fatalf("got %v, want Windows Claude level-2 conflict", err)
	}
	seamCode(t, err, "conflict")
	if lvlPermCount(t, h) != 0 || seamRecord(t, h) != "" {
		t.Fatal("refused perm.set wrote to store or audit")
	}

	// Flip each operand of goos == "windows" && agent == "claude" && level == 2.
	allowed := []struct {
		goos  string
		agent string
		group string
		level int
		repo  bool
	}{
		{"darwin", "claude", lvlGroup1, 2, false},
		{"linux", "claude", lvlGroup2, 2, true},
		{"windows", "codex", lvlGroup1, 2, false},
		{"windows", "claude", lvlGroup1, 1, false},
		{"windows", "claude", lvlGroup1, 3, true},
	}
	for _, tc := range allowed {
		if err := lvlSetPerm(h, "owner-dev", lvlPayload(t, h, tc.agent, tc.group, tc.level, tc.repo), tc.goos); err != nil {
			t.Fatalf("%+v failed: %v", tc, err)
		}
		lvlAssertPerm(t, h, tc.agent, tc.group, tc.level, tc.repo)
	}
}

func TestPermSetBadRequestRefusals(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	bad := []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"t":"perm.set"}`),
		json.RawMessage(`{"t":"perm.set","n":"` + lvlNonce + `","ts":1,"to":"x","agent":"claude","g":"bad-id","level":3,"repo_config":false}`),
		json.RawMessage(`{"t":"perm.set","n":"` + lvlNonce + `","ts":1,"to":"x","agent":"claude","g":"","level":3,"repo_config":false}`),
		lvlPayload(t, h, "gemini", lvlGroup1, 3, false),
		lvlPayload(t, h, "", lvlGroup1, 3, false),
		lvlPayload(t, h, "Claude", lvlGroup1, 3, false),
		lvlPayload(t, h, "claude", lvlGroup1, -1, false),
		lvlPayload(t, h, "claude", lvlGroup1, 0, false),
		lvlPayload(t, h, "claude", lvlGroup1, 4, false),
	}
	for _, raw := range bad {
		ans, err := Surface["perm.set"](t.Context(), h, nil, "owner-dev", raw)
		if ans != nil {
			t.Fatalf("got answer %v for %s", ans, raw)
		}
		seamCode(t, err, "bad_request")
	}
	if lvlPermCount(t, h) != 0 || seamRecord(t, h) != "" {
		t.Fatal("bad_request wrote to perm or audit")
	}
}

func TestPermSetAndEffectivePermStoreErrors(t *testing.T) {
	// Dropping audit rolls back the perm insert in the same transaction.
	h, _ := seamHost(t, seamStore(t))
	seamSQL(t, h.Store, "DROP TABLE audit")
	_, err := Surface["perm.set"](t.Context(), h, nil, "owner-dev", lvlPayload(t, h, "claude", lvlGroup1, 1, true))
	if err == nil || !strings.Contains(err.Error(), "no such table: audit") {
		t.Fatalf("got %v, want missing audit table error", err)
	}
	if got := lvlPermCount(t, h); got != 0 {
		t.Fatalf("perm row survived failed audit write: %d", got)
	}

	// Dropping perm fails both perm.set and EffectivePerm.
	h2, _ := seamHost(t, seamStore(t))
	seamSQL(t, h2.Store, "DROP TABLE perm")
	_, err = Surface["perm.set"](t.Context(), h2, nil, "owner-dev", lvlPayload(t, h2, "claude", lvlGroup1, 3, false))
	if err == nil || !strings.Contains(err.Error(), "no such table: perm") {
		t.Fatalf("got %v, want missing perm table error", err)
	}
	level, repo, err := h2.EffectivePerm("claude", lvlGroup1)
	if err == nil || !strings.Contains(err.Error(), "no such table: perm") || level != 0 || repo {
		t.Fatalf("EffectivePerm on broken store = (%d, %t, %v)", level, repo, err)
	}
}

func TestTaskToolsAllowedAndToolsFor(t *testing.T) {
	for _, tc := range []struct {
		level int
		want  bool
	}{
		{-1, false},
		{0, false},
		{1, false},
		{2, true},
		{3, true},
	} {
		if got := TaskToolsAllowed(tc.level); got != tc.want {
			t.Fatalf("TaskToolsAllowed(%d) = %t, want %t", tc.level, got, tc.want)
		}
	}

	before := maps.Clone(Tools)
	t.Cleanup(func() { Tools = before })
	forTask := func(c Caller) bool { return c.Kind == TaskSession && TaskToolsAllowed(c.Level) }
	Tools = map[string]Tool{
		"post":          {Description: "every session"},
		"start_task":    {Description: "working group", For: func(c Caller) bool { return c.Kind == GroupSession && !c.Secretary }},
		"submit_review": {Description: "review", For: func(c Caller) bool { return c.Kind == ReviewSession }},
		"propose_brief": {Description: "secretary", For: func(c Caller) bool { return c.Secretary }},
		"push_branch":   {Description: "task > 1", For: forTask},
		"run_checks":    {Description: "task > 1", For: forTask},
		"submit_task":   {Description: "task > 1", For: forTask},
	}

	cases := []struct {
		caller Caller
		want   []string
	}{
		{Caller{Session: Session{Kind: TaskSession}, Level: 1}, []string{"post"}},
		{Caller{Session: Session{Kind: TaskSession}, Level: 2}, []string{"post", "push_branch", "run_checks", "submit_task"}},
		{Caller{Session: Session{Kind: TaskSession}, Level: DefaultLevel}, []string{"post", "push_branch", "run_checks", "submit_task"}},
		{Caller{Session: Session{Kind: GroupSession}, Level: DefaultLevel}, []string{"post", "start_task"}},
		{Caller{Session: Session{Kind: GroupSession, Secretary: true}, Level: DefaultLevel}, []string{"post", "propose_brief"}},
		{Caller{Session: Session{Kind: ReviewSession}, Level: DefaultLevel}, []string{"post", "submit_review"}},
	}
	for _, tc := range cases {
		got := slices.Sorted(maps.Keys(ToolsFor(tc.caller)))
		if !slices.Equal(got, tc.want) {
			t.Fatalf("ToolsFor(%+v) = %v, want %v", tc.caller, got, tc.want)
		}
	}
}
