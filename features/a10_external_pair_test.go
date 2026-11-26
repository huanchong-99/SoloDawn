//go:build acceptance

package features

import "github.com/cucumber/godog"

// externalPairSteps keeps the steps of the A10.22 brief run of X and a managed
// AI, which checked worktrees, review checkouts, branches and the merge.
func externalPairSteps(sc *godog.ScenarioContext, _ *externalWorld) {
	gone := rewritten("AC-3", "a task runs in its assignee's workspace, is submitted by report and is done at review: no worktree, review checkout, branch or merge (MODEL-1126 plan 1.5, 1.6, A10.22)")
	for _, step := range []string{
		`^a project with external X, a managed AI that works and reviews, and forge "([^"]+)"$`,
		`^the owner confirms a brief of two tasks that X and the managed AI do and review for each other$`,
		`^X works on its task while the review of the other waits, and X's next run is that review$`,
		`^both external tasks are on the base with two summary cards, and the owner saw no card but the brief$`,
		`^each task and review had a folder of its own, and the folders and branches are gone$`,
	} {
		sc.Step(step, gone)
	}
}
