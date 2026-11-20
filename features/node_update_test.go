//go:build acceptance

package features

import (
	"context"
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
	cleanup := afterScenario(sc)
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
		image, err := stampedRelease(f, tag)
		if err != nil {
			return err
		}
		return os.WriteFile(u.Binary, image, 0o755)
	})
	sc.Step(`^the release server offers "([^"]+)" with a (matching|mismatching) checksum$`, func(tag, checksum string) error {
		image, err := stampedRelease(f, tag)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(image)
		if checksum == "mismatching" {
			image = image[:len(image)-1]
		}
		name := "solodawn_" + tag + "_" + runtime.GOOS + "_" + runtime.GOARCH + exe
		mux := http.NewServeMux()
		mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "%x  %s\n", sum, name) })
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, _ *http.Request) { w.Write(image) })
		server := httptest.NewTLSServer(mux) // the node downloads over https only
		cleanup(server.Close)
		u.ReleaseURL, u.Transport = server.URL, server.Client().Transport
		return nil
	})
	sc.Step(`^a session is (working|takeover)$`, func(s string) { state = s })
	sc.Step(`^the session becomes idle$`, func() { state = "idle" })
	sc.Step(`^the node checks for an update$`, func(ctx context.Context) error { return u.Check(ctx, time.Now()) })
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

// stampedRelease builds solodawn stamped with tag, as the release build does,
// once per tag and run of the suite: the build lives in the suite's TempDir.
func stampedRelease(f fixtures, tag string) ([]byte, error) {
	bin, ok := f.stamped[tag]
	if !ok {
		bin = filepath.Join(f.t.TempDir(), tag)
		out, err := exec.Command("go", "build", "-o", bin, "-ldflags", "-X github.com/huanchong-99/SoloDawn/internal/node.Version="+tag, "../cmd/solodawn").CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("go build %s: %v\n%s", tag, err, out)
		}
		f.stamped[tag] = bin
	}
	return os.ReadFile(bin)
}
