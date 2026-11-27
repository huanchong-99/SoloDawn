package proto

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidProfile reports whether p satisfies the bounds of
// planning/contracts/step-3/profiles.md section 1.
func ValidProfile(p Profile) bool {
	return validBoundedText(p.Model, 100) &&
		validBoundedText(p.Effort, 32) &&
		validTier(p.Tier) &&
		validSkills(p.Skills)
}

func validBoundedText(s string, limit int) bool {
	return utf8.RuneCountInString(s) <= limit && strings.IndexFunc(s, unicode.IsControl) == -1
}

func validTier(tier string) bool {
	switch tier {
	case "light", "standard", "strong":
		return true
	default:
		return false
	}
}

func validSkills(s string) bool {
	return utf8.RuneCountInString(s) <= 200 && !strings.ContainsAny(s, "\r\n")
}
