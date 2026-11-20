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
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func checkImage(t *testing.T, path, content string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || string(b) != content {
		t.Fatalf("binary = %q, %v; want %q", b, err, content)
	}
}

type updateFixture struct {
	u              Updater
	name, manifest string
	requests       map[string]int
	restarts       int
	busy           bool
}

func setupUpdate(t *testing.T, mux *http.ServeMux) *updateFixture {
	t.Helper()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	f := &updateFixture{
		name:     "solodawn_1.10.1_" + updateTarget(runtime.GOOS, runtime.GOARCH),
		requests: make(map[string]int),
	}
	f.manifest = fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(newImage)), f.name)
	f.u = Updater{
		ReleaseURL: server.URL + "/latest/download/",
		Binary:     filepath.Join(t.TempDir(), "solodawn"),
		Version:    "v1.9.1",
		Busy:       func() bool { return f.busy },
		Restart:    func() error { f.restarts++; return nil },
	}
	writeImage(t, f.u.Binary, "old binary")
	return f
}

func (f *updateFixture) serve(mux *http.ServeMux, image string) {
	mux.HandleFunc("/latest/download/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		f.requests["manifest"]++
		fmt.Fprint(w, f.manifest)
	})
	mux.HandleFunc("/latest/download/"+f.name, func(w http.ResponseWriter, r *http.Request) {
		f.requests["binary"]++
		fmt.Fprint(w, image)
	})
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func checkUpdateValue(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestUpdateWhenIdle(t *testing.T) {
	mux := http.NewServeMux()
	f := setupUpdate(t, mux)
	f.serve(mux, newImage)
	now := time.Now()
	noErr(t, f.u.Apply())
	noErr(t, f.u.Check(t.Context(), now))
	f.busy = true // a session started during download
	noErr(t, f.u.Apply())
	noErr(t, f.u.Check(t.Context(), now.Add(48*time.Hour)))
	checkImage(t, f.u.Binary, "old binary")
	checkUpdateValue(t, f.u.Status, (*UpdateStatus)(nil))
	checkUpdateValue(t, f.restarts, 0)
	info, err := os.Stat(f.u.Binary)
	noErr(t, err)
	f.busy = false
	noErr(t, f.u.Apply())
	noErr(t, f.u.Apply())
	checkImage(t, f.u.Binary, newImage)
	installed, err := os.Stat(f.u.Binary)
	noErr(t, err)
	checkUpdateValue(t, installed.Mode(), info.Mode())
	checkUpdateValue(t, f.u.Status, &UpdateStatus{Version: "1.10.1", Status: "pending"})
	checkUpdateValue(t, f.restarts, 1)
	checkUpdateValue(t, f.requests, map[string]int{"manifest": 1, "binary": 1})
}

func TestUpdateMismatchAndDailyCheck(t *testing.T) {
	mux := http.NewServeMux()
	f := setupUpdate(t, mux)
	f.serve(mux, "different bytes")
	var logged bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(oldWriter) })
	now := time.Now()
	checkUpdateValue(t, f.u.Check(t.Context(), now), errUpdateMismatch)
	if !strings.HasSuffix(logged.String(), errUpdateMismatch.Error()+" for "+f.name+"\n") {
		t.Fatalf("log = %q", logged.String())
	}
	noErr(t, f.u.Apply())
	checkImage(t, f.u.Binary, "old binary")
	assertOnlyInstalled(t, f.u.Binary)
	checkUpdateValue(t, f.u.Status, &UpdateStatus{Version: "1.10.1", Status: "mismatch"})
	checkUpdateValue(t, f.restarts, 0)
	f.u.Version = "1.10.1"
	noErr(t, f.u.Check(t.Context(), now.Add(24*time.Hour-time.Nanosecond)))
	checkUpdateValue(t, f.requests, map[string]int{"manifest": 1, "binary": 1})
	noErr(t, f.u.Check(t.Context(), now.Add(24*time.Hour)))
	checkUpdateValue(t, f.u.Status, (*UpdateStatus)(nil))
	checkUpdateValue(t, f.requests, map[string]int{"manifest": 2, "binary": 1})
}

