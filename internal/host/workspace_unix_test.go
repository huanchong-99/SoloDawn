//go:build !windows

package host

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// slowCheckout commits slow.txt with a smudge filter that signals on a FIFO
// and then hangs, so a checkout of it stops at a known point.
func slowCheckout(t *testing.T, f workspaceFixture) (ready string) {
	t.Helper()
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	workOK(t, os.WriteFile(filepath.Join(f.repo, ".gitattributes"), []byte("slow.txt filter=slow\n"), 0o644))
	workOK(t, os.WriteFile(filepath.Join(f.repo, "slow.txt"), []byte("data\n"), 0o644))
	fixtureGit(t, f.repo, "add", ".gitattributes", "slow.txt")
	fixtureGit(t, f.repo, "commit", "-m", "slow file")
	dir := t.TempDir()
	ready = filepath.Join(dir, "ready")
	workOK(t, syscall.Mkfifo(ready, 0o600))
	filter := filepath.Join(dir, "smudge")
	workOK(t, os.WriteFile(filter, []byte("#!/bin/sh\necho ready > '"+ready+"'\nexec sleep 30\n"), 0o755))
	fixtureGit(t, f.repo, "config", "filter.slow.smudge", filter)
	return ready
}

// killAdd starts CreateTaskWorkspace and stops it, with SIGKILL through the
// command runner, while git is checking out slow.txt.
func killAdd(t *testing.T, f workspaceFixture, ready string, num int) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := CreateTaskWorkspace(ctx, f.project, num, "x", aiOne, "trunk", false)
		done <- err
	}()
	_, err := os.ReadFile(ready)
	workOK(t, err)
	cancel()
	err = <-done
	workEqual(t, strings.HasPrefix(err.Error(), "git: signal: killed: Preparing worktree (new branch 'task/"), true)
}

// M3 (a): a SIGKILL of `worktree add -b` during checkout.
func TestWorkspaceAfterAKilledAdd(t *testing.T) {
	f := workspaceSetup(t)
	ready := slowCheckout(t, f)
	killAdd(t, f, ready, 1)
	tree := filepath.Join(f.project, "wt", "1-x")
	entries, err := os.ReadDir(tree)
	workOK(t, err)
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	workEqual(t, strings.Join(names, " "), ".git .gitattributes")
	workEqual(t, strings.Contains(worktreeList(t, f.repo), "\nbranch refs/heads/task/1-x\nlocked initializing\n"), true)
	fixtureGit(t, f.repo, "config", "filter.slow.smudge", "cat")
	again, err := CreateTaskWorkspace(t.Context(), f.project, 1, "x", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, again, "status", "--porcelain", "--untracked-files=all"), "")
	requireFile(t, filepath.Join(again, "slow.txt"), "data\n")
	workEqual(t, holderOf(t, again), aiOne)
	workEqual(t, strings.Contains(worktreeList(t, f.repo), "locked"), false)
	// Removing the state that the kill leaves, without a create in between.
	fixtureGit(t, f.repo, "config", "filter.slow.smudge", filepath.Join(filepath.Dir(ready), "smudge"))
	killAdd(t, f, ready, 2)
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 2, "x", aiOne, TaskCancelled))
	requireAbsent(t, filepath.Join(f.project, "wt", "2-x"))
}

// M3 (b): `worktree add -b` failed after it created the branch.
func TestWorkspaceAfterAnAddThatFailedInCheckout(t *testing.T) {
	f := workspaceSetup(t)
	slowCheckout(t, f)
	fixtureGit(t, f.repo, "config", "filter.slow.smudge", "false")
	fixtureGit(t, f.repo, "config", "filter.slow.required", "true")
	_, err := CreateTaskWorkspace(t.Context(), f.project, 1, "x", aiOne, "trunk", false)
	requireGitExit(t, err, 128)
	workEqual(t, fixtureGit(t, f.repo, "branch", "--list", "task/1-x"), "  task/1-x")
	fixtureGit(t, f.repo, "config", "filter.slow.smudge", "cat")
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 1, "x", aiOne, "trunk", false)
	workOK(t, err)
	requireFile(t, filepath.Join(tree, "slow.txt"), "data\n")
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/1-x")
}

// A temporary clone folder that cannot be removed stops the clone.
func TestWorkspaceCloneTempThatCannotBeRemoved(t *testing.T) {
	f := workspaceSetup(t)
	project := filepath.Join(t.TempDir(), "group")
	temp := filepath.Join(project, cloneTemp)
	workOK(t, os.MkdirAll(temp, 0o755))
	workOK(t, os.WriteFile(filepath.Join(temp, "partial"), nil, 0o644))
	workOK(t, os.Chmod(temp, 0o555))
	t.Cleanup(func() { workOK(t, os.Chmod(temp, 0o755)) })
	_, err := EnsureGroupWorkspace(t.Context(), project, f.upstream(), "trunk")
	workEqual(t, errors.Is(err, fs.ErrPermission), true)
	requireAbsent(t, filepath.Join(project, "repo"))
	entries, err := os.ReadDir(temp)
	workOK(t, err)
	workEqual(t, len(entries), 1)
	workEqual(t, entries[0].Name(), "partial")
}
