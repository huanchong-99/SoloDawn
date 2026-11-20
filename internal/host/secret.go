package host

import (
	"errors"
	"regexp"
)

const assignment = `[A-Za-z_]\w*=[^\n]*`

var (
	errSecret = errors.New("secret: this looks like a secret; remove it and send again")

	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----`),
		regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`),       // GitHub classic personal access token
		regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), // AWS access key ID
	}
	assignmentRun = regexp.MustCompile(`(?m)^(?:` + assignment + `\n){2,}` + assignment)
	sensitiveName = regexp.MustCompile(`(?m)^\w*(?:KEY|SECRET|TOKEN|PASSWORD)\w*=`)
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
