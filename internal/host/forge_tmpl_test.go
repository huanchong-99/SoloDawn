package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func forgeTestTemplates() *proto.ForgeTemplates {
	return &proto.ForgeTemplates{
		Push:   []string{"gh", "push", "{branch}", "{base}", "{commit}"},
		Open:   []string{"gh", "open", "--title={title}", "--summary={summary}", "{tag}"},
		Checks: []string{"gh", "checks", "{branch}", "{base}", "{commit}", "{request}"},
		Merge:  []string{"gh", "merge", "{branch}", "{base}", "{commit}", "{request}"},
	}
}

func TestForgeArgumentsAreSinglePassAndLiteral(t *testing.T) {
	v := forgeValues{branch: "task/4-fix", base: "{title}", commit: "head", request: 7, title: `a "quote"; $(touch marker)`, summary: "checked {commit}"}
	assertForge(t, v.arguments([]string{"forge", "{branch}:{base}:{commit}:{request}", "--title={title}", "--summary={summary}", "{tag}", ""}),
		[]string{"forge", "task/4-fix:{title}:head:7", `--title=a "quote"; $(touch marker)`, "--summary=checked {commit}", "{tag}", ""})
	assertForge(t, v.arguments([]string{}), []string{})
}

func TestForgeProgramResolution(t *testing.T) {
	_, err := forgeProgram("missing-template-program", false)
	assertForge(t, err != nil, true)
	dir := t.TempDir()
	for _, name := range []string{"tool.cmd", "tool.exe"} {
		forgeOK(os.WriteFile(filepath.Join(dir, name), []byte("not executed"), 0700))
	}
	t.Setenv("PATH", dir)
	_, err = forgeProgram("tool.cmd", true)
	assertForge(t, err.Error(), "a forge program must resolve to an .exe file")
	_, err = forgeProgram("tool.exe", true)
	assertForge(t, err, nil)
}

func TestCustomForgeSubmitAndReuse(t *testing.T) {
	f, _, head := forgeRepo(t)
	f.Base, f.Templates = "base", forgeTestTemplates()
	calls := fakeForgeGH(t, forgeAnswer{}, forgeAnswer{Stdout: "progress\n https://forge.invalid/requests/7?view=1 \nfinished\n"}, forgeAnswer{})
	pr, err := f.Submit(t.Context(), head, "base", "Title {commit}; $()", "Ready")
	assertForge(t, err, nil)
	assertForge(t, pr, &ForgePullRequest{Number: 7, URL: "https://forge.invalid/requests/7?view=1"})
	f.Request = *pr
	got, err := f.Submit(t.Context(), head, "base", "ignored", "ignored")
	assertForge(t, err, nil)
	assertForge(t, got, pr)
	calls([]string{"push", f.Branch, "base", head}, []string{"open", "--title=Title {commit}; $()", "--summary=Ready", "{tag}"}, []string{"push", f.Branch, "base", head})
}

func TestCustomForgePushAndOpenErrors(t *testing.T) {
	f, _, head := forgeRepo(t)
	f.Base, f.Templates = "base", forgeTestTemplates()
	calls := fakeForgeGH(t, forgeAnswer{Stdout: "push failed", Code: 2}, forgeAnswer{Stderr: "open failed", Code: 3})
	assertForge(t, strings.Contains(f.PushBranch(t.Context()).Error(), "push failed"), true)
	_, err := f.openTemplate(t.Context(), head, "T", "S")
	assertForge(t, strings.Contains(err.Error(), "open failed"), true)
	calls([]string{"push", f.Branch, "base", head}, []string{"open", "--title=T", "--summary=S", "{tag}"})
	f.Worktree = t.TempDir()
	assertForge(t, f.PushBranch(t.Context()) != nil, true)
	_, _, err = runForgeTemplate(t.Context(), f.Worktree, []string{"missing-template-program"}, forgeValues{})
	assertForge(t, err != nil, true)
}

func TestTemplateRequestURL(t *testing.T) {
	for _, output := range []string{"", "not a URL", "https:///42", "https://forge.invalid/not-a-number", "https://%invalid/42"} {
		_, err := templateRequest(output)
		assertForge(t, err != nil, true)
	}
	pr, err := templateRequest("https://forge.invalid/1\nlog\nhttp://forge.invalid/42\n")
	assertForge(t, err, nil)
	assertForge(t, pr, &ForgePullRequest{Number: 42, URL: "http://forge.invalid/42"})
}

