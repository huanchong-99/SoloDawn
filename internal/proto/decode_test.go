package proto

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The two characters outside ASCII that fold to an ASCII letter.
const (
	longS  = string(rune(0x17f))  // folds to s
	kelvin = string(rune(0x212a)) // folds to k
)

// One field of every sort the wire types have.
type sample struct {
	Kind   string          `json:"kind"`
	Size   int             `json:"size"`
	At     int64           `json:"at"`
	Big    uint64          `json:"big"`
	Share  float64         `json:"share"`
	Done   bool            `json:"done"`
	List   []string        `json:"list"`
	Part   part            `json:"part"`
	Parts  []part          `json:"parts"`
	Thread *string         `json:"thread"` // "or null" in a contract
	Body   json.RawMessage `json:"body"`
	Note   string          `json:"note,omitempty"`
	Turns  *int            `json:"turns,omitempty"`
	Check  *part           `json:"check,omitempty"`
	embedded
}

type part struct {
	Name string `json:"name"`
	Seq  int64  `json:"seq"`
}

type embedded struct {
	Group string `json:"g"`
}

// document writes the members of a sample in order; change may replace the
// text of a member, remove it with "", or add text after the last one under "+".
func document(change map[string]string) string {
	members := [][2]string{
		{"kind", `"node"`}, {"size", "3"}, {"at", "1763600000000"}, {"big", "7"}, {"share", "12.5"}, {"done", "true"},
		{"list", `["a","b"]`}, {"part", `{"name":"p","seq":1}`}, {"parts", `[{"name":"q","seq":2}]`},
		{"thread", "null"}, {"body", `{"text":"hello"}`}, {"g", `"g1"`},
	}
	var out []string
	for _, m := range members {
		text, changed := change[m[0]]
		if !changed {
			text = m[1]
		}
		if text != "" {
			out = append(out, `"`+m[0]+`":`+text)
		}
	}
	return "{" + strings.Join(out, ",") + change["+"] + "}"
}

func whole() sample {
	return sample{
		Kind: "node", Size: 3, At: 1763600000000, Big: 7, Share: 12.5, Done: true, List: []string{"a", "b"},
		Part: part{"p", 1}, Parts: []part{{"q", 2}}, Body: json.RawMessage(`{"text":"hello"}`), embedded: embedded{"g1"},
	}
}

func decoded(t *testing.T, what, p string, want sample) {
	t.Helper()
	var got sample
	if err := Decode([]byte(p), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("%s: %s\n got  %+v, %v\n want %+v", what, p, got, err, want)
	}
}

// refused checks that Decode fails, with want when it is one of Decode's own
// errors; those come before anything is written. For a wrong type the error is
// encoding/json's, which may have filled other fields by then.
func refused(t *testing.T, what, p string, want error) {
	t.Helper()
	got := sample{Kind: "untouched"}
	err := Decode([]byte(p), &got)
	if err == nil || want != nil && (err != want || !reflect.DeepEqual(got, sample{Kind: "untouched"})) {
		t.Errorf("%s: %s\n got %+v, error %v; want error %v", what, p, got, err, want)
	}
}

// What Decode exists for: encoding/json alone reads these names as the
// lower-case fields and reads null as nothing. If a later Go release stops
// doing so, this test says so.
func TestPlainUnmarshalIsNotHowABrowserReads(t *testing.T) {
	for what, p := range map[string]string{
		"upper case":    document(map[string]string{"+": `,"KIND":"browser"`}),
		"mixed case":    document(map[string]string{"+": `,"Kind":"browser"`}),
		"a Kelvin sign": document(map[string]string{"+": `,"` + kelvin + `ind":"browser"`}),
	} {
		var got sample
		if err := json.Unmarshal([]byte(p), &got); err != nil || got.Kind != "browser" {
			t.Errorf("%s: encoding/json read kind as %q, %v", what, got.Kind, err)
		}
	}
	var got sample
	if err := json.Unmarshal([]byte(document(map[string]string{"+": `,"` + longS + `ize":9`})), &got); err != nil || got.Size != 9 {
		t.Errorf("a long s: encoding/json read size as %d, %v", got.Size, err)
	}
	got = sample{Kind: "kept"}
	if err := json.Unmarshal([]byte(`{"kind":null,"size":null,"list":null}`), &got); err != nil || got.Kind != "kept" {
		t.Errorf("null and missing members: encoding/json gave %+v, %v", got, err)
	}
}

