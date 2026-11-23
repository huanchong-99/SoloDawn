//go:build acceptance

package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func TestS3LiveFeature(t *testing.T) {
	if os.Getenv("SOLODAWN_LIVE") != "1" {
		t.Skip("set SOLODAWN_LIVE=1 with the real CLI and its account configured")
	}
	suite := godog.TestSuite{ScenarioInitializer: func(sc *godog.ScenarioContext) { s3LiveSteps(sc, t) },
		Options: &godog.Options{Format: "pretty", Paths: []string{"s3_drivers.feature"}, Tags: "@live && ~@pending", Strict: true, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("S3 live scenarios failed")
	}
}

func s3LiveSteps(sc *godog.ScenarioContext, t *testing.T) {
	cleanup := afterScenario(sc)
	sc.Step(`^the named live step-1 case A3\.8 runs for "([^"]+)"$`, func(agent string) error { return s3LiveRepoConfig(t, cleanup, agent) })
}

func s3LiveRepoConfig(t *testing.T, cleanup func(func()), agent string) error {
	dir, err := shortTempRoot(t, cleanup, "s3live")
	if err != nil {
		return err
	}
	if err := initLocalGitRepo(dir); err != nil {
		return err
	}
	marker := filepath.Join(dir, "hook-ran")
	if err := s3WriteHook(dir, marker, agent); err != nil {
		return err
	}
	for _, repoConfig := range []bool{false, true, true} {
		if err := s3LiveStart(agent, dir, marker, repoConfig); err != nil {
			return err
		}
	}
	return nil
}

func s3WriteHook(dir, marker, agent string) error {
	path := filepath.Join(dir, ".opencode", "plugins")
	text := fmt.Sprintf("import { writeFileSync } from 'node:fs';\nexport default async () => { writeFileSync(%q, 'hook'); return {}; };\n", filepath.ToSlash(marker))
	name := "acceptance.ts"
	if agent == "qwen" {
		path, name = filepath.Join(dir, ".qwen"), "settings.json"
		text = fmt.Sprintf(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":%q}]}]}}`, "node -e \"require('fs').writeFileSync('"+filepath.ToSlash(marker)+"','hook')\"")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, name), []byte(text), 0600)
}

func s3LiveStart(agent, dir, marker string, repoConfig bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	p, err := host.Drivers[agent].Start(ctx, host.Launch{Folder: dir, Level: 1, RepoConfig: repoConfig, Shim: []string{"solodawn", "mcp"}})
	if err != nil {
		return err
	}
	defer p.Close()
	if err := s3AwaitReady(ctx, agent, p.Events()); err != nil {
		return err
	}
	return s3CheckHook(marker, repoConfig)
}

func s3AwaitReady(ctx context.Context, agent string, events <-chan host.Event) error {
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return fmt.Errorf("%s exited before ready", agent)
			}
			if event.Kind == host.Ready {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func s3CheckHook(marker string, want bool) error {
	data, err := os.ReadFile(marker)
	if !want && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !want || string(data) != "hook" {
		return fmt.Errorf("hook marker = %q, want present=%v", data, want)
	}
	return os.Remove(marker)
}
