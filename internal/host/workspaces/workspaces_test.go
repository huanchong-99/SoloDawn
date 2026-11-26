package workspaces

import (
	"errors"
	"io/fs"
	"iter"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var t0 = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

// write makes the session file name under root, modified at t0 plus minutes,
// with one JSON record per line.
func write(t *testing.T, root, name string, minutes int, records ...string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Join(records, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	at := t0.Add(time.Duration(minutes) * time.Minute)
	if err := os.Chtimes(file, at, at); err != nil {
		t.Fatal(err)
	}
}

func folders(seq iter.Seq[Folder]) []Folder { return slices.Collect(seq) }

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

func same(t *testing.T, got, want []Folder) {
	t.Helper()
	if !slices.EqualFunc(got, want, func(a, b Folder) bool { return a.Path == b.Path && a.Used.Equal(b.Used) }) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestClaudeNewestFileOfEachProjectNewestFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := filepath.Join(home, ".claude")
	write(t, p, "projects/-work-site/old.jsonl", 1, `{"cwd":"/work/old"}`)
	write(t, p, "projects/-work-site/new.jsonl", 5, `{"type":"queue-operation"}`, `{"cwd":""}`, `{"cwd":"/work/site","sessionId":"s"}`, `{"cwd":"/work/elsewhere"}`)
	write(t, p, "projects/-work-api/a.jsonl", 3, `{"cwd":"/work/api"}`)
	write(t, p, "projects/-work-twin/a.jsonl", 2, `{"cwd":"/work/site"}`)
	write(t, p, "projects/-work-summary/a.jsonl", 9, `{"type":"summary"}`)
	write(t, p, "projects/-work-broken/a.jsonl", 8, `{"type":`)
	write(t, p, "projects/-work-site/sub/agent.jsonl", 10, `{"cwd":"/work/sub"}`)
	write(t, p, "projects/-work-site/notes.txt", 10, `{"cwd":"/work/txt"}`)
	same(t, folders(Claude(home)), []Folder{{"/work/site", at(5)}, {"/work/api", at(3)}})
}

func TestClaudeConfigDir(t *testing.T) {
	config := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	write(t, config, "projects/-x/a.jsonl", 1, `{"cwd":"/x"}`)
	same(t, folders(Claude(t.TempDir())), []Folder{{"/x", at(1)}})
}

func TestClaudeReadsAtMost200Records(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	p := filepath.Join(home, ".claude")
	filler := slices.Repeat([]string{`{"type":"user"}`}, 199)
	write(t, p, "projects/-in/a.jsonl", 2, append(filler, `{"cwd":"/in"}`)...)
	write(t, p, "projects/-out/a.jsonl", 1, append(filler, `{"type":"user"}`, `{"cwd":"/out"}`)...)
	same(t, folders(Claude(home)), []Folder{{"/in", at(2)}})
}

func TestQwenNewestChatOfEachProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("QWEN_RUNTIME_DIR", "")
	t.Setenv("QWEN_HOME", "")
	q := filepath.Join(home, ".qwen")
	record := func(cwd string) string {
		return `{"uuid":"u","parentUuid":null,"sessionId":"s","timestamp":"t","type":"user","cwd":"` + cwd + `","version":"0.28.6"}`
	}
	write(t, q, "projects/-work-site/chats/0f3c.jsonl", 6, record("/work/site"))
	write(t, q, "projects/-work-site/chats/1a2b.jsonl", 2, record("/work/older"))
	write(t, q, "projects/-work-api/chats/9e8d.jsonl", 4, record("/work/api"))
	write(t, q, "tmp/-work-tmp/chats/7c6b.jsonl", 9, record("/work/tmp"))
	same(t, folders(Qwen(home)), []Folder{{"/work/site", at(6)}, {"/work/api", at(4)}})
}

func TestNothingToRead(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("QWEN_RUNTIME_DIR", "")
	t.Setenv("QWEN_HOME", "")
	home := filepath.Join(t.TempDir(), "missing")
	for _, seq := range []iter.Seq[Folder]{Claude(home), Qwen(home)} {
		same(t, folders(seq), nil)
	}
}

// gone lists every file of MapFS but cannot stat the file stat or open the
// file open, as when a session file is deleted while it is read.
type gone struct {
	fstest.MapFS
	stat, open string
}

func (g gone) Stat(name string) (fs.FileInfo, error) {
	if name == g.stat {
		return nil, fs.ErrNotExist
	}
	return g.MapFS.Stat(name)
}

func (g gone) Open(name string) (fs.File, error) {
	if name == g.open {
		return nil, fs.ErrNotExist
	}
	return g.MapFS.Open(name)
}

func TestFileGoneWhileRead(t *testing.T) {
	file := func(cwd string, minutes int) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(`{"cwd":"` + cwd + `"}`), ModTime: at(minutes)}
	}
	fsys := gone{MapFS: fstest.MapFS{
		"projects/a/s.jsonl": file("/a", 1),
		"projects/b/s.jsonl": file("/b", 2),
		"projects/c/s.jsonl": file("/c", 3),
	}, stat: "projects/b/s.jsonl", open: "projects/c/s.jsonl"}
	same(t, folders(claude(fsys)), []Folder{{"/a", at(1)}})
}

func TestStopsWhenTheCallerHasEnough(t *testing.T) {
	read := 0
	fsys := fstest.MapFS{}
	for i, dir := range []string{"a", "b", "c"} {
		fsys["projects/"+dir+"/s.jsonl"] = &fstest.MapFile{Data: []byte(`{"cwd":"/` + dir + `"}`), ModTime: at(i)}
	}
	seq := newestFirst(newestPerDir(sessionFiles(fsys, "projects/*/*.jsonl")), func(name string) string {
		read++
		return fileFolder(fsys)(name)
	})
	for f := range seq {
		if f.Path != "/c" {
			t.Fatalf("first folder %v, want /c", f)
		}
		break
	}
	if read != 1 {
		t.Fatalf("read %d files, want 1", read)
	}
}

