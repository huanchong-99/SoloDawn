package proto

import (
	"encoding/json"
	"reflect"
	"testing"
)

// There is one system golden. Check the distinct wire fields of its other codes here.
func TestSystemBodies(t *testing.T) {
	for _, tc := range []struct {
		body any
		want string
	}{
		{RoundsLimitBody{SystemCode{"rounds_limit"}, "t", 3}, `{"code":"rounds_limit","task":"t","n":3}`},
		{GroupPauseBody{SystemCode{"group_paused"}, "m"}, `{"code":"group_paused","by":"m"}`},
		{GroupPauseBody{SystemCode{"group_resumed"}, "m"}, `{"code":"group_resumed","by":"m"}`},
		{PreemptedBody{SystemCode{"preempted"}, "t", "m"}, `{"code":"preempted","task":"t","by":"m"}`},
		{CapReachedBody{SystemCode{"cap_reached"}, "a", "day", new("t")}, `{"code":"cap_reached","ai":"a","cap":"day","task":"t"}`},
		{RateLimitedBody{SystemCode{"rate_limited"}, "a", 42}, `{"code":"rate_limited","ai":"a","resets":42}`},
		{LoginInvalidBody{SystemCode{"login_invalid"}, "a", new("signed out")}, `{"code":"login_invalid","ai":"a","detail":"signed out"}`},
		{TaskFailedBody{SystemCode{"task_failed"}, "t", "failures"}, `{"code":"task_failed","task":"t","reason":"failures"}`},
		{TaskMergedBody{SystemCode{"task_merged"}, "t", "abc", "def"}, `{"code":"task_merged","task":"t","commit":"abc","merge_commit":"def"}`},
		{SystemCode{"no_secretary"}, `{"code":"no_secretary"}`},
		{SessionReplacedBody{SystemCode{"session_replaced"}, "a", nil}, `{"code":"session_replaced","ai":"a"}`},
	} {
		if got := jsonValue(t, wireJSON(t, tc.body)); !reflect.DeepEqual(got, jsonValue(t, []byte(tc.want))) {
			t.Errorf("body = %v, want %s", got, tc.want)
		}
		got := reflect.New(reflect.TypeOf(tc.body))
		if err := json.Unmarshal([]byte(tc.want), got.Interface()); err != nil || !reflect.DeepEqual(got.Elem().Interface(), tc.body) {
			t.Errorf("decode %s = %v, %v", tc.want, got, err)
		}
	}
}
