//go:build acceptance

package features

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/node"
)

func init() {
	stepSets["cmd_local"] = localCommandSteps
}

func localCommandSteps(sc *godog.ScenarioContext, f fixtures) {
	var root string
	var stdout, stderr bytes.Buffer
	fingerprint := exampleKey("node").Fingerprint()
	cleanup := afterScenario(sc)
	sc.Step(`^a local node socket that reports its device fingerprint$`, func() error {
		var err error
		root, err = shortTempRoot(f.t, cleanup, "sd")
		if err != nil {
			return err
		}
		data := ""
		if runtime.GOOS == "darwin" {
			data = filepath.Join("Library", "Application Support")
		}
		data = filepath.Join(root, data)
		l, err := node.ListenLocal(filepath.Join(data, "solodawn"))
		if err != nil {
			return err
		}
		registerBothCleanups(f.t, cleanup, func() { _ = l.Close() })
		go answerStatus(l, fingerprint)
		return nil
	})
	sc.Step(`^the owner asks solodawn for that node's status$`, func() error {
		cmd := exec.Command(f.bin, "status")
		cmd.Env = append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "LocalAppData="+root)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		return cmd.Run()
	})
	sc.Step(`^the command prints the node's fingerprint and succeeds$`, func() error {
		if got := stdout.String(); got != fingerprint+"\n" || stderr.Len() != 0 {
			return fmt.Errorf("status stdout %q, stderr %q", got, stderr.String())
		}
		return nil
	})
}

func answerStatus(l net.Listener, fingerprint string) {
	conn, err := l.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	if line, _ := bufio.NewReader(conn).ReadString('\n'); line == "{\"cmd\":\"status\"}\n" {
		fmt.Fprintf(conn, "{\"text\":%q}\n{\"done\":true,\"ok\":true}\n", fingerprint)
	}
}
