//go:build acceptance

package features

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func init() {
	stepSets["node"] = nodeFeatureSteps
}

type runningNodeProc struct {
	root   string
	sock   string
	env    []string
	cmd    *exec.Cmd
	output bytes.Buffer
}

func nodeFeatureSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	var nodes []*runningNodeProc
	var statuses []string

	sc.Step(`^two foreground nodes running in separate home directories on this machine$`, func() error {
		nodes = nil
		statuses = nil
		for range 2 {
			np, err := startAcceptanceNode(f.bin, cleanup)
			if err != nil {
				return err
			}
			nodes = append(nodes, np)
		}
		return nil
	})

	sc.Step(`^the owner runs "solodawn status" in each node's environment$`, func() error {
		for _, np := range nodes {
			cmd := exec.Command(f.bin, "status")
			cmd.Env = np.env
			var out, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &stderr
			if err := cmd.Run(); err != nil || stderr.Len() != 0 {
				return fmt.Errorf("status failed (%v): stdout=%q stderr=%q node=%q", err, out.String(), stderr.String(), np.output.String())
			}
			statuses = append(statuses, out.String())
		}
		return nil
	})

	sc.Step(`^each status prints its node's version and distinct device fingerprint$`, func() error {
		if len(statuses) != 2 {
			return fmt.Errorf("got %d status outputs, want 2", len(statuses))
		}
		var fps []string
		for i, out := range statuses {
			if !strings.Contains(out, "version dev\n") {
				return fmt.Errorf("node %d status missing version: %q", i, out)
			}
			fp := extractFingerprintLine(out)
			if fp == "" {
				return fmt.Errorf("node %d status missing fingerprint: %q", i, out)
			}
			fps = append(fps, fp)
		}
		if fps[0] == fps[1] {
			return fmt.Errorf("both nodes reported the same fingerprint %q", fps[0])
		}
		return nil
	})
}

func startAcceptanceNode(bin string, cleanup func(func())) (*runningNodeProc, error) {
	base, rel := os.TempDir(), "solodawn"
	if runtime.GOOS == "darwin" {
		base, rel = "/tmp", filepath.Join("Library", "Application Support", "solodawn")
	}
	root, err := os.MkdirTemp(base, "sdn")
	if err != nil {
		return nil, err
	}
	cleanup(func() { os.RemoveAll(root) })
	sock := filepath.Join(root, rel, "sock")
	env := append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "LocalAppData="+root)
	np := &runningNodeProc{root: root, sock: sock, env: env}
	cmd := exec.Command(bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.output, &np.output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	np.cmd = cmd
	cleanup(func() {
		if cmd.Process != nil {
			done := make(chan struct{})
			go func() { cmd.Wait(); close(done) }()
			if cmd.Process.Signal(os.Interrupt) != nil {
				cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				cmd.Process.Kill()
				<-done
			}
		}
	})
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return nil, fmt.Errorf("node did not bind %s: %w (%s)", sock, err, np.output.String())
	}
	return np, nil
}

func waitAcceptanceSocket(sock string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			conn.Close()
			return nil
		}
		last = err
		time.Sleep(25 * time.Millisecond)
	}
	return last
}

func extractFingerprintLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if fp, ok := strings.CutPrefix(line, "fingerprint "); ok && fp != "" {
			return fp
		}
	}
	return ""
}
