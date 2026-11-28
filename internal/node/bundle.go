package node

import (
	"path/filepath"
	"strconv"
	"strings"
)

// bundleOf returns the <name>.app root when exe sits at
// <name>.app/Contents/MacOS/<binary>, and "" otherwise.
func bundleOf(exe string) string {
	clean := filepath.Clean(exe)
	macOS := filepath.Dir(clean)
	contents := filepath.Dir(macOS)
	app := filepath.Dir(contents)
	if filepath.Base(macOS) != "MacOS" || filepath.Base(contents) != "Contents" {
		return ""
	}
	base := filepath.Base(app)
	if !strings.HasSuffix(base, ".app") {
		return ""
	}
	return app
}

// IsBundled reports whether the current executable lives inside a macOS .app bundle.
func IsBundled() bool {
	exe, err := executablePath()
	return err == nil && bundleOf(exe) != ""
}

// Launch starts the background node if none answers on the local socket and
// opens the local web page in the browser.
func Launch() error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	conn, err := dialSocket(dir, true, false)
	if err != nil {
		return err
	}
	conn.Close()
	return openURL("http://localhost:" + strconv.Itoa(defaultWebPort))
}
