package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceLoginAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	plist := filepath.Join(home, "Library", "LaunchAgents", "org.solodawn.node.plist")
	serviceOK(t, InstallService("/opt/old"))
	serviceOK(t, InstallService("/opt/a <node> & b"))
	serviceFile(t, plist, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>/opt/a &lt;node&gt; &amp; b</string><string>node</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`)
	serviceOK(t, RemoveService())
	serviceAbsent(t, plist)
}

func TestServiceAgentPathErrors(t *testing.T) {
	t.Setenv("HOME", "")
	serviceError(t, InstallService("/opt/node"), "$HOME is not defined")
	serviceError(t, RemoveService(), "$HOME is not defined")

	home := t.TempDir()
	t.Setenv("HOME", home)
	file := filepath.Join(home, "Library")
	serviceOK(t, os.WriteFile(file, nil, 0600))
	serviceError(t, InstallService("/opt/node"), "mkdir "+file+": not a directory")
}
