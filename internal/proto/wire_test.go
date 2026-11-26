package proto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"
)

// testdata/wire.json keeps the two readers of the wire to one answer: Decode
// here, and the schemas of web/src/lib/proto.ts. Both test suites read it and
// judge every case the same way (contracts/conventions.md section 5). It is
// written by this file with -update, after the examples.
type wireFacts struct {
	// Shapes says, for the members each example has, which may be left out,
	// which may be null, which numbers may be a fraction, which texts are
	// random ids, and which members the frame's own type does not look into.
	// A path names its members with dots and leaves out the places in a list.
	Shapes map[string]shapeFacts `json:"shapes"`
	// Cases are examples with one value replaced.
	Cases []wireCase `json:"cases"`
}

type shapeFacts struct {
	Optional []string `json:"optional"`
	Nullable []string `json:"nullable"`
	Fraction []string `json:"fraction"`
	ID       []string `json:"id"`
	Opaque   []string `json:"opaque"`
}

type wireCase struct {
	Name  string          `json:"name"`
	File  string          `json:"file"`
	At    []string        `json:"at"`
	Value json.RawMessage `json:"value"`
	OK    bool            `json:"ok"`
}

const notAnID = "system"

var writeWire sync.Once

func parsed(t *testing.T, data []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

// factsOf reads from a type's tags what its example may and may not hold.
func factsOf(example any, t reflect.Type) shapeFacts {
	found := map[string][]string{}
	describe(example, t, "", found)
	list := func(kind string) []string {
		paths := append([]string{}, found[kind]...)
		slices.Sort(paths)
		return slices.Compact(paths)
	}
	return shapeFacts{list("optional"), list("nullable"), list("fraction"), list("id"), list("opaque")}
}

func describe(value any, t reflect.Type, path string, found map[string][]string) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == rawMessage || t.Kind() == reflect.Slice && t.Elem() == rawMessage {
		found["opaque"] = append(found["opaque"], path)
		return
	}
	switch value := value.(type) {
	case map[string]any:
		for _, field := range reflect.VisibleFields(t) {
			name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
			member, there := value[name]
			if name == "" || !there {
				continue
			}
			at := strings.TrimPrefix(path+"."+name, ".")
			if options == "omitempty" {
				found["optional"] = append(found["optional"], at)
			} else if field.Type.Kind() == reflect.Pointer {
				found["nullable"] = append(found["nullable"], at)
			}
			describe(member, field.Type, at, found)
		}
	case []any:
		for _, element := range value {
			describe(element, t.Elem(), path, found)
		}
	case string:
		if t == idType {
			found["id"] = append(found["id"], path)
		}
	case float64:
		if t.Kind() == reflect.Float64 {
			found["fraction"] = append(found["fraction"], path)
		}
	}
}

// The cases that are not about one member of every example: which booleans
// may be false.
func wireCases() []wireCase {
	cases := []wireCase{}
	add := func(name, file string, ok bool, value any, at ...string) {
		encoded, err := json.Marshal(value)
		if err != nil {
			panic(err)
		}
		cases = append(cases, wireCase{name, file, at, encoded, ok})
	}
	add("fast that is false", "host-info-answer", false, false, "agents", "0", "models", "0", "fast")
	add("refresh that is false", "host-info-request", true, false, "refresh")
	add("ask that is false", "row-task-stuck", true, false, "stuck", "ask")

	for _, c := range []struct {
		file string
		at   string
	}{{"cmd-ok", "ok"}, {"welcome", "ok"}, {"synced", "ok"}, {"fetched", "ok"}, {"push-off", "off"}, {"item-mention", "jump"}} {
		add(c.at+" that is true", c.file, true, true, c.at)
		add(c.at+" that is false", c.file, false, false, c.at)
	}
	return cases
}

func generated(t *testing.T) wireFacts {
	t.Helper()
	facts := wireFacts{map[string]shapeFacts{}, wireCases()}
	for _, name := range exampleNames(t) {
		facts.Shapes[name] = factsOf(parsed(t, example(t, name)), shapes[name])
	}
	return facts
}

// wireFile reads the file, and with -update writes it first: once, from the
// examples the tests before this one have written.
func wireFile(t *testing.T) wireFacts {
	t.Helper()
	if *update {
		writeWire.Do(func() {
			if err := os.WriteFile(filepath.Join("testdata", "wire.json"), written(generated(t)), 0644); err != nil {
				t.Fatal(err)
			}
		})
	}
	var facts wireFacts
	if err := Decode(example(t, "wire"), &facts); err != nil {
		t.Fatal(err)
	}
	return facts
}

// written lays the facts out with one shape and one case to a line,
// in ASCII: a character outside it is written as its escape, so that a letter
// that only looks like an ASCII one can be told from it.
func written(facts wireFacts) []byte {
	line := func(value any) string {
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		sure(e.Encode(value))
		var ascii strings.Builder
		for _, r := range strings.TrimSuffix(b.String(), "\n") {
			if r < 0x80 {
				ascii.WriteRune(r)
			} else {
				for _, unit := range utf16.Encode([]rune{r}) {
					fmt.Fprintf(&ascii, `\u%04x`, unit)
				}
			}
		}
		return ascii.String()
	}
	lines := func(of []string) string { return "\n  " + strings.Join(of, ",\n  ") + "\n " }
	names := []string{}
	for name := range facts.Shapes {
		names = append(names, name)
	}
	slices.Sort(names)
	shapes, cases := []string{}, []string{}
	for _, name := range names {
		shapes = append(shapes, line(name)+": "+line(facts.Shapes[name]))
	}
	for _, c := range facts.Cases {
		cases = append(cases, line(c))
	}
	return []byte("{\n \"shapes\": {" + lines(shapes) + "},\n \"cases\": [" + lines(cases) + "]\n}\n")
}

