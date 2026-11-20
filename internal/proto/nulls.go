package proto

import (
	"encoding/json"
	"slices"
	"strconv"
)

// UnexpectedNulls returns the sorted JSON Pointers to the nulls in encoded,
// except those allowed. The empty pointer names the root value. encoded is
// what the caller's own code just encoded; anything else is a panic.
func UnexpectedNulls(encoded []byte, allowed ...string) []string {
	var value any
	sure(json.Unmarshal(encoded, &value))
	paths := []string{}
	collectNulls(value, "", &paths)
	paths = slices.DeleteFunc(paths, func(p string) bool { return slices.Contains(allowed, p) })
	slices.Sort(paths)
	return paths
}

// Names on the wire are made of a to z, 0 to 9 and _ (conventions section 2),
// so a name needs no escaping in a pointer.
func collectNulls(value any, path string, paths *[]string) {
	switch v := value.(type) {
	case nil:
		*paths = append(*paths, path)
	case map[string]any:
		for k, child := range v {
			collectNulls(child, path+"/"+k, paths)
		}
	case []any:
		for i, child := range v {
			collectNulls(child, path+"/"+strconv.Itoa(i), paths)
		}
	}
}
