package fakecli

import (
	"bufio"
	"strings"
	"testing"
)

func TestInputExpectations(t *testing.T) {
	const line = `{"id":9007199254740993,"msg":{"body":null,"roles":["peer","owner"]}}`
	for _, tt := range []struct {
		want string
		ok   bool
	}{
		{`{"msg":{"body":null}}`, true},
		{`{"msg":{"absent":null}}`, false},
		{`{"id":9007199254740992}`, false},
		{`{"msg":{"roles":["owner","peer"]}}`, false},
	} {
		id, err := expect(bufio.NewReader(strings.NewReader(line+"\n")), []byte(tt.want))
		if (err == nil) != tt.ok || (tt.ok && string(id["id"]) != "9007199254740993") {
			t.Errorf("expect(%s): got %s, %v", tt.want, id["id"], err)
		}
	}
}
