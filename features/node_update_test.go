//go:build acceptance

package features

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/node"
)

func init() {
	stepSets["node_update"] = nodeUpdateSteps
}

func nodeUpdateSteps(sc *godog.ScenarioContext, f fixtures) {
	var u node.Updater
	var state string
	var restarts int
	sc.Step(`^the node has release "([^"]+)" installed$`, func(version string) error {
		state, restarts = "idle", 0
		u = node.Updater{
			Version: version,
			Binary:  filepath.Join(f.t.TempDir(), "solodawn"),
			Busy:    func() bool { return state == "working" || state == "takeover" },
			Restart: func() error { restarts++; return nil },
		}
		return os.WriteFile(u.Binary, []byte("binary "+version), 0755)
	})
	sc.Step(`^the release server offers "([^"]+)" with a (matching|mismatching) checksum$`, func(version, checksum string) {
		u.ReleaseURL = updateServer(f.t, version, checksum)
	})
	sc.Step(`^the node checks for an update$`, func() error {
		err := u.Check(f.t.Context(), time.Now())
		if u.Status != nil && u.Status.Status == "mismatch" {
			return nil // the following steps inspect the recorded mismatch and the binary
		}
		return err
	})
	sc.Step(`^the node applies its verified update$`, u.Apply)
	sc.Step(`^a session is in (working|takeover) when the node applies its verified update$`, func(active string) error {
		state = active
		return u.Apply()
	})
	sc.Step(`^the session becomes idle and the node applies its verified update$`, func() error {
		state = "idle"
		return u.Apply()
	})
	updateAssertions(sc, &u, &restarts)
}

func updateAssertions(sc *godog.ScenarioContext, u *node.Updater, restarts *int) {
	sc.Step(`^the installed binary contains release "([^"]+)"$`, func(version string) error {
		b, err := os.ReadFile(u.Binary)
		if err != nil {
			return err
		}
		if string(b) != "binary "+version {
			return fmt.Errorf("installed binary contains %q", b)
		}
		return nil
	})
	sc.Step(`^the update status is "([^"]+)" for "([^"]+)"$`, func(status, version string) error {
		if u.Status == nil || *u.Status != (node.UpdateStatus{Version: version, Status: status}) {
			return fmt.Errorf("update status is %+v", u.Status)
		}
		return nil
	})
	sc.Step(`^the node has requested (\d+) restarts?$`, func(n int) error {
		if *restarts != n {
			return fmt.Errorf("node requested %d restarts", *restarts)
		}
		return nil
	})
}

func updateServer(t *testing.T, version, checksum string) string {
	name := "solodawn_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	body := "binary " + version
	sum := sha256.Sum256([]byte(body))
	if checksum == "mismatching" {
		body = "corrupt download"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "%x  %s\n", sum, name) })
	mux.HandleFunc("/"+name, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL
}
