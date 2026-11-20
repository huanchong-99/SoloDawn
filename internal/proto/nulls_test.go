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
		{"nested indexes", json.RawMessage(`{"z":[null,true,{"d":null}],"a":null,"ts":0,"x":""}`),
			[]string{"/z/0"}, []string{"/a", "/z/2/d"}},
		{"empty lists and objects", json.RawMessage(`{"mentions":[],"body":{},"thread":null}`), []string{"/thread"}, []string{}},
		{"root", nil, nil, []string{""}},
		{"allowed root", nil, []string{""}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnexpectedNulls(wireJSON(t, tc.value), tc.allowed...); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected nulls = %q; want %q", got, tc.want)
			}
		})
	}
}