// The file is what the types and the cases above say, so that a change of a
// tag shows in a file the web client's tests read.
func TestWireFactsAreWhatTheTypesSay(t *testing.T) {
	if got, want := wireFile(t), generated(t); !reflect.DeepEqual(jsonValue(t, wireJSON(t, got)), jsonValue(t, wireJSON(t, want))) {
		t.Errorf("testdata/wire.json is not what the types say; run the tests with -update, after the examples")
	}
}

// reads reports whether Decode takes a document as the example of that name.
func reads(name string, document any) bool {
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return Decode(encoded, reflect.New(shapes[name]).Interface()) == nil
}

// replaced is a copy of a document with the value at a path replaced, or
// removed when change is nil.
func replaced(t *testing.T, document any, at []string, change func() any) any {
	t.Helper()
	copied := parsed(t, wireJSON(t, document))
	parent := copied
	for _, key := range at[:len(at)-1] {
		parent = child(t, parent, key)
	}
	last := at[len(at)-1]
	switch parent := parent.(type) {
	case map[string]any:
		if change == nil {
			delete(parent, last)
		} else {
			parent[last] = change()
		}
	case []any:
		parent[index(t, last)] = change()
	default:
		t.Fatalf("nothing at %q to change", at)
	}
	return copied
}

func child(t *testing.T, parent any, key string) any {
	t.Helper()
	if list, isList := parent.([]any); isList {
		return list[index(t, key)]
	}
	return parent.(map[string]any)[key]
}

func index(t *testing.T, key string) int {
	t.Helper()
	i, err := strconv.Atoi(key)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// named is a path as the facts write it: without the places in a list.
func named(at []string) string {
	names := slices.DeleteFunc(slices.Clone(at), func(key string) bool { _, err := strconv.Atoi(key); return err == nil })
	return strings.Join(names, ".")
}

// A judge changes one value of an example in every way the facts speak of,
// and holds Decode to them.
type judge struct {
	t        *testing.T
	name     string
	document any
	facts    shapeFacts
}

func (j judge) expect(what string, at []string, want bool, change func() any) {
	j.t.Helper()
	if got := reads(j.name, replaced(j.t, j.document, at, change)); got != want {
		j.t.Errorf("%s with %s %s: read %v, want %v", j.name, named(at), what, got, want)
	}
}

func (j judge) member(at []string) {
	path := named(at)
	j.expect("left out", at, slices.Contains(j.facts.Optional, path), nil)
	if !slices.Contains(j.facts.Opaque, path) {
		j.expect("null", at, slices.Contains(j.facts.Nullable, path), func() any { return nil })
	}
}

func (j judge) value(value any, at []string) {
	path := named(at)
	if slices.Contains(j.facts.Opaque, path) {
		return
	}
	refused := func(what string, change any) {
		if len(at) > 0 { // the whole example is not replaced
			j.expect(what, at, false, func() any { return change })
		}
	}
	switch value := value.(type) {
	case map[string]any:
		refused("a list", []any{})
		for name, member := range value {
			j.member(append(slices.Clone(at), name))
			j.value(member, append(slices.Clone(at), name))
		}
	case []any:
		refused("an object", map[string]any{})
		for i, element := range value {
			place := append(slices.Clone(at), strconv.Itoa(i))
			j.expect("null in the list", place, false, func() any { return nil })
			j.value(element, place)
		}
	case string:
		refused("a number", 7)
		if slices.Contains(j.facts.ID, path) {
			refused("a text that is no id", notAnID)
		}
	case float64:
		fraction := slices.Contains(j.facts.Fraction, path)
		refused("a text", "7")
		j.expect("a fraction", at, fraction, func() any { return 0.5 })
		j.expect("2^53", at, fraction, func() any { return 9007199254740992.0 })
	case bool:
		refused("a text", "true")
	}
}

// Every example, with each of its values changed in turn: left out, null, of
// another type, a fraction, beyond 2^53-1, a text that is no id. The web
// client's tests make the same changes and expect the same of its schemas.
func TestEveryExampleIsReadAsTheFactsSay(t *testing.T) {
	facts := wireFile(t)
	for _, name := range exampleNames(t) {
		document := parsed(t, example(t, name))
		if !reads(name, document) {
			t.Errorf("%s is not read", name)
		}
		judge{t, name, document, facts.Shapes[name]}.value(document, nil)
	}
}

func TestWireCases(t *testing.T) {
	for _, c := range wireFile(t).Cases {
		document := replaced(t, parsed(t, example(t, c.File)), c.At, func() any { return parsed(t, c.Value) })
		if got := reads(c.File, document); got != c.OK {
			t.Errorf("%s: %s with %s %s: read %v, want %v", c.Name, c.File, named(c.At), c.Value, got, c.OK)
		}
	}
}

// What the examples do not show is not in the facts: a path there is a member
// of the example.
func TestFactsNameOnlyMembersOfTheirExample(t *testing.T) {
	for name, facts := range wireFile(t).Shapes {
		has := map[string]bool{}
		collect(parsed(t, example(t, name)), "", has)
		for _, path := range slices.Concat(facts.Optional, facts.Nullable, facts.Fraction, facts.ID, facts.Opaque) {
			if !has[path] {
				t.Errorf("%s has no %s", name, path)
			}
		}
	}
}

func collect(value any, path string, has map[string]bool) {
	has[path] = true
	switch value := value.(type) {
	case map[string]any:
		for name, member := range value {
			collect(member, strings.TrimPrefix(path+"."+name, "."), has)
		}
	case []any:
		for _, element := range value {
			collect(element, path, has)
		}
	}
}
