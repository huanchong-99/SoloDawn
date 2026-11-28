package node

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const darwinServiceLabel = "org.solodawn.node"

const (
	agentGoneTick = 50 * time.Millisecond
	agentGoneWait = 10 * time.Second
)

var (
	runLaunchctl = execLaunchctl
	getUID       = os.Getuid
	launchctlBin = "/bin/launchctl"
	agentPause   = time.Sleep
)

func agentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", darwinServiceLabel+".plist"), nil
}

// InstallService registers the absolute executable path for the next user
// login, with path as its PATH: launchd gives an agent only the system's
// directories, so the node would not find the CLIs.
func InstallService(executable, path string) error {
	_, err := writeAgentPlist(executable, path)
	return err
}

func writeAgentPlist(executable, path string) (string, error) {
	agent, err := agentPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(agent), 0700); err != nil {
		return "", err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>` + darwinServiceLabel + `</string>
  <key>ProgramArguments</key><array><string>` + html.EscapeString(executable) + `</string><string>node</string></array>
  <key>AssociatedBundleIdentifiers</key><string>` + darwinServiceLabel + `</string>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>` + html.EscapeString(path) + `</string></dict>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`
	if err := os.WriteFile(agent, []byte(plist), 0600); err != nil {
		return "", err
	}
	return agent, nil
}

func execLaunchctl(args ...string) error {
	out, err := exec.Command(launchctlBin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %w (%s)", args[0], err, strings.Join(strings.Fields(string(out)), " "))
	}
	return nil
}

// startAgent loads plist in place of the agent that is loaded. bootout
// answers while the old node is still stopping, and bootstrap fails with an
// I/O error until launchd has removed that service (macOS 15 takes
// bootout --wait as a usage error).
func startAgent(plist string) error {
	domain := "gui/" + strconv.Itoa(getUID())
	service := domain + "/" + darwinServiceLabel
	if runLaunchctl("bootout", service) == nil {
		if err := awaitRemoval(service); err != nil {
			return err
		}
	}
	return runLaunchctl("bootstrap", domain, plist)
}

// awaitRemoval asks launchd every agentGoneTick whether it still has service,
// for agentGoneWait at most.
func awaitRemoval(service string) error {
	for waited := time.Duration(0); runLaunchctl("print", service) == nil; waited += agentGoneTick {
		if waited >= agentGoneWait {
			return fmt.Errorf("launchd did not remove %s within %s of bootout", service, agentGoneWait)
		}
		agentPause(agentGoneTick)
	}
	return nil
}

// RemoveService removes start-at-login without stopping a running node.
func RemoveService() error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
