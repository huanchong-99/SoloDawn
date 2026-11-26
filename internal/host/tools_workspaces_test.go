package host

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host/workspaces"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func askWorkspaces(t *testing.T, h *Host, agent string) []proto.Workspace {
	t.Helper()
	res, err := h.Workspaces(t.Context(), json.RawMessage(marshalWire(proto.HostWorkspacesRequest{Request: proto.Request{Type: "host.workspaces", ID: 9}, Agent: agent})))
	if err != nil {
		t.Fatal(err)
	}
	return res.(proto.HostWorkspaces).Workspaces
}

// claudeSession writes a session file of Claude Code that names folder, used
// at the given time.
func claudeSession(t *testing.T, config, project, folder string, used time.Time) {
	t.Helper()
	dir := filepath.Join(config, "projects", project)
	file := filepath.Join(dir, "s.jsonl")
	codexNoErr(t, os.MkdirAll(dir, 0o700))
	codexNoErr(t, os.WriteFile(file, []byte(`{"type":"summary"}`+"\n"+marshalWire(map[string]string{"cwd": folder})+"\n"), 0o600))
	codexNoErr(t, os.Chtimes(file, used, used))
}

// The list holds each folder resolved as host.join resolves it, once, with
// its newest use, newest first; a folder that is gone, a relative one and one
// under the node's data directory are left out (contracts/local.md section 7).
func TestWorkspacesAreResolvedAndFiltered(t *testing.T) {
	data := t.TempDir()
	db, dataDir := openFileStore(t, data)
	t.Cleanup(func() { db.Close() })
	h, _ := seamHost(t, db)
	work, _ := filepath.EvalSymlinks(t.TempDir())
	site, notes := filepath.Join(work, "site"), filepath.Join(work, "notes")
	for _, dir := range []string{site, notes, filepath.Join(dataDir, "projects", "old")} {
		codexNoErr(t, os.MkdirAll(dir, 0o700))
	}
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	at := time.UnixMilli(1763600000000)
	claudeSession(t, config, "a", site+string(filepath.Separator)+".", at)
	claudeSession(t, config, "b", site, at.Add(-time.Hour))
	claudeSession(t, config, "c", notes, at.Add(-2*time.Hour))
	claudeSession(t, config, "d", filepath.Join(work, "gone"), at.Add(time.Hour))
	claudeSession(t, config, "e", "relative", at.Add(time.Hour))
	claudeSession(t, config, "f", filepath.Join(dataDir, "projects", "old"), at.Add(time.Hour))
	claudeSession(t, config, "g", dataDir, at.Add(time.Hour))

	want := []proto.Workspace{{Path: site, Used: at.UnixMilli()}, {Path: notes, Used: at.Add(-2 * time.Hour).UnixMilli()}}
	if got := askWorkspaces(t, h, "claude"); !slices.Equal(got, want) {
		t.Fatalf("claude = %+v, want %+v", got, want)
	}
	qwen := t.TempDir()
	t.Setenv("QWEN_RUNTIME_DIR", qwen)
	claudeSession(t, qwen, filepath.Join("n", "chats"), notes, at)
	if got := askWorkspaces(t, h, "qwen"); !slices.Equal(got, []proto.Workspace{{Path: notes, Used: at.UnixMilli()}}) {
		t.Fatalf("qwen = %+v", got)
	}
	if got := askWorkspaces(t, h, "external"); got == nil || len(got) != 0 {
		t.Fatalf("external = %#v, want []", got)
	}
	for _, req := range []string{`{"t":"host.workspaces","id":9,"agent":"nano"}`, `{`} {
		_, err := h.Workspaces(t.Context(), json.RawMessage(req))
		seamCode(t, err, "bad_request")
	}
}

// A node whose database is in memory has no data directory: nothing is left
// out for it.
func TestWorkspacesWithoutADataDirectory(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	if dir := dataDir(h); within(dir, string(filepath.Separator)) {
		t.Fatalf("data directory %q holds the root", dir)
	}
}

