package host

import (
	"errors"
	"regexp"
)

var (
	errSecret = errors.New("secret: this looks like a secret; remove it and send again")

	// Each vendor credential is a prefix not preceded by a letter or digit,
	// then at least the run of key characters local.md §8 gives; a longer
	// token matches too.
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----`),
		// OpenAI keys (sk-, sk-proj-) and Anthropic keys: every sk-ant- key
		// with its run of 32 is also an sk- key with a run of at least 32.
		regexp.MustCompile(`(?:^|[^A-Za-z0-9])sk-[A-Za-z0-9_-]{32}`),
		regexp.MustCompile(`(?:^|[^A-Za-z0-9])ghp_[A-Za-z0-9_]{36}`),        // GitHub classic personal access token
		regexp.MustCompile(`(?:^|[^A-Za-z0-9])github_pat_[A-Za-z0-9_]{30}`), // GitHub fine-grained personal access token
		regexp.MustCompile(`(?:^|[^A-Za-z0-9])gh[ousr]_[A-Za-z0-9_.-]{30}`), // GitHub OAuth, user, installation, refresh
		regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),                 // AWS access key ID
	}
	assignmentRun = regexp.MustCompile(`(?m)^(?:[A-Za-z_]\w*=[^\n]*(?:\n|\z)){3,}`)
	sensitiveName = regexp.MustCompile(`(?m)^\w*(?:KEY|SECRET|TOKEN|PASSWORD)`)
)

// CheckOutboundSecret checks text for the fixed secret patterns in local.md §8.
func CheckOutboundSecret(text string) error {
	for _, pattern := range secretPatterns {
		if pattern.MatchString(text) {
			return errSecret
		}
	}
	for _, run := range assignmentRun.FindAllString(text, -1) {
		if sensitiveName.MatchString(run) {
			return errSecret
		}
	}
	return nil
}
