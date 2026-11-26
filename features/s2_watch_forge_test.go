//go:build acceptance

package features

import "github.com/cucumber/godog"

// s2RegisterForge keeps the steps of A9.24 and A9.25, which plan 3.3 of
// MODEL-1126 removes with custom forge templates.
func s2RegisterForge(sc *godog.ScenarioContext, _ *s2WatchWorld) {
	gone := rewritten("AC-3", "custom forge templates are removed; A9.24 and A9.25 go (MODEL-1126 plan 3.3)")
	for _, step := range []string{
		"^a project uses the fake forge templates$",
		"^the secretary proposes two tasks for cross-review and P confirms the brief$",
		"^the stale reviewed head is refused and the secretary reassigns the task$",
		"^both tasks merge after checks and cross-review and receive summaries$",
		"^the forge records the filled template arguments and gh never runs$",
		"^the project refuses a push template containing the title place$",
		"^P confirms a brief with a title containing spaces, quotes and shell punctuation$",
		"^the forge receives the title as one argument and the tag as a literal without creating a marker$",
	} {
		sc.Step(step, gone)
	}
}
