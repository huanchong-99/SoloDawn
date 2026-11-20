//go:build acceptance

package features

import (
	"fmt"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_secret"] = hostSecretSteps
}

func hostSecretSteps(sc *godog.ScenarioContext, _ fixtures) {
	var result error
	sc.Step(`^the host checks this AI post for secrets:$`, func(text *godog.DocString) {
		result = host.CheckOutboundSecret(text.Content)
	})
	sc.Step(`^the outbound secret check refuses it with "([^"]*)"$`, func(want string) error {
		if result == nil || result.Error() != want {
			return fmt.Errorf("got %v, want %q", result, want)
		}
		return nil
	})
	sc.Step(`^the outbound secret check allows it$`, func() error { return result })
}
