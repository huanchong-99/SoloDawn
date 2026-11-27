package proto

import (
	"strings"
	"testing"
)

func TestValidProfile(t *testing.T) {
	base := Profile{Model: "opus", Effort: "high", Tier: "strong", Skills: "Go backend"}
	cases := []struct {
		name    string
		profile Profile
		want    bool
	}{
		{"valid strong", base, true},
		{"valid light empty fields", Profile{Tier: "light"}, true},
		{"valid standard", Profile{Tier: "standard"}, true},
		{"model 100 runes", Profile{Model: strings.Repeat("你", 100), Tier: "standard"}, true},
		{"model 101 runes", Profile{Model: strings.Repeat("a", 101), Tier: "standard"}, false},
		{"model control char", Profile{Model: "opus\n", Tier: "standard"}, false},
		{"model nul control char", Profile{Model: "opus\x00", Tier: "standard"}, false},
		{"effort 32 runes", Profile{Effort: strings.Repeat("é", 32), Tier: "standard"}, true},
		{"effort 33 runes", Profile{Effort: strings.Repeat("a", 33), Tier: "standard"}, false},
		{"effort control char", Profile{Effort: "high\t", Tier: "standard"}, false},
		{"tier empty", Profile{}, false},
		{"tier expert", Profile{Tier: "expert"}, false},
		{"skills 200 runes", Profile{Tier: "standard", Skills: strings.Repeat("术", 200)}, true},
		{"skills 201 runes", Profile{Tier: "standard", Skills: strings.Repeat("a", 201)}, false},
		{"skills newline", Profile{Tier: "standard", Skills: "a\nb"}, false},
		{"skills carriage return", Profile{Tier: "standard", Skills: "a\rb"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidProfile(tc.profile); got != tc.want {
				t.Errorf("ValidProfile(%+v) = %v, want %v", tc.profile, got, tc.want)
			}
		})
	}
}
