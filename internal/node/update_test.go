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
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"testing/synctest"
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
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	f := newOfflineUpdate(t)
	f.u.ReleaseURL = server.URL + "/download"
	f.u.Transport = server.Client().Transport
	return f, mux
}

// newOfflineUpdate is newUpdate without a server, which a synctest bubble
// cannot wait out: the caller sets ReleaseURL and Transport.
func newOfflineUpdate(t *testing.T) *updateFixture {
	t.Helper()
	f := &updateFixture{
		name:     "solodawn_v1.10.1_" + updateTarget(runtime.GOOS, runtime.GOARCH),
		image:    newImage,
		requests: map[string]int{},
	}
	f.manifest = fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(newImage)), f.name)
	f.u = Updater{
		Binary:  filepath.Join(t.TempDir(), "solodawn"),
		Version: "v1.9.1",
		Busy:    func() bool { return f.busy },
		Restart: func() error { f.restarts++; return nil },
	}
	writeImage(t, f.u.Binary, "old binary")
	return f
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
	// Windows has no executable bit, and keeps the replaced image beside the new one.
	mode, folder := fs.FileMode(0o755), []string{"solodawn"}
	if runtime.GOOS == "windows" {
		mode, folder = 0o666, []string{"solodawn", "solodawn.old"}
	}
	checkUpdateValue(t, info.Mode(), mode)
	checkFolder(t, filepath.Dir(f.u.Binary), folder...)
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
	// Windows first moves the binary aside, which a folder survives; a
	// folder where it goes stops the install there.
	stuck := f.u.Binary
	if runtime.GOOS == "windows" {
		stuck += ".old"
		noErr(t, os.MkdirAll(filepath.Join(stuck, "in use"), 0o700))
	}
	var rename *os.LinkError
	if err := f.u.Check(t.Context(), time.Now()); !errors.As(err, &rename) || rename.Op != "rename" || rename.New != stuck {
		t.Fatalf("install over a folder: %v", err)
	}
	checkUpdateValue(t, f.u.Status, (*UpdateStatus)(nil))
	// The download that could not be installed is gone.
	want := []string{"solodawn"}
	if runtime.GOOS == "windows" {
		want = append(want, "solodawn.old")
	}
	checkFolder(t, filepath.Dir(f.u.Binary), want...)
}

func TestUpdateNeedsHTTPS(t *testing.T) {
	f, _ := newUpdate(t)
	f.u.ReleaseURL = "http://127.0.0.1:1/download"
	checkUpdateValue(t, fmt.Sprint(f.u.Check(t.Context(), time.Now())), "self-update: http://127.0.0.1:1/download is not https")
}

// GitHub redirects twice; up to maxRedirects are followed, each to https.
func TestUpdateRedirects(t *testing.T) {
	f, mux := newUpdate(t)
	f.serve(mux)
	mux.HandleFunc("/hop/{n}/{file}", func(w http.ResponseWriter, r *http.Request) {
		next := "/download/" + r.PathValue("file")
		if n, _ := strconv.Atoi(r.PathValue("n")); n > 1 {
			next = fmt.Sprintf("/hop/%d/%s", n-1, r.PathValue("file"))
		}
		http.Redirect(w, r, next, http.StatusFound)
	})
	mux.HandleFunc("/plain/{file}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+"/download/"+r.PathValue("file"), http.StatusFound)
	})
	release := f.u.ReleaseURL
	f.u.ReleaseURL = strings.Replace(release, "/download", "/hop/5", 1)
	body, err := f.u.get(t.Context(), "SHA256SUMS")
	noErr(t, err)
	got, _ := io.ReadAll(body)
	body.Close()
	checkUpdateValue(t, string(got), f.manifest)
	for _, path := range []string{"/hop/6", "/plain"} {
		f.u.ReleaseURL = strings.Replace(release, "/download", path, 1)
		if _, err := f.u.get(t.Context(), "SHA256SUMS"); !errors.Is(err, errRedirect) {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// A download longer than the limit is cut, and fails its checksum.
func TestUpdateSizeLimit(t *testing.T) {
	f, mux := newUpdate(t)
	f.image = strings.Repeat("x", 1000)
	f.manifest = fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(f.image)), f.name)
	f.serve(mux)
	saved := maxDownload
	t.Cleanup(func() { maxDownload = saved })
	maxDownload = int64(len(f.manifest)) // all of SHA256SUMS, a tenth of the binary
	noErr(t, f.u.Check(t.Context(), time.Now()))
	checkUpdateValue(t, f.u.Status, &UpdateStatus{Version: "v1.10.1", Status: "mismatch"})
	checkImage(t, f.u.Binary, "old binary")
}

// setStall sets the stall limit for the test and returns a channel that a
// handler waits on to hang: it is closed when the test ends, before the
// server stops. The request's own context is not used for that, because the
// server may end it before the client gives up.
func setStall(t *testing.T, d time.Duration) <-chan struct{} {
	saved := stallTimeout
	hang := make(chan struct{})
	t.Cleanup(func() { stallTimeout = saved; close(hang) })
	stallTimeout = d
	return hang
}