func TestOpenCodeSessionsAcrossPages(t *testing.T) {
	pages := map[string]string{
		"": `{"data":[` +
			`{"id":"s3","location":{"directory":"/work/site"},"time":{"created":30,"updated":` + ms(3) + `}},` +
			`{"id":"s2","location":{"directory":"/work/api"},"time":{"created":20,"updated":` + ms(6) + `}},` +
			`{"id":"s1","location":{"directory":""},"time":{"created":10,"updated":` + ms(9) + `}}` +
			`],"cursor":{"previous":"p1","next":"n1"}}`,
		"n1": `{"data":[{"id":"s0","location":{"directory":"/work/site","workspaceID":"w"},"time":{"created":5,"updated":` + ms(8) + `}}],"cursor":{"next":"n2"}}`,
		"n2": `{"data":[],"cursor":{}}`,
	}
	var asked []string
	get := func(p string) ([]byte, error) {
		asked = append(asked, p)
		u, err := url.Parse(p)
		if err != nil || u.Path != "/api/session" || u.Query().Get("order") != "desc" || u.Query().Get("limit") != "1000" {
			t.Fatalf("asked %q", p)
		}
		return []byte(pages[u.Query().Get("cursor")]), nil
	}
	same(t, folders(OpenCode(get)), []Folder{{"/work/site", at(8)}, {"/work/api", at(6)}})
	if len(asked) != 3 {
		t.Fatalf("asked %v", asked)
	}
}

func ms(minutes int) string { return strconv.FormatInt(at(minutes).UnixMilli(), 10) }

func TestOpenCodeServerFails(t *testing.T) {
	first := `{"data":[{"location":{"directory":"/a"},"time":{"updated":` + ms(1) + `}}],"cursor":{"next":"n"}}`
	for name, second := range map[string]func() ([]byte, error){
		"error":    func() ([]byte, error) { return nil, errors.New("connection refused") },
		"not json": func() ([]byte, error) { return []byte("<html>"), nil },
	} {
		calls := 0
		get := func(string) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte(first), nil
			}
			return second()
		}
		t.Run(name, func(t *testing.T) { same(t, folders(OpenCode(get)), []Folder{{"/a", at(1)}}) })
	}
}

func TestQwenRuntimeDirThenQwenHome(t *testing.T) {
	runtime, qwenHome := t.TempDir(), t.TempDir()
	write(t, runtime, "projects/-r/chats/a.jsonl", 1, `{"cwd":"/r"}`)
	write(t, qwenHome, "projects/-h/chats/a.jsonl", 2, `{"cwd":"/h"}`)
	t.Setenv("QWEN_HOME", qwenHome)
	t.Setenv("QWEN_RUNTIME_DIR", runtime)
	same(t, folders(Qwen(t.TempDir())), []Folder{{"/r", at(1)}})
	t.Setenv("QWEN_RUNTIME_DIR", "")
	same(t, folders(Qwen(t.TempDir())), []Folder{{"/h", at(2)}})
}

func TestCodexThreadsAcrossPages(t *testing.T) {
	sec := func(minutes int) string { return strconv.FormatInt(at(minutes).Unix(), 10) }
	thread := func(id, cwd string, minutes int) string {
		return `{"id":"` + id + `","cwd":"` + cwd + `","createdAt":1,"updatedAt":` + sec(minutes) + `,"source":"cli","turns":[]}`
	}
	pages := map[string]string{
		"":   `{"data":[` + thread("t3", "/work/site", 6) + `,` + thread("t2", "/work/api", 4) + `],"nextCursor":"c1","backwardsCursor":"b"}`,
		"c1": `{"data":[` + thread("t1", "/work/site", 2) + `,` + thread("t0", "/work/old", 1) + `],"nextCursor":null}`,
	}
	var asked []string
	call := func(method string, params any) ([]byte, error) {
		p := params.(map[string]any)
		cursor, _ := p["cursor"].(string)
		delete(p, "cursor")
		// an empty, not a missing, modelProviders: every provider's threads
		if want := map[string]any{"sortKey": "updated_at", "limit": 100, "modelProviders": []string{}}; method != "thread/list" || !reflect.DeepEqual(p, want) {
			t.Fatalf("asked %s %v", method, params)
		}
		asked = append(asked, cursor)
		return []byte(pages[cursor]), nil
	}
	same(t, folders(Codex(call)), []Folder{{"/work/site", at(6)}, {"/work/api", at(4)}, {"/work/old", at(1)}})
	if !slices.Equal(asked, []string{"", "c1"}) {
		t.Fatalf("asked cursors %q", asked)
	}
}

func TestCodexServerFails(t *testing.T) {
	first := `{"data":[{"cwd":"/a","updatedAt":` + strconv.FormatInt(at(1).Unix(), 10) + `}],"nextCursor":"n"}`
	for name, second := range map[string]func() ([]byte, error){
		"error":    func() ([]byte, error) { return nil, errors.New("app server exited") },
		"not json": func() ([]byte, error) { return []byte("{"), nil },
	} {
		calls := 0
		call := func(string, any) ([]byte, error) {
			calls++
			if calls == 1 {
				return []byte(first), nil
			}
			return second()
		}
		t.Run(name, func(t *testing.T) { same(t, folders(Codex(call)), []Folder{{"/a", at(1)}}) })
	}
}
