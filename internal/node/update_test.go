package node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

const newImage = "new release binary\x00"

func writeImage(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func checkImage(t *testing.T, path, content string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != content {
		t.Fatalf("%s = %q, %v; want %q", path, b, err, content)
	}
}

func checkUpdateValue(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func checkFolder(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	noErr(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	checkUpdateValue(t, names, want)
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type updateFixture struct {
	u        Updater
	name     string
	manifest string
	image    string
	requests map[string]int
	restarts int
	busy     bool
}

// newUpdate serves release v1.10.1 to a node running v1.9.1 whose binary is
// the only file in its folder.
func newUpdate(t *testing.T) (*updateFixture, *http.ServeMux) {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	f := &updateFixture{
		name:     "solodawn_v1.10.1_" + updateTarget(runtime.GOOS, runtime.GOARCH),
		image:    newImage,
		requests: map[string]int{},
	}
	f.manifest = fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(newImage)), f.name)
	f.u = Updater{
		ReleaseURL: server.URL + "/download",
		Binary:     filepath.Join(t.TempDir(), "solodawn"),
		Version:    "v1.9.1",
		Busy:       func() bool { return f.busy },
		Restart:    func() error { f.restarts++; return nil },
	}
	writeImage(t, f.u.Binary, "old binary")
	return f, mux
}

func (f *updateFixture) serve(mux *http.ServeMux) {
	mux.HandleFunc("/download/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
		f.requests["SHA256SUMS"]++
		fmt.Fprint(w, f.manifest)
	})
	mux.HandleFunc("/download/"+f.name, func(w http.ResponseWriter, _ *http.Request) {
		f.requests["binary"]++
		fmt.Fprint(w, f.image)
	})
}

func TestUpdateInstallsThenRestartsWhenIdle(t *testing.T) {
	f, mux := newUpdate(t)
	f.serve(mux)
	now := time.Now()
	noErr(t, f.u.Apply())
	f.busy = true
	noErr(t, f.u.Check(t.Context(), now))
	checkImage(t, f.u.Binary, newImage)
	info, err := os.Stat(f.u.Binary)
	noErr(t, err)
	checkUpdateValue(t, info.Mode(), fs.FileMode(0o755))
	checkFolder(t, filepath.Dir(f.u.Binary), "solodawn")
	checkUpdateValue(t, f.u.Status, &UpdateStatus{Version: "v1.10.1", Status: "pending"})
	noErr(t, f.u.Apply())
	checkUpdateValue(t, f.restarts, 0)
	f.busy = false
	noErr(t, f.u.Apply())
	checkUpdateValue(t, f.restarts, 1)
	// An installed release waits for its restart; no further check runs.
	noErr(t, f.u.Check(t.Context(), now.Add(48*time.Hour)))
	checkUpdateValue(t, f.requests, map[string]int{"SHA256SUMS": 1, "binary": 1})
}

func TestUpdateRestartError(t *testing.T) {
	f, mux := newUpdate(t)
	f.serve(mux)
	noErr(t, f.u.Check(t.Context(), time.Now()))
	f.u.Restart = func() error { return io.ErrClosedPipe }
	checkUpdateValue(t, f.u.Apply(), io.ErrClosedPipe)
}

func TestUpdateMismatchAndDailyCheck(t *testing.T) {
	f, mux := newUpdate(t)
	f.image = "different bytes"
	f.serve(mux)
	var logged bytes.Buffer
	writer, flags := log.Writer(), log.Flags()
	log.SetOutput(&logged)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(writer); log.SetFlags(flags) })
	now := time.Now()
	noErr(t, f.u.Check(t.Context(), now))
	checkUpdateValue(t, logged.String(), "self-update: SHA-256 mismatch for "+f.name+"\n")
	checkImage(t, f.u.Binary, "old binary")
	checkFolder(t, filepath.Dir(f.u.Binary), "solodawn")
	checkUpdateValue(t, f.u.Status, &UpdateStatus{Version: "v1.10.1", Status: "mismatch"})
	noErr(t, f.u.Apply())
	checkUpdateValue(t, f.restarts, 0)
	f.u.Version = "v1.10.1"
	noErr(t, f.u.Check(t.Context(), now.Add(24*time.Hour-time.Nanosecond)))
	checkUpdateValue(t, f.requests, map[string]int{"SHA256SUMS": 1, "binary": 1})
	noErr(t, f.u.Check(t.Context(), now.Add(24*time.Hour)))
	checkUpdateValue(t, f.u.Status, (*UpdateStatus)(nil))
	checkUpdateValue(t, f.requests, map[string]int{"SHA256SUMS": 2, "binary": 1})
}