func assertOnlyInstalled(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	noErr(t, err)
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func TestApplyFailures(t *testing.T) {
	mux := http.NewServeMux()
	f := setupUpdate(t, mux)
	f.serve(mux, newImage)
	noErr(t, f.u.Check(t.Context(), time.Now()))
	noErr(t, os.Remove(f.u.Binary))
	if err := f.u.Apply(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("install: got %v, want ErrNotExist", err)
	}
	writeImage(t, f.u.Binary, "old binary")
	f.u.Restart = func() error { return io.ErrClosedPipe }
	if err := f.u.Apply(); err != io.ErrClosedPipe {
		t.Fatalf("restart: got %v", err)
	}
	checkImage(t, f.u.Binary, newImage)
	f.u.Restart = func() error { f.restarts++; return nil }
	f.busy = true
	noErr(t, f.u.Apply())
	if f.restarts != 0 {
		t.Fatal("retried restart while busy")
	}
	f.busy = false
	noErr(t, f.u.Apply())
	if f.restarts != 1 {
		t.Fatalf("restart requests = %d", f.restarts)
	}
}

func TestReleaseSelection(t *testing.T) {
	for _, tc := range []struct {
		version, current string
		newer            bool
	}{
		{"2.0.0", "1.9.9", true}, {"1.3.0", "v1.2.9", true}, {"1.0.10", "1.0.9", true},
		{"1.2.3", "1.2.3", false}, {"1.1.9", "1.2.0", false}, {"1.0.1", "dev", false},
	} {
		t.Run(tc.version+"_vs_"+tc.current, func(t *testing.T) {
			checkUpdateValue(t, newerRelease(tc.version, tc.current), tc.newer)
		})
	}
	sum := strings.Repeat("A", 64)
	name := "solodawn_v1.2.3_windows_arm64.exe"
	line := sum + " *" + name + "\r\n"
	target := updateTarget("windows", "arm64")
	file, err := findRelease(strings.NewReader(sum+"  solodawn_1.2.3_linux_arm64\n"+line), target)
	noErr(t, err)
	checkUpdateValue(t, *file, releaseFile{name: name, version: "1.2.3", sum: sum})
	if _, err := findRelease(strings.NewReader(line+line), target); err == nil || err.Error() != "self-update: multiple SHA256SUMS entries for windows_arm64.exe" {
		t.Fatalf("duplicate entry: %v", err)
	}
	_, err = findRelease(iotest.ErrReader(io.ErrUnexpectedEOF), target)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("manifest read: %v", err)
	}
}

func TestUpdateHTTPFailures(t *testing.T) {
	mux := http.NewServeMux()
	f := setupUpdate(t, mux)
	now := time.Now()
	checkUpdateValue(t, fmt.Sprint(f.u.Check(t.Context(), now)), "self-update: SHA256SUMS: 404 Not Found")
	mux.HandleFunc("/latest/download/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, f.manifest) })
	checkUpdateValue(t, fmt.Sprint(f.u.Check(t.Context(), now.Add(24*time.Hour))), "self-update: "+f.name+": 404 Not Found")
	f.manifest = "xyz  " + f.name
	want := "self-update: no valid SHA256SUMS entry for " + updateTarget(runtime.GOOS, runtime.GOARCH)
	if err := f.u.Check(t.Context(), now.Add(48*time.Hour)); err == nil || err.Error() != want {
		t.Fatalf("invalid manifest: %v", err)
	}
	checkImage(t, f.u.Binary, "old binary")
	assertOnlyInstalled(t, f.u.Binary)
}

func TestUpdateRequestErrors(t *testing.T) {
	u := Updater{ReleaseURL: "https://%zz"}
	if _, err := u.open(t.Context(), "file"); err == nil || err.Error() != `parse "https://%zz/file": invalid URL escape "%zz"` {
		t.Fatalf("invalid URL: %v", err)
	}
	u.ReleaseURL = "https://example.invalid"
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := u.open(ctx, "file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request: %v", err)
	}
}

func TestStageErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := stageRelease(filepath.Join(dir, "missing"), strings.NewReader(""), ""); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("create stage: %v", err)
	}
	if _, err := stageRelease(dir, iotest.ErrReader(io.ErrUnexpectedEOF), ""); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("download: %v", err)
	}
	entries, err := os.ReadDir(dir)
	noErr(t, err)
	if len(entries) != 0 {
		t.Fatalf("partial downloads: %v", entries)
	}
}

func TestWindowsInstall(t *testing.T) {
	dir := t.TempDir()
	binary, staged := filepath.Join(dir, "solodawn.exe"), filepath.Join(dir, "stage")
	writeImage(t, binary, "old")
	if err := installRelease(staged, binary, "windows"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing staged file: %v", err)
	}
	writeImage(t, staged, "new")
	noErr(t, os.Mkdir(binary+".old", 0700))
	if err := installRelease(staged, binary, "windows"); err == nil {
		t.Fatal("replaced a directory with the backup")
	}
	checkImage(t, binary, "old")
	noErr(t, os.Remove(binary+".old"))
	if err := installRelease(binary, binary, "windows"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed second rename: %v", err)
	}
	checkImage(t, binary, "old") // rollback restored the running version
	noErr(t, installRelease(staged, binary, "windows"))
	checkImage(t, binary, "new")
	checkImage(t, binary+".old", "old")
}