func TestDecodeReadsFieldsByTheirExactNames(t *testing.T) {
	decoded(t, "every field", document(nil), whole())

	text, turns := "main", 4
	full := whole()
	full.Thread, full.Note, full.Turns, full.Check = &text, "n", &turns, &part{"c", 5}
	decoded(t, "with the optional fields", document(map[string]string{"thread": `"main"`, "+": `,"note":"n","turns":4,"check":{"name":"c","seq":5}`}), full)

	other := `,"KIND":"browser","Kind":"browser","kinD":"browser","SIZE":9,"` + longS + `ize":9,"` + kelvin + `ind":"browser","G":"g2","NOTE":"n","Turns":1`
	decoded(t, "other names after the exact ones", document(map[string]string{"+": other}), whole())
	decoded(t, "other names before the exact ones", "{"+other[1:]+","+document(nil)[1:], whole())
	decoded(t, "fields the receiver does not know", document(map[string]string{"+": `,"new":null,"more":{"KIND":1,"x":[null,{"A":2}]},"":null`}), whole())

	twice := whole()
	twice.Kind, twice.Size = "browser", 9
	decoded(t, "a name twice: the last counts", document(map[string]string{"+": `,"kind":"browser","size":9`}), twice)

	refused(t, "kind only in upper case", document(map[string]string{"kind": "", "+": `,"KIND":"node"`}), errMissing)
	refused(t, "kind only with a Kelvin sign", document(map[string]string{"kind": "", "+": `,"` + kelvin + `ind":"node"`}), errMissing)
	refused(t, "size only with a long s", document(map[string]string{"size": "", "+": `,"` + longS + `ize":3`}), errMissing)
	refused(t, "g only in upper case", document(map[string]string{"g": "", "+": `,"G":"g1"`}), errMissing)
}

func TestDecodeDropsOnlyNamesThatCouldMatchInAnotherCase(t *testing.T) {
	p := `{"a":1,"z":1,"0":1,"_":1,"@":1,"[":1,"é":1,"A":1,"Z":1,"M":1,"aB":1,"` + longS + `":1,"x` + kelvin + `":1}`
	var kept map[string]any
	if err := Decode([]byte(p), &kept); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for name := range kept {
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"0", "@", "[", "_", "a", "z", "é"}; !slices.Equal(names, want) {
		t.Errorf("kept %q, want %q", names, want)
	}
}

func TestDecodeReadsExactNamesAtEveryDepth(t *testing.T) {
	want := whole()
	want.Body = json.RawMessage(`{"state":{"status":"open"},"text":"hello"}`)
	decoded(t, "inside objects, lists and a body kept as JSON", document(map[string]string{
		"part":  `{"name":"p","NAME":"x","seq":1,"SEQ":9}`,
		"parts": `[{"Name":"x","name":"q","seq":2,"Seq":9}]`,
		"body":  `{"text":"hello","TEXT":"x","state":{"status":"open","STATUS":"done"}}`,
	}), want)
	refused(t, "a name of an inner object only in upper case", document(map[string]string{"part": `{"NAME":"p","seq":1}`}), errMissing)
	refused(t, "the same in a list", document(map[string]string{"parts": `[{"name":"q","seq":2},{"name":"r","SEQ":3}]`}), errMissing)

	var deep any
	if err := Decode([]byte(`[[{"a":[{"B":1,"c":{"D":2,"e":3,"f":null}}]}]]`), &deep); err != nil {
		t.Fatal(err)
	}
	if want := []any{[]any{map[string]any{"a": []any{map[string]any{"c": map[string]any{"e": 3.0, "f": nil}}}}}}; !reflect.DeepEqual(deep, want) {
		t.Errorf("got %v, want %v", deep, want)
	}
}

func TestDecodeRefusesAMissingField(t *testing.T) {
	for _, name := range []string{"kind", "size", "at", "big", "share", "done", "list", "part", "parts", "thread", "body", "g"} {
		refused(t, "no "+name, document(map[string]string{name: ""}), errMissing)
	}
	refused(t, "a part without its seq", document(map[string]string{"part": `{"name":"p"}`}), errMissing)
	refused(t, "a part in a list without its name", document(map[string]string{"parts": `[{"seq":2}]`}), errMissing)
	refused(t, "an optional part without its name", document(map[string]string{"+": `,"check":{"seq":5}`}), errMissing)
	refused(t, "an empty object", "{}", errMissing)

	empty := whole()
	empty.Kind, empty.Size, empty.Done, empty.List, empty.Parts, empty.Share = "", 0, false, []string{}, []part{}, 0
	decoded(t, "values that say nothing are values", document(map[string]string{"kind": `""`, "size": "0", "done": "false", "list": "[]", "parts": "[]", "share": "0"}), empty)
}

