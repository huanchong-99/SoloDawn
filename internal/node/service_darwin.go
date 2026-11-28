package node

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const darwinServiceLabel = "org.solodawn.node"

var (
	runLaunchctl = execLaunchctl
	getUID       = os.Getuid
	launchctlBin = "/bin/launchctl"
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

func startAgent(plist string) error {
	domain := "gui/" + strconv.Itoa(getUID())
	_ = runLaunchctl("bootout", domain+"/"+darwinServiceLabel)
	return runLaunchctl("bootstrap", domain, plist)
}

// RemoveService removes start-at-login without stopping a running node.
func RemoveService() error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
