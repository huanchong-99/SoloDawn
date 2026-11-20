package proto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestUnexpectedNulls(t *testing.T) {
	for _, tc := range []struct {
		name          string
		value         any
		allowed, want []string
	}{
		{"nil list and raw body", MsgRow{}, []string{"/thread", "/re", "/state"}, []string{"/body", "/mentions"}},
		{"nested indexes and escaped keys", json.RawMessage(`{"z":[null,true,{"d/~":null}],"a":null,"ts":0,"x":""}`),
			[]string{"/z/0"}, []string{"/a", "/z/2/d~1~0"}},
		{"empty lists and objects", json.RawMessage(`{"mentions":[],"body":{},"thread":null}`), []string{"/thread"}, []string{}},
		{"root", nil, nil, []string{""}},
		{"allowed root", nil, []string{""}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UnexpectedNulls(wireJSON(t, tc.value), tc.allowed...)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected nulls = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestUnexpectedNullsInvalidJSON(t *testing.T) {
	got, err := UnexpectedNulls([]byte("{"))
	if got != nil || err == nil || err.Error() != "unexpected end of JSON input" {
		t.Fatalf("invalid JSON: got %q, %v", got, err)
	}
}
