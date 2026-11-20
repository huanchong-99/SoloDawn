package proto

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// UnexpectedNulls returns the sorted JSON Pointers to nulls in encoded, except
// those explicitly allowed. The empty pointer names the root value.
func UnexpectedNulls(encoded []byte, allowed ...string) ([]string, error) {
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	paths := []string{}
	collectNulls(value, "", &paths)
	paths = slices.DeleteFunc(paths, func(p string) bool { return slices.Contains(allowed, p) })
	slices.Sort(paths)
	return paths, nil
}

var pointerEscape = strings.NewReplacer("~", "~0", "/", "~1")

func collectNulls(value any, path string, paths *[]string) {
	switch v := value.(type) {
	case nil:
		*paths = append(*paths, path)
	case map[string]any:
		for k, child := range v {
			collectNulls(child, path+"/"+pointerEscape.Replace(k), paths)
		}
	case []any:
		for i, child := range v {
			collectNulls(child, path+"/"+strconv.Itoa(i), paths)
		}
	}
}
