package host

import (
	"errors"
	"regexp"
)

var (
	errSecret = errors.New("secret: this looks like a secret; remove it and send again")

	// The patterns of local.md §8. Each has two groups: what precedes the
	// secret, which stays, and the secret, which redaction replaces. A vendor
	// credential is a prefix not preceded by a letter or digit, then at least
	// the run of key characters the contract gives; the whole longer run is
	// the secret.
	secretPatterns = []*regexp.Regexp{
		// A PEM or OpenPGP private key: the header, through its footer when
		// the text has one, else to the end of the text.
		regexp.MustCompile(`()(-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----(?s:.*?-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----|.*))`),
		// A footer left without its header, as when output was cut inside a
		// key: from the start of the text through the last such footer.
		regexp.MustCompile(`()((?s:\A.*)-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----)`),
		// OpenAI keys (sk-, sk-proj-) and Anthropic keys: every sk-ant- key
		// with its run of 32 is also an sk- key with a run of at least 32.
		regexp.MustCompile(`(^|[^A-Za-z0-9])(sk-[A-Za-z0-9_-]{32,})`),
		regexp.MustCompile(`(^|[^A-Za-z0-9])(ghp_[A-Za-z0-9_]{36,})`),        // GitHub classic personal access token
		regexp.MustCompile(`(^|[^A-Za-z0-9])(github_pat_[A-Za-z0-9_]{30,})`), // GitHub fine-grained personal access token
		regexp.MustCompile(`(^|[^A-Za-z0-9])(gh[ousr]_[A-Za-z0-9_.-]{30,})`), // GitHub OAuth, user, installation, refresh
		regexp.MustCompile(`(^|[^A-Za-z0-9])((?:AKIA|ASIA)[A-Z0-9]{16,})`),   // AWS access key id
	}
	// Three or more NAME=value lines in a row, without the last line's end.
	assignmentRun = regexp.MustCompile(`(?m)^[A-Za-z_]\w*=[^\n]*(?:\n[A-Za-z_]\w*=[^\n]*){2,}`)
	// A run is a secret when a name contains one of these words, in any case.
	sensitiveName = regexp.MustCompile(`(?mi)^\w*(?:KEY|SECRET|TOKEN|PASSWORD)`)
)

const (
	redacted = "[redacted]"
	// keepLead is the replacement of a pattern match: the text before the
	// secret, then the mark.
	keepLead = "${1}[redacted]"
)

// CheckOutboundSecret checks text an AI sends for the fixed secret patterns
// in local.md §8.
func CheckOutboundSecret(text string) error {
	if RedactSecrets(text) != text {
		return errSecret
	}
	return nil
}

// RedactSecrets replaces each match of the patterns in local.md §8 with
// "[redacted]", for free text the host or the node writes and sends to the
// group (local.md §7, "Text the host writes"). An environment-file run is
// replaced as a whole.
func RedactSecrets(text string) string {
	for _, pattern := range secretPatterns {
		text = pattern.ReplaceAllString(text, keepLead)
	}
	return assignmentRun.ReplaceAllStringFunc(text, func(run string) string {
		if sensitiveName.MatchString(run) {
			return redacted
		}
		return run
	})
}
