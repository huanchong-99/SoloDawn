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

// InstallService registers the absolute executable path for the next user login.
func InstallService(executable string) error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>` + html.EscapeString(executable) + `</string><string>node</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`
	return os.WriteFile(path, []byte(plist), 0600)
}

// RemoveService removes start-at-login without stopping a running node.
func RemoveService() error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