func TestUpdateFailures(t *testing.T) {
	f, mux := newUpdate(t)
	now := time.Now()
	checkUpdateValue(t, fmt.Sprint(f.u.Check(t.Context(), now)), "self-update: SHA256SUMS: 404 Not Found")
	mux.HandleFunc("/download/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, f.manifest) })
	checkUpdateValue(t, fmt.Sprint(f.u.Check(t.Context(), now.Add(24*time.Hour))), "self-update: "+f.name+": 404 Not Found")
	f.manifest = "xyz  " + f.name
	checkUpdateValue(t, fmt.Sprint(f.u.Check(t.Context(), now.Add(48*time.Hour))), "self-update: no SHA256SUMS entry for "+updateTarget(runtime.GOOS, runtime.GOARCH))
	checkImage(t, f.u.Binary, "old binary")
	checkFolder(t, filepath.Dir(f.u.Binary), "solodawn")
	checkUpdateValue(t, f.u.Status, (*UpdateStatus)(nil))
}

func TestUpdateInstallFailure(t *testing.T) {
	f, mux := newUpdate(t)
	f.serve(mux)
	noErr(t, os.Remove(f.u.Binary))
	noErr(t, os.MkdirAll(filepath.Join(f.u.Binary, "in use"), 0o700))
	var rename *os.LinkError
	if err := f.u.Check(t.Context(), time.Now()); !errors.As(err, &rename) || rename.Op != "rename" || rename.New != f.u.Binary {
		t.Fatalf("install over a folder: %v", err)
	}
	checkUpdateValue(t, f.u.Status, (*UpdateStatus)(nil))
}

func TestUpdateRequestErrors(t *testing.T) {
	u := Updater{ReleaseURL: "https://%zz"}
	_, err := u.get(t.Context(), "file")
	checkUpdateValue(t, fmt.Sprint(err), `parse "https://%zz/file": invalid URL escape "%zz"`)
	u.ReleaseURL = "https://example.invalid"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := u.get(ctx, "file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request: %v", err)
	}
}

func TestReleaseSelection(t *testing.T) {
	for _, tc := range []struct {
		tag, current string
		newer        bool
	}{
		{"v2.0.0", "v1.9.9", true}, {"v1.3.0", "v1.2.9", true}, {"v1.0.10", "v1.0.9", true},
		{"v1.2.3", "v1.2.3", false}, {"v1.1.9", "v1.2.0", false}, {"v1.0.9", "v1.0.10", false},
		{"v1.0.1", "dev", false}, {"1.2.3", "v1.2.0", false}, {"v1.2.3-rc.1", "v1.2.0", false},
		{"v01.2.3", "v1.2.0", false},
	} {
		checkUpdateValue(t, []any{tc.tag, tc.current, newerRelease(tc.tag, tc.current)}, []any{tc.tag, tc.current, tc.newer})
	}
}

func TestFindRelease(t *testing.T) {
	sum := strings.Repeat("a", 64)
	target := updateTarget("windows", "arm64")
	name, tag, got, err := findRelease(strings.NewReader(sum+"  solodawn_v1.2.3_linux_arm64\n"+sum+"  solodawn_v1.2.3_windows_arm64.exe\r\n"), target)
	checkUpdateValue(t, []any{name, tag, got, err}, []any{"solodawn_v1.2.3_windows_arm64.exe", "v1.2.3", sum, nil})
	_, _, _, err = findRelease(iotest.ErrReader(io.ErrUnexpectedEOF), target)
	checkUpdateValue(t, err, io.ErrUnexpectedEOF)
}

func TestStageErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := stageRelease(filepath.Join(dir, "missing"), strings.NewReader(""), ""); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("create stage: %v", err)
	}
	if _, err := stageRelease(dir, iotest.ErrReader(io.ErrUnexpectedEOF), ""); !errors.Is(err, io.ErrUnexpectedEOF) || err.Error() != "unexpected EOF" {
		t.Fatalf("download: %v", err)
	}
	checkFolder(t, dir)
}

func TestInstallRelease(t *testing.T) {
	dir := t.TempDir()
	binary, staged := filepath.Join(dir, "solodawn.exe"), filepath.Join(dir, "stage")
	if err := installRelease(staged, binary, "windows"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing staged file: %v", err)
	}
	writeImage(t, staged, "new")
	if err := installRelease(staged, binary, "windows"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing binary: %v", err)
	}
	writeImage(t, binary, "old")
	writeImage(t, binary+".old", "older")
	noErr(t, installRelease(staged, binary, "windows"))
	checkImage(t, binary, "new")
	checkImage(t, binary+".old", "old")
	writeImage(t, staged, "newer")
	noErr(t, installRelease(staged, binary, "linux"))
	checkImage(t, binary, "newer")
	checkImage(t, binary+".old", "old")
	checkFolder(t, dir, "solodawn.exe", "solodawn.exe.old")
}
