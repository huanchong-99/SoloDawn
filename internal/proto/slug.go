package proto

import (
	"regexp"
	"strings"
)

// Slug is a task's name in its branch task/<num>-<slug> and in its worktree
// (contracts/rows.md section 4): 1 to 40 characters, groups of a to z and 0 to
// 9 with one "-" between two groups. That is every text MakeSlug gives and no
// other. Decode refuses any other text in a field of this type, so that none
// reaches a path or a branch name.
type Slug string

const slugLength = 40

var (
	slugText = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	notSlug  = regexp.MustCompile(`[^a-z0-9]+`)
)

func isSlug(s string) bool {
	return len(s) <= slugLength && slugText.MatchString(s)
}

// MakeSlug gives the slug of a task with this title, by the rule of
// contracts/local.md section 7. The home calls it once, when it creates the
// task; hosts read the slug from the task row.
func MakeSlug(title string) Slug {
	s := strings.Trim(notSlug.ReplaceAllString(strings.Map(lowerASCII, title), "-"), "-")
	s = strings.TrimRight(s[:min(len(s), slugLength)], "-")
	if s == "" {
		return "task"
	}
	return Slug(s)
}

// lowerASCII lowers A to Z and nothing else: a letter outside ASCII is never
// part of a slug, also one that a general lower-casing turns into a to z.
func lowerASCII(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + 'a' - 'A'
	}
	return r
}
