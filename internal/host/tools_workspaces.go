package host

import (
	"bufio"
	"context"
	"encoding/json"
	"iter"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/host/workspaces"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// workspaceFinders yields, per agent, the folders its CLI was used in on this
// computer, newest use first (contracts/local.md section 7).
var workspaceFinders = map[string]func(context.Context) iter.Seq[workspaces.Folder]{
	"claude":      func(context.Context) iter.Seq[workspaces.Folder] { return workspaces.Claude(userHome()) },
	"qwen":        func(context.Context) iter.Seq[workspaces.Folder] { return workspaces.Qwen(userHome()) },
	"codex":       codexWorkspaces,
	"opencode":    openCodeWorkspaces,
	externalAgent: func(context.Context) iter.Seq[workspaces.Folder] { return slices.Values([]workspaces.Folder{}) },
}

func userHome() string {
	home, _ := os.UserHomeDir()
	return home
}

// Workspaces answers host.workspaces (contracts/datachannel.md section 7).
func (h *Host) Workspaces(ctx context.Context, data json.RawMessage) (any, error) {
	var req proto.HostWorkspacesRequest
	proto.Decode(data, &req)
	find, ok := workspaceFinders[req.Agent]
	if !ok {
		return nil, &Error{"bad_request", "unknown agent: " + req.Agent}
	}
	return proto.HostWorkspaces{Workspaces: listWorkspaces(find(ctx), dataDir(h))}, nil
}

// listWorkspaces resolves each folder as host.join does, keeps the newer use
// of two spellings of one folder, leaves out a folder that does not exist and
// one under the node's data directory, and stops where the answer would pass
// the size of a ctl message.
func listWorkspaces(found iter.Seq[workspaces.Folder], data string) []proto.Workspace {
	list := []proto.Workspace{}
	room := ctlLimit - len(marshalWire(proto.HostWorkspaces{Success: proto.Success{Re: math.MaxInt64}, Workspaces: list}))
	seen := map[string]bool{}
	for f := range found {
		path, err := ResolveWorkspace(f.Path)
		if err != nil || seen[path] || within(data, path) {
			continue
		}
		w := proto.Workspace{Path: path, Used: f.Used.UnixMilli()}
		if room -= len(marshalWire(w)) + min(len(list), 1); room < 0 {
			break
		}
		seen[path] = true
		list = append(list, w)
	}
	return list
}

// dataDir is the node's data directory, the folder of its database file.
func dataDir(h *Host) string {
	var file string
	h.Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&file)
	})
	dir, _ := filepath.EvalSymlinks(filepath.Dir(file))
	return dir
}

// within reports whether path is dir or a folder under it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// codexWorkspaces asks Codex's app server for its threads with thread/list,
// as the models are probed.
func codexWorkspaces(ctx context.Context) iter.Seq[workspaces.Folder] {
	folders, _ := probeCLI(ctx, "codex", []string{"app-server"}, readCodexThreads)
	return slices.Values(folders)
}

func readCodexThreads(from *bufio.Reader, to *json.Encoder) ([]workspaces.Folder, error) {
	if err := codexInitialize(from, to); err != nil {
		return nil, err
	}
	id := int64(1)
	call := func(method string, params any) ([]byte, error) {
		id++
		var result json.RawMessage
		err := modelRPC(from, to, codexRPCRequest{ID: id, Method: method, Params: params}, &result)
		return result, err
	}
	return slices.Collect(workspaces.Codex(call)), nil
}

// openCodeWorkspaces lists OpenCode's sessions through a server of its own,
// as the models are listed.
func openCodeWorkspaces(ctx context.Context) iter.Seq[workspaces.Folder] {
	var folders []workspaces.Folder
	openCodeServer(ctx, func(get func(path string) ([]byte, error)) {
		folders = slices.Collect(workspaces.OpenCode(get))
	})
	return slices.Values(folders)
}
