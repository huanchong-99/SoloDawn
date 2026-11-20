package host

import (
	"errors"
	"regexp"
	"strings"
)

var errSecret = errors.New("secret: this looks like a secret; remove it and send again")

// secretPattern is one entry of the list in local.md §8. find is what the
// check looks for; it starts with a literal, which the search skips to.
// With lead, a match counts only when no letter or digit precedes it.
// redact has two groups, what precedes the secret, which stays, and the
// secret, which redaction replaces. One of literals occurs in every match:
// a cheap test before a regular expression runs.
type secretPattern struct {
	literals     []string
	lead         bool
	find, redact *regexp.Regexp
}

// in reports a match of p in text, trying each place find matches.
func (p secretPattern) in(text string) bool {
	for off := 0; ; {
		loc := p.find.FindStringIndex(text[off:])
		if loc == nil {
			return false
		}
		at := off + loc[0]
		if !p.lead || at == 0 || !isAlnum(text[at-1]) {
			return true
		}
		off = at + 1
	}
}

func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
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
	return secretPattern{
		literals: literals,
		lead:     true,
		find:     regexp.MustCompile(expr),
		redact:   regexp.MustCompile(`(^|[^A-Za-z0-9])(` + expr + `)`),
	}
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
		credential(`sk-[A-Za-z0-9_-]{32,}`, "sk-"),
		credential(`ghp_[A-Za-z0-9_]{36,}`, "ghp_"),               // GitHub classic personal access token
		credential(`github_pat_[A-Za-z0-9_]{30,}`, "github_pat_"), // GitHub fine-grained personal access token
		credential(`gh[ousr]_[A-Za-z0-9_.-]{30,}`, "gho_", "ghu_", "ghs_", "ghr_"),
		credential(`A[KS]IA[A-Z0-9]{16,}`, "AKIA", "ASIA"), // AWS access key id: AKIA, ASIA
	}
	// Redaction only: a footer left without its header, as when output was
	// cut inside a key, goes from the start of the text through the last
	// such footer. The check does not use it: a footer alone is no secret,
	// and code that handles keys names one.
	redactPatterns = append(secretPatterns[:len(secretPatterns):len(secretPatterns)], secretPattern{
		literals: []string{"-----END "},
		redact:   regexp.MustCompile(`()((?s:\A.*)` + privateKeyFooter + `)`),
	})

	// Environment-file content, for redaction: three or more NAME=value
	// lines in a row (without the last line's end), a secret when a name
	// contains one of sensitiveWords, in any case. envSecretIn is the same
	// rule for the check.
	envRun  = regexp.MustCompile(`(?m)^[A-Za-z_]\w*=[^\n]*(?:\n[A-Za-z_]\w*=[^\n]*){2,}`)
	envName = regexp.MustCompile(`(?mi)^\w*(?:KEY|SECRET|TOKEN|PASSWORD)`)
)

var sensitiveWords = []string{"KEY", "SECRET", "TOKEN", "PASSWORD"}

// envSecretIn reports environment-file content in text, whose lines start
// with prefix: "" for plain text, ">" for the added lines of a diff. It
// reads the text once and builds no copy of it.
func envSecretIn(text, prefix string) bool {
	run, sensitive := 0, false
	for line := range strings.SplitSeq(text, "\n") {
		name, ok := envLineName(line, prefix)
		if !ok {
			run, sensitive = 0, false
			continue
		}
		run++
		sensitive = sensitive || hasSensitiveWord(name)
		if run >= 3 && sensitive {
			return true
		}
	}
	return false
}

// envLineName is the NAME of a NAME=value line: a letter or '_', then
// letters, digits and '_', up to the first '='.
func envLineName(line, prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(line, prefix)
	name, _, found := strings.Cut(rest, "=")
	if !ok || !found || name == "" || '0' <= name[0] && name[0] <= '9' {
		return "", false
	}
	return name, strings.IndexFunc(name, notWordRune) < 0
}

func notWordRune(r rune) bool {
	return !(r == '_' || r < 128 && isAlnum(byte(r)))
}

func hasSensitiveWord(name string) bool {
	for _, word := range sensitiveWords {
		for i := 0; i+len(word) <= len(name); i++ {
			if strings.EqualFold(name[i:i+len(word)], word) {
				return true
			}
		}
	}
	return false
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
	if containsSecret(text, "") {
		return errSecret
	}
	return nil
}

// containsSecret reports a match of the patterns of local.md §8 in text,
// whose lines start with linePrefix. It builds no copy of the text.
func containsSecret(text, linePrefix string) bool {
	for _, p := range secretPatterns {
		if p.mayMatch(text) && p.in(text) {
			return true
		}
	}
	return envSecretIn(text, linePrefix)
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
	return envRun.ReplaceAllStringFunc(text, func(run string) string {
		if envName.MatchString(run) {
			return redacted
		}
		return run
	})
}
