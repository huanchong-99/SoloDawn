package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func audCallSurface(t *testing.T, h *Host, raw string) (audResult, error) {
	t.Helper()
	handle := Surface["audit"]
	if handle == nil {
		t.Fatal("audit handler is not registered in Surface")
	}
	got, err := handle(context.Background(), h, nil, "owner-device", json.RawMessage(raw))
	if err != nil {
		if got != nil {
			t.Fatalf("got non-nil result %+v alongside error %v", got, err)
		}
		return audResult{}, err
	}
	res, ok := got.(audResult)
	if !ok {
		t.Fatalf("got result type %T, want audResult", got)
	}
	return res, nil
}

func audWireAnswer(t *testing.T, re int64, res audResult) []byte {
	t.Helper()
	body, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		Rows json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	frame := struct {
		Re   int64           `json:"re"`
		OK   bool            `json:"ok"`
		Rows json.RawMessage `json:"rows"`
	}{Re: re, OK: true, Rows: fields.Rows}
	wire, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func TestAuditEmptyStoreEncodesEmptyArray(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	for _, raw := range []string{
		`{"t":"audit","id":6,"before":0,"limit":1}`,
		`{"t":"audit","id":7,"before":50,"limit":100}`,
	} {
		res, err := audCallSurface(t, h, raw)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", raw, err)
		}
		if res.Rows == nil || len(res.Rows) != 0 {
			t.Fatalf("%s: got %#v, want empty non-nil slice", raw, res.Rows)
		}
		encoded, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != `{"rows":[]}` {
			t.Fatalf("got %s, want {\"rows\":[]}", encoded)
		}
		wire := audWireAnswer(t, 6, res)
		if nulls := proto.UnexpectedNulls(wire); len(nulls) != 0 {
			t.Fatalf("unexpected nulls in %s: %v", wire, nulls)
		}
		var decoded proto.AuditAnswer
		if err := proto.Decode(wire, &decoded); err != nil {
			t.Fatalf("proto.Decode(%s): %v", wire, err)
		}
	}
}

