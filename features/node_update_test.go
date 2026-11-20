//go:build acceptance

package features

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/node"
)

func init() {
	stepSets["node_update"] = nodeUpdateSteps
}

func nodeUpdateSteps(sc *godog.ScenarioContext, f fixtures) {
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	release := func(tag string) []byte { return stampedRelease(f, tag) }
	var u node.Updater
	var state string
	var restarts int
	sc.Step(`^the node runs release "([^"]+)"$`, func(tag string) error {
		state, restarts = "idle", 0
		u = node.Updater{
			Version: tag,
			Binary:  filepath.Join(f.t.TempDir(), "solodawn"+exe),
			Busy:    func() bool { return state != "idle" },
			Restart: func() error { restarts++; return nil },
		}
		return os.WriteFile(u.Binary, release(tag), 0o755)
	})
	sc.Step(`^the release server offers "([^"]+)" with a (matching|mismatching) checksum$`, func(tag, checksum string) {
		image := release(tag)
		sum := sha256.Sum256(image)
		if checksum == "mismatching" {
			image = image[:len(image)-1]
		}
		name := "solodawn_" + tag + "_" + runtime.GOOS + "_" + runtime.GOARCH + exe
		mux := http.NewServeMux()
		mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "%x  %s\n", sum, name) })
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, _ *http.Request) { w.Write(image) })
		server := httptest.NewServer(mux)
		f.t.Cleanup(server.Close)
		u.ReleaseURL = server.URL
	})
	sc.Step(`^a session is (working|takeover)$`, func(s string) { state = s })
	sc.Step(`^the session becomes idle$`, func() { state = "idle" })
	sc.Step(`^the node checks for an update$`, func() error { return u.Check(f.t.Context(), time.Now()) })
	sc.Step(`^the node applies its update$`, func() error { return u.Apply() })
	sc.Step(`^the installed binary prints version "([^"]+)"$`, func(tag string) error {
		out, err := exec.Command(u.Binary, "version").Output()
		if err != nil || string(out) != tag+"\n" {
			return fmt.Errorf("installed binary printed %q, %v", out, err)
		}
		return nil
	})
	sc.Step(`^the update status is "([^"]+)" for "([^"]+)"$`, func(status, tag string) error {
		if u.Status == nil || *u.Status != (node.UpdateStatus{Version: tag, Status: status}) {
			return fmt.Errorf("update status is %+v", u.Status)
		}
		return nil
	})
	sc.Step(`^there is no update status$`, func() error {
		if u.Status != nil {
			return fmt.Errorf("update status is %+v", *u.Status)
		}
		return nil
	})
	sc.Step(`^the node has requested (\d+) restarts?$`, func(n int) error {
		if restarts != n {
			return fmt.Errorf("node requested %d restarts", restarts)
		}
		return nil
	})
}

// stampedBuilds holds one build of solodawn per tag for the whole suite.
var stampedBuilds = map[string]string{}

// stampedRelease builds solodawn stamped with tag, as the release build does.
func stampedRelease(f fixtures, tag string) []byte {
	bin, ok := stampedBuilds[tag]
	if !ok {
		bin = filepath.Join(f.t.TempDir(), tag)
		out, err := exec.Command("go", "build", "-o", bin, "-ldflags", "-X github.com/huanchong-99/SoloDawn/internal/node.Version="+tag, "../cmd/solodawn").CombinedOutput()
		if err != nil {
			f.t.Fatalf("go build %s: %v\n%s", tag, err, out)
		}
		stampedBuilds[tag] = bin
	}
	image, err := os.ReadFile(bin)
	if err != nil {
		f.t.Fatal(err)
	}
	return image
}