func TestCustomForgeChecksPendingPassFailureAndTimeout(t *testing.T) {
	f := TaskForge{Worktree: t.TempDir(), Branch: "task/4-fix", Base: "base", Templates: forgeTestTemplates()}
	calls := fakeForgeGH(t, forgeAnswer{Code: 8}, forgeAnswer{}, forgeAnswer{Code: 2, Stdout: "bad check"}, forgeAnswer{Code: 8})
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		body, err := f.WaitHostedChecks(t.Context(), 7, "head", start)
		assertForge(t, err, nil)
		assertForge(t, body, &proto.ChecksBody{Stage: "cloud", OK: true, Commit: "head", Items: []proto.CheckItem{{Name: "checks", OK: true}}})
		assertForge(t, time.Since(start), 30*time.Second)
	})
	body, err := f.WaitHostedChecks(t.Context(), 7, "head", time.Now())
	assertForge(t, err, nil)
	assertForge(t, body, &proto.ChecksBody{Stage: "cloud", Commit: "head", Items: []proto.CheckItem{{Name: "checks", Detail: new("exit status 2\nbad check")}}})
	body, err = f.WaitHostedChecks(t.Context(), 7, "head", time.Time{})
	assertForge(t, err, nil)
	assertForge(t, body.OK, false)
	assertForge(t, *body.Items[0].Detail, "hosted checks timed out")
	args := []string{"checks", f.Branch, "base", "head", "7"}
	calls(args, args, args, args)
}

func TestCustomForgeChecksCancellation(t *testing.T) {
	f := TaskForge{Worktree: t.TempDir(), Templates: forgeTestTemplates()}
	fakeForgeGH(t, forgeAnswer{Code: 8})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)
		_, err := f.WaitHostedChecks(ctx, 1, "head", time.Now())
		assertForge(t, err, context.Canceled)
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := f.WaitHostedChecks(ctx, 1, "head", time.Now())
	assertForge(t, err, context.Canceled)
}

func TestCustomForgePollingUsesRepositoryState(t *testing.T) {
	h, root := tskDiskHost(t)
	grp := tskGroupFixture("custom")
	grp.Templates = forgeTestTemplates()
	tskPutRow(t, h, "group", tskTestGroupID, grp)
	repo := filepath.Join(root, "projects", tskTestGroupID, "repo")
	forgeOK(os.MkdirAll(repo, 0700))
	home := newTskScriptHome()
	h.Attach("home-device", home)
	calls := fakeForgeGH(t, forgeAnswer{})
	startHostedCheck(h, "home-device", tskTestGroupID, HostedCheck{Task: tskTestTaskID, Branch: "task/12-fix-path", Commit: "head", PR: 7, Since: time.Now()})
	asked := home.waitAsked(t, 1)
	cloud := tskOpenSignedCmd[proto.TaskChecked](t, h, asked[0])
	assertForge(t, cloud.Commit, "head")
	assertForge(t, cloud.Items, []proto.CheckItem{{Name: "checks", OK: true}})
	calls([]string{"checks", "task/12-fix-path", grp.Base, "head", "7"})
}

func TestCustomForgeChecksRedactProgramErrors(t *testing.T) {
	f := TaskForge{Worktree: t.TempDir(), Templates: forgeTestTemplates()}
	f.Templates.Checks[0] = joinTestSecret
	body, pending := f.readTemplateChecks(t.Context(), 7, "head")
	assertForge(t, pending, false)
	assertForge(t, body.OK, false)
	assertForge(t, strings.Contains(*body.Items[0].Detail, joinTestSecret), false)
	assertForge(t, strings.Contains(*body.Items[0].Detail, "[redacted]"), true)
}

func TestCustomForgeMergeAndRestart(t *testing.T) {
	f, _, head := forgeRepo(t)
	m := ReviewedMerge{Repo: f.Worktree, Base: "base", Branch: f.Branch, Commit: head, PR: 7, Templates: forgeTestTemplates()}
	calls := fakeForgeGH(t, forgeAnswer{}, forgeAnswer{Code: 2, Stderr: "head changed"}, forgeAnswer{})
	result, err := m.Apply(t.Context())
	assertForge(t, err, nil)
	assertForge(t, result, MergeResult{MergeCommit: forgeGit(t, f.Worktree, "rev-parse", "origin/base")})
	result, err = m.Apply(t.Context())
	assertForge(t, err, nil)
	assertForge(t, strings.Contains(result.Conflict, "head changed"), true)
	result, err = m.Inspect(t.Context())
	assertForge(t, err, nil)
	assertForge(t, result, MergeResult{Conflict: "custom forge merge was interrupted; inspect the request before retrying"})
	forgeGit(t, f.Worktree, "remote", "remove", "origin")
	_, err = m.Apply(t.Context())
	assertForge(t, err != nil, true)
	args := []string{"merge", f.Branch, "base", head, "7"}
	calls(args, args, args)
}