func TestAuditMapsAllColumnsAndOmitsEmptyOptionals(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	const (
		groupID  = "rZNvy-1jH6Z-BcPqA5U5BQ"
		deviceID = "akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs"
	)
	seamAudit(t, h, AuditRow{
		What:   AuditSetting,
		Detail: "max_open 4",
		By:     deviceID,
	})
	seamAudit(t, h, AuditRow{
		Group:  groupID,
		Agent:  "codex",
		What:   AuditDelivered,
		Item:   4411,
		Home:   "home-device",
		Detail: "",
	})
	seamAudit(t, h, AuditRow{
		Group:  groupID,
		Agent:  "claude",
		What:   AuditApproval,
		Detail: "git push origin other-branch",
		By:     deviceID,
		Result: "approved",
	})

	res, err := audCallSurface(t, h, `{"t":"audit","id":10,"before":0,"limit":50}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []proto.AuditRow{
		{
			ID:     3,
			TS:     seamStart + 2000,
			Group:  proto.ID(groupID),
			AI:     "claude",
			What:   AuditApproval,
			Detail: "git push origin other-branch",
			By:     deviceID,
			Result: "approved",
		},
		{
			ID:     2,
			TS:     seamStart + 1000,
			Group:  proto.ID(groupID),
			AI:     "codex",
			What:   AuditDelivered,
			Detail: "",
		},
		{
			ID:     1,
			TS:     seamStart,
			What:   AuditSetting,
			Detail: "max_open 4",
			By:     deviceID,
		},
	}
	if !reflect.DeepEqual(res.Rows, want) {
		t.Fatalf("got %+v, want %+v", res.Rows, want)
	}

	wire := audWireAnswer(t, 10, res)
	if nulls := proto.UnexpectedNulls(wire); len(nulls) != 0 {
		t.Fatalf("unexpected nulls in %s: %v", wire, nulls)
	}
	var decoded proto.AuditAnswer
	if err := proto.Decode(wire, &decoded); err != nil {
		t.Fatalf("proto.Decode(%s): %v", wire, err)
	}
	if !reflect.DeepEqual(decoded.Rows, want) {
		t.Fatalf("decoded %+v, want %+v", decoded.Rows, want)
	}

	row1JSON, err := json.Marshal(res.Rows[2])
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{`"g"`, `"ai"`, `"result"`, `"item"`, `"home"`} {
		if strings.Contains(string(row1JSON), absent) {
			t.Fatalf("row 1 JSON %s contains %s", row1JSON, absent)
		}
	}
	row2JSON, err := json.Marshal(res.Rows[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{`"by"`, `"result"`, `"item"`, `"home"`} {
		if strings.Contains(string(row2JSON), absent) {
			t.Fatalf("row 2 JSON %s contains %s", row2JSON, absent)
		}
	}
	if !strings.Contains(string(row2JSON), `"detail":""`) {
		t.Fatalf("row 2 JSON %s omitted empty detail", row2JSON)
	}
}

func TestAuditPagesByIDAcrossIdenticalTimestamps(t *testing.T) {
	db := seamStore(t)
	fixedTS := int64(1_763_600_009_000)
	h, _, err := New(db, seamKey(), func() time.Time { return time.UnixMilli(fixedTS) })
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		seamAudit(t, h, AuditRow{
			What:   AuditSetting,
			Detail: fmt.Sprintf("step-%d", i),
		})
	}

	pages := []struct {
		before  int64
		wantIDs []int64
	}{
		{0, []int64{5, 4}},
		{4, []int64{3, 2}},
		{2, []int64{1}},
		{1, []int64{}},
	}
	for _, tc := range pages {
		raw := fmt.Sprintf(`{"t":"audit","id":1,"before":%d,"limit":2}`, tc.before)
		res, err := audCallSurface(t, h, raw)
		if err != nil {
			t.Fatalf("before=%d: %v", tc.before, err)
		}
		gotIDs := make([]int64, 0, len(res.Rows))
		for _, r := range res.Rows {
			if r.TS != fixedTS {
				t.Fatalf("row %d ts = %d, want %d", r.ID, r.TS, fixedTS)
			}
			gotIDs = append(gotIDs, r.ID)
		}
		if !reflect.DeepEqual(gotIDs, tc.wantIDs) {
			t.Fatalf("before=%d: got ids %v, want %v", tc.before, gotIDs, tc.wantIDs)
		}
	}
}

func TestAuditBoundsOnBeforeAndLimit(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	for i := 1; i <= 105; i++ {
		seamAudit(t, h, AuditRow{What: AuditSetting, Detail: fmt.Sprintf("n-%d", i)})
	}

	for _, bad := range []string{
		`{"t":"audit","id":1,"before":-1,"limit":1}`,
		`{"t":"audit","id":1,"before":-50,"limit":50}`,
		`{"t":"audit","id":1,"before":0,"limit":-1}`,
		`{"t":"audit","id":1,"before":0,"limit":0}`,
		`{"t":"audit","id":1,"before":0,"limit":101}`,
	} {
		_, err := audCallSurface(t, h, bad)
		seamCode(t, err, "bad_request")
	}

	one, err := audCallSurface(t, h, `{"t":"audit","id":2,"before":0,"limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Rows) != 1 || one.Rows[0].ID != 105 {
		t.Fatalf("limit=1: got %+v, want single row id 105", one.Rows)
	}

	hundred, err := audCallSurface(t, h, `{"t":"audit","id":3,"before":0,"limit":100}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(hundred.Rows) != 100 || hundred.Rows[0].ID != 105 || hundred.Rows[99].ID != 6 {
		t.Fatalf("limit=100: got %d rows (first=%d, last=%d), want 100 rows 105..6",
			len(hundred.Rows), hundred.Rows[0].ID, hundred.Rows[len(hundred.Rows)-1].ID)
	}

	tail, err := audCallSurface(t, h, `{"t":"audit","id":4,"before":6,"limit":100}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Rows) != 5 || tail.Rows[0].ID != 5 || tail.Rows[4].ID != 1 {
		t.Fatalf("before=6,limit=100: got %+v, want 5 rows 5..1", tail.Rows)
	}
}

func TestAuditRefusesMalformedAndWrongTypeFrames(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	for _, raw := range []string{
		`{`,
		`[]`,
		`null`,
		`{"t":"audit","id":1,"before":0}`,
		`{"t":"audit","id":1,"limit":10}`,
		`{"t":"audit","before":0,"limit":10}`,
		`{"t":"audit","id":1,"before":null,"limit":10}`,
		`{"t":"audit","id":1,"before":0,"limit":"10"}`,
		`{"t":"audit","id":1,"before":0,"limit":9007199254740992}`,
		`{"t":"audit","id":1,"Before":0,"limit":10}`,
		`{"t":"host.info","id":1,"before":0,"limit":10}`,
		`{"t":"","id":1,"before":0,"limit":10}`,
	} {
		_, err := audCallSurface(t, h, raw)
		seamCode(t, err, "bad_request")
	}
}

func TestAuditPropagatesStoreFailure(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seamSQL(t, h.Store, "DROP TABLE audit")
	_, err := audCallSurface(t, h, `{"t":"audit","id":1,"before":0,"limit":10}`)
	if err == nil || !strings.Contains(err.Error(), "no such table: audit") {
		t.Fatalf("got %v, want store error for missing audit table", err)
	}
	if errors.As(err, new(*Error)) {
		t.Fatalf("store failure wrapped as wire *Error (%v), want plain error for internal mapping", err)
	}
}
