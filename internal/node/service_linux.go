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

// InstallService registers the absolute executable path for the next user login.
func InstallService(executable string) error {
	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// systemd rejects quotes even in an escaped executable name. Pass the path
	// as an argument, never shell source. ':' and '%%' disable unit expansion.
	pathArg := strconv.Quote(strings.ReplaceAll(executable, "%", "%%"))
	unit := "[Unit]\nDescription=SoloDawn node\n\n[Service]\nExecStart=:/bin/sh -c 'exec \"$0\" node' " + pathArg + "\n\n[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(path, []byte(unit), 0600); err != nil {
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
