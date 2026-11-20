package host

import (
	"errors"
	"regexp"
	"strings"
)

var errSecret = errors.New("secret: this looks like a secret; remove it and send again")

// secretPattern is one entry of the list in local.md §8. find is what the
// check looks for; redact has two groups, what precedes the secret, which
// stays, and the secret, which redaction replaces. One of literals occurs
// in every match: a cheap test before a regular expression runs.
type secretPattern struct {
	literals     []string
	find, redact *regexp.Regexp
}

func (p secretPattern) mayMatch(text string) bool {
	for _, literal := range p.literals {
		if strings.Contains(text, literal) {
			return true
		}
	}
	return false
}

// credential is a vendor credential: a prefix not preceded by a letter or
// digit, then at least the run of key characters the contract gives; the
// whole longer run is the secret.
func credential(expr string, literals ...string) secretPattern {
	re := regexp.MustCompile(expr)
	return secretPattern{literals: literals, find: re, redact: re}
}

const (
	privateKeyHeader = `-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`
	privateKeyFooter = `-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`
)

var (
	secretPatterns = []secretPattern{
		// A PEM or OpenPGP private key: the check looks for the header;
		// redaction takes it through its footer when the text has one, else
		// to the end of the text.
		{
			literals: []string{"-----BEGIN "},
			find:     regexp.MustCompile(privateKeyHeader),
			redact:   regexp.MustCompile(`()(` + privateKeyHeader + `(?s:.*?` + privateKeyFooter + `|.*))`),
		},
		// OpenAI keys (sk-, sk-proj-) and Anthropic keys: every sk-ant- key
		// with its run of 32 is also an sk- key with a run of at least 32.
		credential(`(^|[^A-Za-z0-9])(sk-[A-Za-z0-9_-]{32,})`, "sk-"),
		credential(`(^|[^A-Za-z0-9])(ghp_[A-Za-z0-9_]{36,})`, "ghp_"),               // GitHub classic personal access token
		credential(`(^|[^A-Za-z0-9])(github_pat_[A-Za-z0-9_]{30,})`, "github_pat_"), // GitHub fine-grained personal access token
		credential(`(^|[^A-Za-z0-9])(gh[ousr]_[A-Za-z0-9_.-]{30,})`, "gho_", "ghu_", "ghs_", "ghr_"),
		credential(`(^|[^A-Za-z0-9])((?:AKIA|ASIA)[A-Z0-9]{16,})`, "AKIA", "ASIA"), // AWS access key id
	}
	// Redaction only: a footer left without its header, as when output was
	// cut inside a key, goes from the start of the text through the last
	// such footer. The check does not use it: a footer alone is no secret,
	// and code that handles keys names one.
	redactPatterns = append(secretPatterns[:len(secretPatterns):len(secretPatterns)], secretPattern{
		literals: []string{"-----END "},
		redact:   regexp.MustCompile(`()((?s:\A.*)` + privateKeyFooter + `)`),
	})

	// Environment-file content in plain text, and in the added lines of a
	// diff, where every line starts with '>'.
	plainEnv = envPattern("")
	diffEnv  = envPattern(">")
)

// envContent matches three or more NAME=value lines in a row (without the
// last line's end); such a run is a secret when a name contains one of the
// words, in any case.
type envContent struct{ run, name *regexp.Regexp }

func envPattern(prefix string) envContent {
	line := prefix + `[A-Za-z_]\w*=[^\n]*`
	return envContent{
		run:  regexp.MustCompile(`(?m)^` + line + `(?:\n` + line + `){2,}`),
		name: regexp.MustCompile(`(?mi)^` + prefix + `\w*(?:KEY|SECRET|TOKEN|PASSWORD)`),
	}
}

const (
	redacted = "[redacted]"
	// keepLead is the replacement of a pattern match: the text before the
	// secret, then the mark.
	keepLead = "${1}[redacted]"
)

// CheckOutboundSecret checks text an AI sends for the fixed secret patterns
// in local.md §8.
func CheckOutboundSecret(text string) error {
	if containsSecret(text, plainEnv) {
		return errSecret
	}
	return nil
}

// containsSecret reports a match of the patterns of local.md §8 in text,
// whose lines are shaped as env expects. It builds no copy of the text.
func containsSecret(text string, env envContent) bool {
	for _, p := range secretPatterns {
		if p.mayMatch(text) && p.find.MatchString(text) {
			return true
		}
	}
	for _, run := range env.run.FindAllStringIndex(text, -1) {
		if env.name.MatchString(text[run[0]:run[1]]) {
			return true
		}
	}
	return false
}

// RedactSecrets replaces each match of the patterns in local.md §8 with
// "[redacted]", for free text the host or the node writes and sends to the
// group (local.md §7, "Text the host writes"). An environment-file run is
// replaced as a whole.
func RedactSecrets(text string) string {
	for _, p := range redactPatterns {
		if p.mayMatch(text) {
			text = p.redact.ReplaceAllString(text, keepLead)
		}
	}
	return plainEnv.run.ReplaceAllStringFunc(text, func(run string) string {
		if plainEnv.name.MatchString(run) {
			return redacted
		}
		return run
	})
}
