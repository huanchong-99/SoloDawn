package node

import (
	"html"
	"os"
	"path/filepath"
)

func agentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "org.solodawn.node.plist"), nil
}

// InstallService registers the absolute executable path for the next user
// login, with path as its PATH: launchd gives an agent only the system's
// directories, so the node would not find the CLIs.
func InstallService(executable, path string) error {
	agent, err := agentPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(agent), 0700); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>` + html.EscapeString(executable) + `</string><string>node</string></array>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>` + html.EscapeString(path) + `</string></dict>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`
	return os.WriteFile(agent, []byte(plist), 0600)
}

// RemoveService removes start-at-login without stopping a running node.
func RemoveService() error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