// The list stops where the answer would pass the size of a ctl message
// (contracts/conventions.md section 4).
func TestWorkspacesStopAtTheSizeOfACtlMessage(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	var found []workspaces.Folder
	for i := range 500 {
		dir := filepath.Join(root, fmt.Sprintf("%03d-%s", i, strings.Repeat("a", 100)))
		codexNoErr(t, os.MkdirAll(dir, 0o700))
		found = append(found, workspaces.Folder{Path: dir, Used: time.UnixMilli(int64(1763600000000 - i))})
	}
	list := listWorkspaces(slices.Values(found), "")
	size := func(list []proto.Workspace) int {
		return len(marshalWire(proto.HostWorkspaces{Success: proto.Success{Re: math.MaxInt64}, Workspaces: list}))
	}
	n := len(list)
	if n == 0 || n == len(found) || size(list) > ctlLimit || size(append(list, proto.Workspace{Path: found[n].Path, Used: found[n].Used.UnixMilli()})) <= ctlLimit {
		t.Fatalf("%d folders in %d bytes", n, size(list))
	}
}

// Codex lists its threads through its app server, as its models are listed.
func TestCodexWorkspacesComeFromThreadList(t *testing.T) {
	modelBinaries(t)
	work, _ := filepath.EvalSymlinks(t.TempDir())
	site, notes := filepath.Join(work, "site"), filepath.Join(work, "notes")
	codexNoErr(t, os.Mkdir(site, 0o700))
	codexNoErr(t, os.Mkdir(notes, 0o700))
	modelScript(t, "codex", `{"session":"threads","steps":[
		{"dir":"stdin","msg":{"id":1,"method":"initialize"}},
		{"dir":"stdout","msg":{"id":1,"result":{}}},
		{"dir":"stdin","msg":{"method":"initialized"}},
		{"dir":"stdin","msg":{"id":2,"method":"thread/list","params":{"sortKey":"updated_at","limit":100,"modelProviders":[]}}},
		{"dir":"stdout","msg":{"id":2,"result":{"data":[{"cwd":`+marshalWire(notes)+`,"updatedAt":1763500000}],"nextCursor":"next"}}},
		{"dir":"stdin","msg":{"id":3,"method":"thread/list","params":{"cursor":"next"}}},
		{"dir":"stdout","msg":{"id":3,"result":{"data":[{"cwd":`+marshalWire(site)+`,"updatedAt":1763600000}],"nextCursor":null}}},
		{"dir":"eof"}]}`)
	h, _ := seamHost(t, seamStore(t))
	want := []proto.Workspace{{Path: site, Used: 1763600000000}, {Path: notes, Used: 1763500000000}}
	if got := askWorkspaces(t, h, "codex"); !slices.Equal(got, want) {
		t.Fatalf("codex = %+v, want %+v", got, want)
	}
	modelScript(t, "codex", `{"session":"threads","exit":1}`)
	if got := askWorkspaces(t, h, "codex"); len(got) != 0 {
		t.Fatalf("codex without an answer = %+v", got)
	}
}

// OpenCode lists its sessions through a server of its own.
func TestOpenCodeWorkspacesComeFromItsSessions(t *testing.T) {
	work, _ := filepath.EvalSymlinks(t.TempDir())
	fakeServerCLI(t, "opencode", `{"session":"list","steps":[
		{"dir":"http","msg":{"call":"GET /api/session","body":null,"res":{"data":[{"location":{"directory":`+marshalWire(work)+`},"time":{"updated":1763600000000}}],"cursor":{"next":"n"}}}},
		{"dir":"http","msg":{"call":"GET /api/session","body":null,"res":{"data":[]}}},
		{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`)
	h, _ := seamHost(t, seamStore(t))
	if got := askWorkspaces(t, h, "opencode"); !slices.Equal(got, []proto.Workspace{{Path: work, Used: 1763600000000}}) {
		t.Fatalf("opencode = %+v", got)
	}
	t.Setenv("PATH", t.TempDir())
	if got := askWorkspaces(t, h, "opencode"); len(got) != 0 {
		t.Fatalf("opencode that is not installed = %+v", got)
	}
}
