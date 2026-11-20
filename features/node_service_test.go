//go:build acceptance

package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/node"
)

func init() {
	stepSets["node_service"] = nodeServiceSteps
}

func nodeServiceSteps(sc *godog.ScenarioContext, f fixtures) {
	var profile string
	var installed bool
	// Scope HOME and XDG_CONFIG_HOME to each operation, not to the whole godog suite.
	inProfile := func(operation func() error) error {
		var err error
		f.t.Run("login profile", func(t *testing.T) {
			t.Setenv("HOME", profile)
			t.Setenv("XDG_CONFIG_HOME", profile)
			err = operation()
		})
		return err
	}
	sc.Step(`^a clean login registration for the current user$`, func() error {
		profile = f.t.TempDir()
		return checkLoginRegistration(profile, false)
	})
	sc.Step(`^the node installs its login service$`, func() error {
		err := inProfile(func() error { return node.InstallService(f.bin) })
		installed = err == nil
		return err
	})
	remove := func() error {
		err := inProfile(node.RemoveService)
		installed = err != nil
		return err
	}
	sc.Step(`^the node removes its login service$`, remove)
	sc.Step(`^the current user has (a|no) SoloDawn login registration$`, func(article string) error {
		return checkLoginRegistration(profile, article == "a")
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if installed {
			return ctx, remove()
		}
		return ctx, nil
	})
}

func checkLoginRegistration(profile string, want bool) error {
	got, err := loginRegistration(profile)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("current user's login registration: got %t, want %t", got, want)
	}
	return nil
}

func loginRegistration(profile string) (bool, error) {
	switch runtime.GOOS {
	case "linux":
		return loginFile(filepath.Join(profile, "systemd", "user", "default.target.wants", "solodawn.service"))
	case "darwin":
		return loginFile(filepath.Join(profile, "Library", "LaunchAgents", "org.solodawn.node.plist"))
	default: // windows
		// Never replace a real user's pre-existing task; the background checks absence first.
		out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference = 'Stop'
$name = 'SoloDawn-' + [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
Get-ScheduledTask -TaskPath '\' | Where-Object TaskName -eq $name | Select-Object -ExpandProperty TaskName`).Output()
		return strings.TrimSpace(string(out)) != "", err
	}
}

func loginFile(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
