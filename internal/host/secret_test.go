package host_test

import (
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/host"
)

func TestCheckOutboundSecretRejects(t *testing.T) {
	for name, text := range map[string]string{
		"PKCS8":                   "copied: -----BEGIN PRIVATE KEY-----\n",
		"traditional":             "-----BEGIN RSA PRIVATE KEY-----",
		"OpenSSH":                 "-----BEGIN OPENSSH PRIVATE KEY-----",
		"encrypted":               "-----BEGIN ENCRYPTED PRIVATE KEY-----",
		"two words":               "-----BEGIN SSH2 ENCRYPTED PRIVATE KEY-----",
		"GitHub":                  "token: (ghp_" + strings.Repeat("xY7", 12) + ")",
		"GitHub long":             "ghp_" + strings.Repeat("a", 37),
		"GitHub underscore":       "ghp_" + strings.Repeat("a", 35) + "_",
		"GitHub after underscore": "MY_ghp_" + strings.Repeat("a", 36),
		"GitHub fine-grained":     "github_pat_" + strings.Repeat("B_7", 10),
		"GitHub OAuth":            "gho_" + strings.Repeat("c", 30),
		"GitHub user":             "ghu_" + strings.Repeat("d", 30),
		"GitHub installation":     "\nghs_" + strings.Repeat("e", 36),
		"GitHub installation JWT": "ghs_12345_eyJhbGciOiJSUzI1NiJ9.eyJz-dWIi_OiIx",
		"GitHub refresh":          "x ghr_" + strings.Repeat("f", 29) + ".",
		"Anthropic":               "ANTHROPIC_API_KEY=sk-ant-api03-" + strings.Repeat("Ab_-", 8),
		"Anthropic run of 32":     "sk-ant-" + strings.Repeat("g", 32),
		"OpenAI":                  "sk-" + strings.Repeat("h", 32),
		"OpenAI project":          "(sk-proj-" + strings.Repeat("Ij_-", 8) + "...)",
		"OpenAI at start":         "sk-" + strings.Repeat("k", 300),
		"AWS long term":           "key=AKIA" + strings.Repeat("Q7", 8),
		"AWS temporary":           "key=ASIA" + strings.Repeat("8S", 8),
		"AWS long":                "ASIA" + strings.Repeat("A", 17),
		"AWS after underscore":    "AWS_AKIA" + strings.Repeat("B", 16),
		"lower-case name":         "A=x\napi_key=y\nC=z",
		"mixed-case name":         "Db_Password=x\nB=y\nC=z",
		"key first":               "API_KEY_ID=a=b\nb=two\nC=three",
		"secret middle":           "A=one\r\nTOP_SECRET=two\r\nC=three\r\n",
		"token last":              "A=\n_b2=two\nTOKEN_SUFFIX=three",
		"long run":                "prefix\nA=1\nB=2\nC=3\nDB_PASSWORD=4\nsuffix",
		"second run":              "A=1\nB=2\nC=3\n---\nX=4\nMY_KEY=5\nZ=6",
	} {
		t.Run(name, func(t *testing.T) {
			const want = "secret: this looks like a secret; remove it and send again"
			if err := host.CheckOutboundSecret(text); err == nil || err.Error() != want {
				t.Errorf("got %v, want %q", err, want)
			}
		})
	}
}

func TestCheckOutboundSecretAllows(t *testing.T) {
	githubBody := strings.Repeat("a", 36)
	awsBody := strings.Repeat("A", 16)
	for name, text := range map[string]string{
		"empty":                     "",
		"public key":                "-----BEGIN PUBLIC KEY-----",
		"private key footer":        "-----END PRIVATE KEY-----",
		"incomplete header":         "-----BEGIN PRIVATE KEY----",
		"no leading dashes":         "BEGIN PRIVATE KEY-----",
		"lower-case label":          "-----BEGIN my PRIVATE KEY-----",
		"GitHub short":              "ghp_" + githubBody[1:],
		"GitHub wrong alphabet":     "ghp_" + githubBody[1:] + "-",
		"GitHub fine-grained short": "github_pat_" + githubBody[7:],
		"GitHub fine-grained dot":   "github_pat_" + githubBody[7:] + ".",
		"GitHub OAuth short":        "gho_" + githubBody[7:],
		"GitHub other letter":       "ghx_" + githubBody,
		"GitHub after a digit":      "1ghs_" + githubBody,
		"Anthropic short":           "sk-ant-" + githubBody[11:],
		"OpenAI short":              "sk-" + githubBody[5:],
		"OpenAI wrong alphabet":     "sk-" + githubBody[5:] + ".",
		"OpenAI inside a word":      "task-" + githubBody,
		"OpenAI no dash":            "sk_" + githubBody,
		"GitHub wrong prefix":       "ghx_" + githubBody,
		"GitHub inside name":        "notghp_" + githubBody,
		"AWS short":                 "AKIA" + awsBody[1:],
		"AWS wrong alphabet":        "AKIA" + awsBody[1:] + "a",
		"AWS wrong prefix":          "ABCD" + awsBody,
		"AWS inside name":           "XAKIA" + awsBody,
		"two assignments":           "API_KEY=x\nB=y",
		"value not name":            "A=KEY\nB=SECRET=value\nC=TOKEN\nD=PASSWORD",
		"blank breaks run":          "A=x\n\nTOKEN=y\nC=z",
		"comment breaks run":        "KEY=x\nB=y\n# note\nC=z",
		"invalid name":              "KEY=x\n2B=y\nC=z",
		"prose before name":         "the KEY=x\nB=y\nC=z",
		"one line":                  "KEY=b=c=d",
		"sensitive outside run":     "KEY=x\ntext\nA=1\nB=2\nC=3",
	} {
		t.Run(name, func(t *testing.T) {
			if err := host.CheckOutboundSecret(text); err != nil {
				t.Errorf("got %v, want nil", err)
			}
		})
	}
}
