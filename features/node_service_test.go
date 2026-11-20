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

	"github.com/cucumber/godog"
)

func init() {
	stepSets["node_service"] = nodeServiceSteps
}

func nodeServiceSteps(sc *godog.ScenarioContext, f fixtures) {
	var profile string
	var installed bool
	// The built binary runs with HOME and XDG_CONFIG_HOME in a temporary
	// profile, and with no node running for it.
	service := func(op string) error {
		cmd := exec.Command(f.bin, "service", op)
		cmd.Env = append(os.Environ(), "HOME="+profile, "XDG_CONFIG_HOME="+profile)
		out, err := cmd.CombinedOutput()
		installed = (op == "install") == (err == nil)
		if err != nil || len(out) != 0 {
			return fmt.Errorf("solodawn service %s: %v, output %q", op, err, out)
		}
		return nil
	}
	sc.Step(`^a clean login registration for the current user$`, func() error {
		profile = f.t.TempDir()
		return checkLoginRegistration(profile, false)
	})
	sc.Step(`^the owner runs "solodawn service (install|remove)" with no node running$`, service)
	sc.Step(`^the current user has (a|no) SoloDawn login registration$`, func(article string) error {
		return checkLoginRegistration(profile, article == "a")
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if installed {
			return ctx, service("remove")
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