func TestDecodeRefusesNullWhereTheContractHasNone(t *testing.T) {
	for _, name := range []string{"kind", "size", "at", "big", "share", "done", "list", "part", "parts", "g"} {
		refused(t, name+" null", document(map[string]string{name: "null"}), errNull)
	}
	refused(t, "null in a list of texts", document(map[string]string{"list": `["a",null]`}), errNull)
	refused(t, "null in a list of objects", document(map[string]string{"parts": `[null]`}), errNull)
	refused(t, "null inside an object", document(map[string]string{"part": `{"name":null,"seq":1}`}), errNull)
	refused(t, "an optional text null", document(map[string]string{"+": `,"note":null`}), errNull)
	refused(t, "an optional number null", document(map[string]string{"+": `,"turns":null`}), errNull)
	refused(t, "an optional object null", document(map[string]string{"+": `,"check":null`}), errNull)
	refused(t, "null for the whole", "null", errNull)

	// The field marked "or null" takes it, and a body is judged by whoever reads it.
	decoded(t, "thread null", document(map[string]string{"thread": "null"}), whole())
	body := whole()
	body.Body = json.RawMessage("null")
	decoded(t, "body null", document(map[string]string{"body": "null"}), body)
	body.Body = json.RawMessage(`[1.5,null,{"a":null}]`)
	decoded(t, "a body that is a list", document(map[string]string{"body": `[1.5,null,{"a":null,"B":1}]`}), body)
}

func TestDecodeReadsAnIntegerByItsValue(t *testing.T) {
	for literal, value := range map[string]int64{
		"2":                  2,
		"2.0":                2,
		"2.000":              2,
		"2e0":                2,
		"20e-1":              2,
		"2.0000000000000001": 2, // the nearest double is 2
		"1.7636e12":          1763600000000,
		"0":                  0,
		"0.0":                0,
		"9007199254740991":   9007199254740991,
		"9007199254740991.0": 9007199254740991,
	} {
		want := whole()
		want.Size, want.At, want.Big, want.Turns = int(value), value, uint64(value), new(int)
		*want.Turns = int(value)
		decoded(t, "integers written "+literal, document(map[string]string{"size": literal, "at": literal, "big": literal, "+": `,"turns":` + literal}), want)
	}
	negative := whole()
	negative.Size, negative.At = -9007199254740991, -3
	decoded(t, "negative integers", document(map[string]string{"size": "-9007199254740991", "at": "-3.0"}), negative)
	zero := whole()
	zero.At = 0
	decoded(t, "minus zero", document(map[string]string{"at": "-0.0"}), zero)

	for _, literal := range []string{"2.5", "1e-1", "9007199254740992", "9007199254740993", "-9007199254740992", "1e20", "1e300", "-1e300"} {
		for _, name := range []string{"size", "at", "big"} {
			refused(t, name+" written "+literal, document(map[string]string{name: literal}), errInteger)
		}
		refused(t, "an optional integer written "+literal, document(map[string]string{"+": `,"turns":` + literal}), errInteger)
		refused(t, "an integer in a list of objects written "+literal, document(map[string]string{"parts": `[{"name":"q","seq":` + literal + `}]`}), errInteger)
	}
	for _, literal := range []string{"1e400", `"2"`, "true", "[2]", "{}"} {
		refused(t, "size written "+literal, document(map[string]string{"size": literal}), nil)
	}

	wide := whole()
	wide.Share = 1e20
	decoded(t, "a number that is no integer field", document(map[string]string{"share": "1e20"}), wide)
	wide.Share = 9007199254740993
	decoded(t, "a large number read as its nearest double", document(map[string]string{"share": "9007199254740993"}), wide)
}

func TestDecodeLeavesWrongTypesToTheDecoder(t *testing.T) {
	for what, change := range map[string]map[string]string{
		"a text for a list":           {"list": `"a"`},
		"an object for a list":        {"list": `{"0":"a"}`},
		"a list for an object":        {"part": `["p",1]`},
		"a number for an object":      {"part": "1"},
		"a number for a text":         {"kind": "1"},
		"a number for a truth value":  {"done": "1"},
		"a text for a number":         {"share": `"12.5"`},
		"an object for a text":        {"kind": `{"kind":"node"}`},
		"a list for an optional part": {"+": `,"check":[]`},
	} {
		refused(t, what, document(change), nil)
	}
}

func TestDecodeRefusesWhatIsNotJSON(t *testing.T) {
	for _, p := range []string{"", "hello", document(nil)[1:], document(nil) + "x", document(nil) + document(nil), `{'kind':'node'}`, "[" + document(nil) + "]"} {
		refused(t, "not one JSON object", p, nil)
	}
}

func TestDecodeKeepsTextAsItIs(t *testing.T) {
	// The signed payload inside an envelope is text, and upper case in text is not a name.
	p := `{"t":"POST","TO":"x","text":"<A> & B","n":null}`
	b, err := json.Marshal(Envelope{Payload: p, Key: "K", Signature: "S"})
	if err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := Decode(b, &env); err != nil || env != (Envelope{p, "K", "S"}) {
		t.Errorf("got %+v, %v", env, err)
	}
}
