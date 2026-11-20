package proto

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"
)

// What Decode refuses although encoding/json would read it.
var (
	errMissing = errors.New("proto: a field that is always present is missing")
	errNull    = errors.New("proto: null where the contract has no null")
	errInteger = errors.New("proto: not an integer of at most 2^53-1")
	errID      = errors.New("proto: not a random id")
)

var (
	rawMessage = reflect.TypeFor[json.RawMessage]()
	idType     = reflect.TypeFor[ID]()
	// 16 bytes are 21 characters and one that carries two bits: its other four are zero.
	canonicalID = regexp.MustCompile(`^[A-Za-z0-9_-]{21}[AQgw]$`)
)

// Decode reads wire JSON into the value v points to, the way
// contracts/conventions.md section 2 says a receiver reads. Everything read
// from the wire is decoded with it: a payload read once by the envelope check
// and once by its handler must not be read two ways. An error is bad_request.
//
// It differs from encoding/json where that package and a browser disagree:
//
//   - Names are exact. encoding/json also matches a name in another case,
//     folding Unicode: for {"to":"A","TO":"B"} it reads B, a browser A. Field
//     names are made of a to z, 0 to 9 and "_", so the only other names it
//     could match hold an upper-case letter or one of the two characters that
//     fold to one; Decode drops those members first, at every depth.
//   - A number is read as its nearest double, and an integer field takes it
//     only if that is an integer of at most 2^53-1 either way: 2.0 is 2, and
//     2^53 is refused. A number beyond the largest double, which a browser
//     reads as Infinity, is refused only by a field that reads it.
//   - A field without omitempty is always present, and only a pointer field
//     without omitempty can be null. encoding/json leaves a field at its zero
//     value for a missing member and for null alike.
//   - A field of type ID takes a random id and no other text.
//
// A json.RawMessage is not looked into: whoever decodes it later does that.
func Decode(data []byte, v any) error {
	if err := json.Unmarshal(data, new(json.RawMessage)); err != nil {
		return err // not one JSON value
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		panic(err) // one JSON value decodes into any
	}
	value = exact(value)
	if err := fits(value, reflect.TypeOf(v).Elem()); err != nil {
		return err
	}
	exact, err := json.Marshal(value)
	if err != nil {
		panic(err) // a value that was just decoded from JSON encodes
	}
	return json.Unmarshal(exact, v)
}

// exact drops the members whose name could match a field in another case, at
// every depth, and reads each number as its nearest double. A number beyond
// the largest double stays as written: encoding/json would refuse the whole
// value for it, a browser reads it as Infinity, and only a field that reads it
// refuses it.
func exact(value any) any {
	switch value := value.(type) {
	case map[string]any:
		exactMembers(value)
	case []any:
		for i, element := range value {
			value[i] = exact(element)
		}
	case json.Number:
		if f, _ := value.Float64(); !math.IsInf(f, 0) {
			return f
		}
	}
	return value
}

func exactMembers(object map[string]any) {
	for name, member := range object {
		if strings.ContainsFunc(name, foldsToLower) {
			delete(object, name)
		} else {
			object[name] = exact(member)
		}
	}
}

// foldsToLower reports whether a character is matched to a lower-case ASCII
// letter when case is ignored: A to Z, the long s (U+017F) and the Kelvin
// sign (U+212A).
func foldsToLower(r rune) bool {
	return 'A' <= r && r <= 'Z' || r == 0x17f || r == 0x212a
}

// fits reports what a decoded value holds that the type it is read into does
// not allow, where encoding/json would not say so.
func fits(value any, t reflect.Type) error {
	if t == rawMessage || t.Kind() == reflect.Interface {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		return fitsPointer(value, t.Elem())
	}
	return fitsValue(value, t)
}

// fitsPointer is for the one kind of field that may be null.
func fitsPointer(value any, t reflect.Type) error {
	if value == nil {
		return nil
	}
	return fitsValue(value, t)
}

func fitsValue(value any, t reflect.Type) error {
	switch value := value.(type) {
	case nil:
		return errNull
	case float64:
		return fitsNumber(value, t)
	case string:
		return fitsText(value, t)
	case map[string]any:
		return fitsStruct(value, t)
	case []any:
		return fitsList(value, t)
	}
	return nil
}

func fitsNumber(f float64, t reflect.Type) error {
	if k := t.Kind(); k >= reflect.Int && k <= reflect.Uint64 && (f != math.Trunc(f) || math.Abs(f) > 9007199254740991) {
		return errInteger
	}
	return nil
}

func fitsText(s string, t reflect.Type) error {
	if t == idType && !canonicalID.MatchString(s) {
		return errID
	}
	return nil
}

func fitsList(list []any, t reflect.Type) error {
	if t.Kind() != reflect.Slice {
		return nil // encoding/json reports a list where none belongs
	}
	for _, element := range list {
		if err := fits(element, t.Elem()); err != nil {
			return err
		}
	}
	return nil
}

func fitsStruct(object map[string]any, t reflect.Type) error {
	if t.Kind() != reflect.Struct {
		return nil // encoding/json reports an object where none belongs
	}
	for _, field := range reflect.VisibleFields(t) {
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		member, there := object[name]
		// An embedded struct has no name of its own; its fields are visited as this struct's.
		if err := fitsField(member, there, field.Type, options == "omitempty"); name != "" && err != nil {
			return err
		}
	}
	return nil
}

// fitsField holds one member to its field: an optional field may be left out
// and is never null; any other field is always there.
func fitsField(member any, there bool, t reflect.Type, optional bool) error {
	if !there && optional {
		return nil
	}
	if !there {
		return errMissing
	}
	if member == nil && optional {
		return errNull
	}
	return fits(member, t)
}
