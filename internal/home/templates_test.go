package home

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func validTemplates() *proto.ForgeTemplates {
	return &proto.ForgeTemplates{
		Push:   []string{"forge", "{branch}", "{base}", "{commit}"},
		Open:   []string{"forge", "--title={title}", "--summary={summary}", "{tag}"},
		Checks: []string{"forge", "{request}"}, Merge: []string{"forge", "{request}"},
	}
}

func TestTemplatesValidation(t *testing.T) {
	for _, forge := range []string{"github", "none"} {
		if templatesCreate(forge, nil) != nil || templatesSet(forge, nil) != nil {
			t.Fatal("built-in forge rejected absent templates")
		}
		if templatesCreate(forge, validTemplates()) == nil || templatesSet(forge, validTemplates()) == nil {
			t.Fatal("built-in forge accepted templates")
		}
	}
	if templatesCreate("custom", nil) == nil || templatesCreate("custom", validTemplates()) != nil || templatesSet("custom", validTemplates()) != nil {
		t.Fatal("custom template presence or validation")
	}
	for _, args := range [][]string{nil, {}, {""}, {"bin/forge"}, {`bin\forge`}, {"forge{branch}"}, {"forge", "{request}"}, {"forge", "--title={title}"}, {"forge", "--summary={summary}"}} {
		tmpl := validTemplates()
		tmpl.Push = args
		if refusal := templatesValid(tmpl); refusal == nil || refusal.Code != "bad_request" {
			t.Fatalf("accepted push template %q", args)
		}
	}
	for _, arg := range []string{"{title}", "{title}suffix", "{summary}", "{summary}suffix", "{request}"} {
		if templateValid("open", []string{"forge", arg}) {
			t.Fatalf("accepted open argument %q", arg)
		}
	}
	if !templateValid("merge", []string{"forge", "{branch}:{base}:{commit}:{request}", "{unknown}"}) {
		t.Fatal("valid merge places or literal braces refused")
	}
}
