package proto

import "testing"

func TestNewNonce(t *testing.T) {
	a, b := NewNonce(), NewNonce()
	if a == b {
		t.Fatalf("duplicate nonces %q", a)
	}
	var got struct {
		N ID `json:"n"`
	}
	if err := Decode([]byte(`{"n":"`+string(a)+`"}`), &got); err != nil || got.N != a {
		t.Fatalf("got %+v, %v", got, err)
	}
}
