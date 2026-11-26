// Package workspaces finds the folders an AI CLI was used in on this
// computer, newest use first (planning/contracts/local.md section 7). Each
// finder yields a folder once, as the CLI gave it, with the time of its
// newest session; the caller resolves, filters and cuts the list, and stops
// the walk when it has enough. A source that cannot be read yields nothing.
package workspaces

import (
	"cmp"
	"encoding/json"
	"io/fs"
	"iter"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Folder is a folder a CLI was used in and the time of its newest session.
type Folder struct {
	Path string
	Used time.Time
}

// Claude reads the newest session file of each project folder of Claude Code
// 2.1.311, `projects/<name>/*.jsonl` under CLAUDE_CONFIG_DIR or ~/.claude: the
// first `cwd` of its first 200 records.
func Claude(home string) iter.Seq[Folder] {
	return claude(os.DirFS(cmp.Or(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home, ".claude"))))
}

func claude(fsys fs.FS) iter.Seq[Folder] {
	return newestFirst(newestPerDir(sessionFiles(fsys, "projects/*/*.jsonl")), fileFolder(fsys))
}

// Qwen reads the newest chat file of each project folder of Qwen Code
// v0.28.6, `projects/<sanitized cwd>/chats/*.jsonl` under QWEN_RUNTIME_DIR,
// else QWEN_HOME, else ~/.qwen (`Storage.getRuntimeBaseDir`,
// packages/core/src/config/storage.ts:172-203), as Claude does: every record
// has `cwd` (`ChatRecord`, packages/core/src/services/chatRecordingService.ts:549).
func Qwen(home string) iter.Seq[Folder] {
	return qwen(os.DirFS(cmp.Or(os.Getenv("QWEN_RUNTIME_DIR"), os.Getenv("QWEN_HOME"), filepath.Join(home, ".qwen"))))
}

func qwen(fsys fs.FS) iter.Seq[Folder] {
	return newestFirst(newestPerDir(sessionFiles(fsys, "projects/*/chats/*.jsonl")), fileFolder(fsys))
}

// Codex lists the threads of Codex CLI 0.164.0 through call, which sends a
// request to its app server and returns the answer's `result`: `thread/list`
// pages by `nextCursor`, each thread's `cwd` and `updatedAt` (seconds). An
// empty `modelProviders` lists the threads of every provider, not only of the
// configured one (app-server thread_processor.rs:5919-5929, rust-v0.164.0).
// call bounds its own time; every page is read before the first folder.
func Codex(call func(method string, params any) ([]byte, error)) iter.Seq[Folder] {
	return paged(func(cursor string) ([]session, string) {
		params := map[string]any{"sortKey": "updated_at", "limit": 100, "modelProviders": []string{}}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page struct {
			Data []struct {
				Cwd       string
				UpdatedAt int64
			}
			NextCursor string
		}
		data, err := call("thread/list", params)
		if err != nil || json.Unmarshal(data, &page) != nil {
			return nil, ""
		}
		var sessions []session
		for _, t := range page.Data {
			sessions = append(sessions, session{t.Cwd, time.Unix(t.UpdatedAt, 0)})
		}
		return sessions, page.NextCursor
	})
}

// OpenCode lists every session of OpenCode v1.19.6 through get, which fetches
// a path from its server: `GET /api/session` pages by `cursor.next` until one
// is empty, each session's `location.directory` and `time.updated`
// (milliseconds; packages/schema/src/session.ts, packages/core/src/session.ts
// `list`). The server orders by creation, so every page is read, with get
// bounding its own time, before the first folder.
func OpenCode(get func(path string) ([]byte, error)) iter.Seq[Folder] {
	return paged(func(cursor string) ([]session, string) {
		query := url.Values{"order": {"desc"}, "limit": {"1000"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var page struct {
			Data []struct {
				Location struct{ Directory string }
				Time     struct{ Updated int64 }
			}
			Cursor struct{ Next string }
		}
		data, err := get("/api/session?" + query.Encode())
		if err != nil || json.Unmarshal(data, &page) != nil || len(page.Data) == 0 {
			return nil, ""
		}
		var sessions []session
		for _, s := range page.Data {
			sessions = append(sessions, session{s.Location.Directory, time.UnixMilli(s.Time.Updated)})
		}
		return sessions, page.Cursor.Next
	})
}

// session is a session file, or a session's folder for the CLIs listed by
// their servers, and the time it was last written.
type session struct {
	ref  string
	used time.Time
}

// paged collects the sessions of every page, asking for each next page with
// the cursor the last one gave, until a page gives none.
func paged(page func(cursor string) ([]session, string)) iter.Seq[Folder] {
	sessions, cursor := page("")
	for cursor != "" {
		var more []session
		more, cursor = page(cursor)
		sessions = append(sessions, more...)
	}
	return newestFirst(sessions, func(dir string) string { return dir })
}

func sessionFiles(fsys fs.FS, pattern string) []session {
	names, _ := fs.Glob(fsys, pattern) // the patterns are valid
	var sessions []session
	for _, name := range names {
		if info, err := fs.Stat(fsys, name); err == nil {
			sessions = append(sessions, session{name, info.ModTime()})
		}
	}
	return sessions
}

func newestPerDir(sessions []session) []session {
	newest := map[string]session{}
	for _, s := range sessions {
		if dir := path.Dir(s.ref); s.used.After(newest[dir].used) {
			newest[dir] = s
		}
	}
	return slices.Collect(maps.Values(newest))
}

// newestFirst yields the folder of each session, newest session first, once
// per folder.
func newestFirst(sessions []session, folder func(ref string) string) iter.Seq[Folder] {
	slices.SortFunc(sessions, func(a, b session) int { return cmp.Or(b.used.Compare(a.used), strings.Compare(a.ref, b.ref)) })
	return func(yield func(Folder) bool) {
		seen := map[string]bool{}
		for _, s := range sessions {
			dir := folder(s.ref)
			if dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			if !yield(Folder{dir, s.used}) {
				return
			}
		}
	}
}

// fileFolder returns the first `cwd` in the first 200 records of a session
// file.
func fileFolder(fsys fs.FS) func(name string) string {
	return func(name string) string {
		f, err := fsys.Open(name)
		if err != nil {
			return ""
		}
		defer f.Close()
		in := json.NewDecoder(f)
		for range 200 {
			var r struct{ Cwd string }
			if in.Decode(&r) != nil {
				return ""
			}
			if r.Cwd != "" {
				return r.Cwd
			}
		}
		return ""
	}
}
