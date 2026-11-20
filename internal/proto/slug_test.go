package proto

import (
	"strings"
	"testing"
)

// The slug of each title in the shared file is the one the rule gives, and a
// slug: the web client's tests hold its schema to the same list.
func TestMakeSlug(t *testing.T) {
	cases := wireFile(t).Slugs
	if len(cases) != len(slugTitles) {
		t.Fatalf("%d slugs in the file, %d titles here", len(cases), len(slugTitles))
	}
	for _, c := range cases {
		if got := MakeSlug(c.Title); string(got) != c.Slug || !isSlug(c.Slug) {
			t.Errorf("%q: slug %q, the file has %q", c.Title, got, c.Slug)
		}
	}
	for title, want := range map[string]Slug{
		"Node: report connection path changes": "node-report-connection-path-changes",
		"ABC xyz 0189":                         "abc-xyz-0189",
		"From A to Z":                          "from-a-to-z",
		"a@b[c`d{e/f:g":                        "a-b-c-d-e-f-g", // the characters next to A, Z, a, z, 0 and 9
		"  --Hello,   World!!  ":               "hello-world",
		"":                                     "task",
		"!!!":                                  "task",
		"任务":                                   "task",
		"Größe ändern":                         "gr-e-ndern",
		"Kelvin":                               "elvin", // a general lower-casing would give kelvin
		"ſtop":                                 "top",
		"a\xffb":                               "a-b", // bytes that are no UTF-8
		strings.Repeat("a", 40):                Slug(strings.Repeat("a", 40)),
		strings.Repeat("a", 41):                Slug(strings.Repeat("a", 40)),
		strings.Repeat("a", 39) + " b":         Slug(strings.Repeat("a", 39)), // cut after the hyphen, which goes too
		strings.Repeat("a", 38) + " bcd":       Slug(strings.Repeat("a", 38) + "-b"),
		strings.Repeat("a", 39) + "--" + strings.Repeat("b", 9): Slug(strings.Repeat("a", 39)),
	} {
		if got := MakeSlug(title); got != want {
			t.Errorf("%q: slug %q, want %q", title, got, want)
		}
	}
}

// A slug is at most 40 characters: groups of a to z and 0 to 9, one hyphen between two.
func TestIsSlug(t *testing.T) {
	long := strings.Repeat("a", 40)
	for _, s := range []string{"a", "z", "0", "9", "task", "a-b", "a1-2b-c3", long, long[:38] + "-b"} {
		if !isSlug(s) {
			t.Errorf("%q is a slug", s)
		}
	}
	for _, s := range []string{"", "-", "-a", "a-", "a--b", "A", "aB", "a b", "a_b", "a.b", "a/b", "../x", "abc\n", "\nabc", "a\x00b",
		"café", "Kelvin", "`", "{", "/", ":", "@", "[", long + "a", long[:39] + "-b"} {
		if isSlug(s) {
			t.Errorf("%q is no slug", s)
		}
	}
}

// Decode takes a slug in a field of type Slug and no other text.
func TestDecodeTakesOnlyASlugInASlugField(t *testing.T) {
	type tagged struct {
		Slug Slug   `json:"slug"`
		Old  Slug   `json:"old,omitempty"`
		All  []Slug `json:"all"`
		Text string `json:"text"`
	}
	read := func(document string) (tagged, error) {
		var got tagged
		err := Decode([]byte(document), &got)
		return got, err
	}
	if got, err := read(`{"slug":"a-b","old":"c","all":["d","e-f"],"text":"Not A Slug"}`); err != nil || got.Slug != "a-b" || got.Old != "c" || len(got.All) != 2 {
		t.Errorf("slugs read as %+v, %v", got, err)
	}
	for _, bad := range []string{`""`, `"A"`, `"a-"`, `"../x"`, `"` + strings.Repeat("a", 41) + `"`} {
		for _, document := range []string{
			`{"slug":` + bad + `,"all":[],"text":""}`,
			`{"slug":"a","old":` + bad + `,"all":[],"text":""}`,
			`{"slug":"a","all":["b",` + bad + `],"text":""}`,
		} {
			if _, err := read(document); err != errSlug {
				t.Errorf("%s: %v, want %v", document, err, errSlug)
			}
		}
	}
	if _, err := read(`{"slug":null,"all":[],"text":""}`); err != errNull {
		t.Errorf("slug null: %v, want %v", err, errNull)
	}
	if _, err := read(`{"all":[],"text":""}`); err != errMissing {
		t.Errorf("no slug: %v, want %v", err, errMissing)
	}
	if _, err := read(`{"slug":7,"all":[],"text":""}`); err == nil || err == errSlug {
		t.Errorf("slug a number: %v, want the decoder's error", err)
	}
}

// A step that cannot fail panics when it does.
func TestSurePanicsOnAnError(t *testing.T) {
	sure(nil)
	defer func() {
		if recover() != errSlug {
			t.Error("sure did not panic with its error")
		}
	}()
	sure(errSlug)
}
