//go:build acceptance

package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type s2ForgeTurn struct {
	Edit, Script string
}

func s2ForgeFile(num int) string { return fmt.Sprintf("task-%d.txt", num) }

func s2ForgeTurnPath(root, vendor string, num, round int) string {
	return filepath.Join(root, fmt.Sprintf("watch-%s-%d-%d.json", vendor, num, round))
}

func s2ForgeTurnFixture(node *s2Node, vendor string, num, round int, edit string, calls []m5ToolCallSpec) error {
	script := m5ClaudeScript(calls)
	if vendor == "codex" {
		script = m5CodexScript(fmt.Sprintf("watch-%d-%d", num, round), calls)
	}
	return os.WriteFile(s2ForgeTurnPath(node.Root, vendor, num, round), mustMarshalNoEscape(s2ForgeTurn{Edit: edit, Script: script}), 0600)
}

func s2PrepareForgeTurn(vendor string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	kind := filepath.Base(filepath.Dir(cwd))
	if kind != "wt" && kind != "review" {
		return nil
	}
	part, _, _ := strings.Cut(filepath.Base(cwd), "-")
	num, err := strconv.Atoi(part)
	if err != nil {
		return nil
	}
	return s2LoadForgeTurn(vendor, cwd, num)
}

func s2LoadForgeTurn(vendor, cwd string, num int) error {
	root := os.Getenv("HOME")
	for round := 1; round <= 2; round++ {
		path := s2ForgeTurnPath(root, vendor, num, round)
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		return s2StartForgeTurn(path, cwd, num, round, raw)
	}
	return nil
}

func s2StartForgeTurn(path, cwd string, num, round int, raw []byte) error {
	var turn s2ForgeTurn
	if err := json.Unmarshal(raw, &turn); err != nil {
		return err
	}
	if err := os.Rename(path, path+".running"); err != nil {
		return err
	}
	if turn.Edit != "" {
		if err := s2ForgeCommitEdit(cwd, num, round, turn.Edit); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path+".script", []byte(turn.Script), 0600); err != nil {
		return err
	}
	vendor := strings.Split(filepath.Base(path), "-")[1]
	return os.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", path+".script")
}

func s2ForgeCommitEdit(cwd string, num, round int, edit string) error {
	if round == 2 {
		if err := s2ForgeSyncBranch(cwd); err != nil {
			return err
		}
	}
	_, err := m5GitCommitFiles(cwd, map[string]string{s2ForgeFile(num): edit + "\n"}, edit)
	return err
}

func s2ForgeSyncBranch(cwd string) error {
	branch, err := m5GitOut(cwd, "branch", "--show-current")
	if err != nil {
		return err
	}
	if err := runGitIn(cwd, "fetch", "origin", branch); err != nil {
		return err
	}
	return runGitIn(cwd, "merge", "--ff-only", "FETCH_HEAD")
}

func s2WatchForgeGit(dir, cwd string, args []string) error {
	if len(args) < 4 {
		return nil
	}
	switch args[0] {
	case "watch-push":
		return runGitIn(cwd, "push", "origin", args[1])
	case "watch-open":
		if _, err := os.Stat(filepath.Join(cwd, "marker")); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("forge open found an unexpected worktree marker: %v", err)
		}
	case "watch-merge":
		return s2ForgeMerge(dir, cwd, args[1], args[2], args[3])
	}
	return nil
}

func s2ForgeMerge(dir, cwd, branch, base, commit string) error {
	flag := filepath.Join(dir, "advance-reviewed-head")
	if err := os.Remove(flag); err == nil {
		if err := s2AdvanceForgeHead(cwd, branch, commit); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	head, err := m5GitOut(cwd, "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		return err
	}
	sha, _, _ := strings.Cut(head, "\t")
	if sha != commit {
		return fmt.Errorf("forge request head %s differs from reviewed %s", sha, commit)
	}
	return runGitIn(cwd, "push", "origin", commit+":refs/heads/"+base)
}

func s2AdvanceForgeHead(cwd, branch, commit string) error {
	advanced, err := m5GitOut(cwd, "-c", "user.name=test", "-c", "user.email=test@test.invalid",
		"commit-tree", commit+"^{tree}", "-p", commit, "-m", "Advance the request head after review")
	if err != nil {
		return err
	}
	return runGitIn(cwd, "push", "origin", advanced+":refs/heads/"+branch)
}
