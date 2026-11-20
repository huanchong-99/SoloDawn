package node

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestServiceLoginAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	plist := filepath.Join(home, "Library", "LaunchAgents", "org.solodawn.node.plist")
	serviceOK(t, InstallService("/opt/old", "/usr/bin"))
	serviceOK(t, InstallService("/opt/a <node> & b", "/opt/a&b/bin:/usr/bin"))
	serviceFile(t, plist, `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Label</key><string>org.solodawn.node</string>
  <key>ProgramArguments</key><array><string>/opt/a &lt;node&gt; &amp; b</string><string>node</string></array>
  <key>EnvironmentVariables</key><dict><key>PATH</key><string>/opt/a&amp;b/bin:/usr/bin</string></dict>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`)
	// launchd reads the agent through the same property list parser as plutil.
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plist).Output()
	serviceOK(t, err)
	var got map[string]any
	serviceOK(t, json.Unmarshal(out, &got))
	want := map[string]any{
		"EnvironmentVariables": map[string]any{"PATH": "/opt/a&b/bin:/usr/bin"},
		"Label":                "org.solodawn.node",
		"ProgramArguments":     []any{"/opt/a <node> & b", "node"},
		"RunAtLoad":            true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plutil: got %v; want %v", got, want)
	}
	serviceOK(t, RemoveService())
	serviceAbsent(t, plist)
}

func TestServiceAgentPathErrors(t *testing.T) {
	t.Setenv("HOME", "")
	serviceError(t, InstallService("/opt/node", "/usr/bin"), "$HOME is not defined")
	serviceError(t, RemoveService(), "$HOME is not defined")

	home := t.TempDir()
	t.Setenv("HOME", home)
	file := filepath.Join(home, "Library")
	serviceOK(t, os.WriteFile(file, nil, 0600))
	serviceError(t, InstallService("/opt/node", "/usr/bin"), "mkdir "+file+": not a directory")
}
