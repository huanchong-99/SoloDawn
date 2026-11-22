package host

import (
	"slices"
	"strings"
	"unicode"
)

var modelTiers = []struct {
	words  []string
	tier   string
	source string
}{
	{[]string{"haiku", "mini", "nano", "flash", "lite"}, "light", "planning/contracts/step-3/drivers.md#102-the-suggested-tier"},
	{[]string{"opus", "fable"}, "strong", "planning/contracts/step-3/drivers.md#102-the-suggested-tier"},
}

func modelTier(id string) string {
	words := strings.FieldsFunc(strings.ToLower(id), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, row := range modelTiers {
		if slices.ContainsFunc(words, func(word string) bool { return slices.Contains(row.words, word) }) {
			return row.tier
		}
	}
	return "standard"
}