// No answer within the stall limit: the request is given up.
func TestUpdateStallsWithoutAnswer(t *testing.T) {
	f, mux := newUpdate(t)
	hang := setStall(t, 50*time.Millisecond)
	mux.HandleFunc("/download/SHA256SUMS", func(http.ResponseWriter, *http.Request) { <-hang })
	if err := f.u.Check(t.Context(), time.Now()); !errors.Is(err, errStalled) {
		t.Fatalf("a server that never answers: %v", err)
	}
}

// The body stops halfway: the download is given up and nothing is installed.
func TestUpdateStallsInTheBody(t *testing.T) {
	f, mux := newUpdate(t)
	hang := setStall(t, 50*time.Millisecond)
	f.serveBinary(mux, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(f.image[:4]))
		w.(http.Flusher).Flush()
		<-hang
	})
	if err := f.u.Check(t.Context(), time.Now()); !errors.Is(err, errStalled) {
		t.Fatalf("a body that stops: %v", err)
	}
	checkImage(t, f.u.Binary, "old binary")
	checkFolder(t, filepath.Dir(f.u.Binary), "solodawn")
}

// trickle is a release server in memory: the manifest at once, the binary one
// byte every gap, and nothing at all when gap is negative. A canceled request
// ends with its cancel cause, as net/http does.
type trickle struct {
	f   *updateFixture
	gap time.Duration
}

func (tr trickle) RoundTrip(req *http.Request) (*http.Response, error) {
	var body io.ReadCloser = io.NopCloser(strings.NewReader(tr.f.manifest))
	if strings.HasSuffix(req.URL.Path, tr.f.name) {
		body = &trickleBody{ctx: req.Context(), gap: tr.gap, left: tr.f.image}
	}
	return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
}

type trickleBody struct {
	ctx  context.Context
	gap  time.Duration
	left string
}

func (b *trickleBody) Read(p []byte) (int, error) {
	if b.left == "" {
		return 0, io.EOF
	}
	select {
	case <-b.ctx.Done():
		return 0, context.Cause(b.ctx)
	case <-time.After(b.gap):
	}
	p[0], b.left = b.left[0], b.left[1:]
	return 1, nil
}

func (b *trickleBody) Close() error { return nil }

// startTrickle makes the fixture download from a trickle, on the clock of the
// synctest bubble the caller is in.
func startTrickle(t *testing.T, gap time.Duration) *updateFixture {
	f := newOfflineUpdate(t)
	f.u.ReleaseURL = "https://release.test/download"
	f.u.Transport = trickle{f, gap}
	setStall(t, 200*time.Millisecond)
	return f
}

// A slow download that keeps sending takes longer than the stall limit in all
// and is installed.
func TestUpdateSlowButSteady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := startTrickle(t, 150*time.Millisecond)
		start := time.Now()
		noErr(t, f.u.Check(t.Context(), start))
		checkImage(t, f.u.Binary, newImage)
		if took := time.Since(start); took < 3*stallTimeout {
			t.Fatalf("the download took %v, not longer than the stall limit in all", took)
		}
	})
}

// The wait for the next byte is what the stall limit measures: a reader that
// takes longer than the limit between two reads (a slow disk) loses nothing.
func TestUpdateSlowReaderIsNotAStall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := startTrickle(t, time.Millisecond)
		body, err := f.u.get(t.Context(), f.name)
		noErr(t, err)
		defer body.Close()
		time.Sleep(2 * stallTimeout)
		got, err := io.ReadAll(iotest.OneByteReader(slowReader{body}))
		noErr(t, err)
		checkUpdateValue(t, string(got), newImage)
	})
}

// slowReader keeps the reader busy for longer than the stall limit after each read.
type slowReader struct{ io.Reader }

func (r slowReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	time.Sleep(2 * stallTimeout)
	return n, err
}

// A gap longer than the stall limit between two bytes is a stall.
func TestUpdateStallsBetweenBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := startTrickle(t, 250*time.Millisecond)
		if err := f.u.Check(t.Context(), time.Now()); !errors.Is(err, errStalled) {
			t.Fatalf("bytes further apart than the limit: %v", err)
		}
		checkImage(t, f.u.Binary, "old binary")
		checkFolder(t, filepath.Dir(f.u.Binary), "solodawn")
	})
}

func (f *updateFixture) serveBinary(mux *http.ServeMux, binary http.HandlerFunc) {
	mux.HandleFunc("/download/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, f.manifest) })
	mux.HandleFunc("/download/"+f.name, binary)
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
	// The new image cannot take the binary's place: the binary is put back.
	if err := installRelease(binary, binary, "windows"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed second rename: %v", err)
	}
	checkImage(t, binary, "old")
	checkFolder(t, dir, "solodawn.exe", "stage")
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
