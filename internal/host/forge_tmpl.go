package host

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type forgeValues struct {
	branch, base, commit, title, summary string
	request                              int
}

func (v forgeValues) arguments(args []string) []string {
	r := strings.NewReplacer("{branch}", v.branch, "{base}", v.base, "{commit}", v.commit,
		"{request}", strconv.Itoa(v.request), "{title}", v.title, "{summary}", v.summary)
	filled := make([]string, len(args))
	for i, arg := range args {
		filled[i] = r.Replace(arg)
	}
	return filled
}

func forgeProgram(name string, windows bool) (string, error) {
	program, err := exec.LookPath(name)
	if err == nil && windows && !strings.EqualFold(filepath.Ext(program), ".exe") {
		err = errors.New("a forge program must resolve to an .exe file")
	}
	return program, err
}

func runForgeTemplate(ctx context.Context, folder string, args []string, v forgeValues) (string, string, error) {
	program, err := forgeProgram(args[0], runtime.GOOS == "windows")
	if err != nil {
		return "", "", err
	}
	filled := v.arguments(args)
	return runIn(ctx, folder, program, filled[1:]...)
}

func forgeTemplateError(out, stderr string, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out+stderr))
	}
	return nil
}

func (f TaskForge) pushTemplate(ctx context.Context) error {
	commit, err := f.command(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	out, stderr, err := runForgeTemplate(ctx, f.Worktree, f.Templates.Push, forgeValues{branch: f.Branch, base: f.Base, commit: commit})
	return forgeTemplateError(out, stderr, err)
}

func (f TaskForge) openTemplate(ctx context.Context, commit, title, summary string) (*ForgePullRequest, error) {
	if f.Request.Number != 0 {
		return &f.Request, nil
	}
	v := forgeValues{branch: f.Branch, base: f.Base, commit: commit, title: title, summary: summary}
	out, stderr, err := runForgeTemplate(ctx, f.Worktree, f.Templates.Open, v)
	if err != nil {
		return nil, forgeTemplateError(out, stderr, err)
	}
	return templateRequest(out)
}

func templateRequest(out string) (*ForgePullRequest, error) {
	last := templateURL(out)
	u, err := url.Parse(last)
	if err != nil || u.Host == "" {
		return nil, errors.New("forge open printed no request URL")
	}
	pr, err := pullRequest(u.Path)
	if err != nil {
		return nil, err
	}
	pr.URL = last
	return pr, nil
}

func templateURL(out string) string {
	var last string
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			last = line
		}
	}
	return last
}

func (f TaskForge) waitTemplateChecks(ctx context.Context, pr int, commit string, since time.Time) (*proto.ChecksBody, error) {
	deadline := since.Add(60 * time.Minute)
	for {
		body, pending := f.readTemplateChecks(ctx, pr, commit)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !pending {
			return body, nil
		}
		if !time.Now().Before(deadline) {
			return &proto.ChecksBody{Stage: "cloud", Commit: commit, Items: []proto.CheckItem{
				{Name: "checks", Detail: new("hosted checks timed out")},
			}}, nil
		}
		if err := pollWait(ctx); err != nil {
			return nil, err
		}
	}
}

func (f TaskForge) readTemplateChecks(ctx context.Context, pr int, commit string) (*proto.ChecksBody, bool) {
	v := forgeValues{branch: f.Branch, base: f.Base, commit: commit, request: pr}
	out, stderr, err := runForgeTemplate(ctx, f.Worktree, f.Templates.Checks, v)
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 8 {
		return nil, true
	}
	item := proto.CheckItem{Name: "checks", OK: err == nil}
	if err != nil {
		item.Detail = new(RedactSecrets(*checkDetail(err, out+stderr)))
	}
	return &proto.ChecksBody{Stage: "cloud", OK: item.OK, Commit: commit, Items: []proto.CheckItem{item}}, false
}

func (m ReviewedMerge) mergeTemplate(ctx context.Context) (MergeResult, error) {
	v := forgeValues{branch: m.Branch, base: m.Base, commit: m.Commit, request: m.PR}
	out, stderr, err := runForgeTemplate(ctx, m.Repo, m.Templates.Merge, v)
	if err != nil {
		return MergeResult{Conflict: RedactSecrets(forgeTemplateError(out, stderr, err).Error())}, nil
	}
	lock := cloneLock(m.Repo)
	lock.Lock()
	defer lock.Unlock()
	if _, err := m.run(ctx, "git", "fetch", "origin"); err != nil {
		return MergeResult{}, err
	}
	commit, err := m.run(ctx, "git", "rev-parse", "origin/"+m.Base)
	return MergeResult{MergeCommit: commit}, err
}
