package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const unitName = "solodawn.service"

func unitPath() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "systemd", "user", unitName), nil
}

// InstallService registers the absolute executable path for the next user
// login, with path as its PATH: the user manager's own PATH lacks the
// directories where the CLIs usually are.
func InstallService(executable, path string) error {
	unit, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unit), 0700); err != nil {
		return err
	}
	// systemd rejects quotes even in an escaped executable name, so the path
	// is an argument of sh, never shell source. In ExecStart, "%%" and "$$"
	// are a literal % and $; Environment expands specifiers only.
	execArg := strings.NewReplacer("%", "%%", "$", "$$").Replace(executable)
	text := "[Unit]\nDescription=SoloDawn node\n\n[Service]\n" +
		"Environment=" + strconv.Quote("PATH="+strings.ReplaceAll(path, "%", "%%")) + "\n" +
		"ExecStart=/bin/sh -c 'exec \"$$0\" node' " + strconv.Quote(execArg) + "\n\n[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(unit, []byte(text), 0600); err != nil {
		return err
	}
	return setUnitEnabled("enable")
}

// RemoveService removes start-at-login without stopping a running node.
func RemoveService() error {
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := setUnitEnabled("disable"); err != nil {
		return err
	}
	return os.Remove(path)
}

func setUnitEnabled(operation string) error {
	// With --root the links are edited on disk, not via the running user bus.
	out, err := exec.Command("systemctl", "--user", "--root=/", operation, unitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w (%s)", operation, err, strings.Join(strings.Fields(string(out)), " "))
	}
	return nil
}
