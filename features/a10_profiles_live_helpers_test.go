//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestA10LiveChoiceValidation(t *testing.T) {
	for _, tc := range []struct {
		name, agent string
		choice      a10LiveChoice
		want        string
	}{
		{"listed", "claude", a10LiveChoice{Model: "haiku", ReportedModel: "resolved-haiku"}, ""},
		{"manual qwen", "qwen", a10LiveChoice{Model: "manual-id", ReportedModel: "resolved-id"}, ""},
		{"empty model", "claude", a10LiveChoice{ReportedModel: "resolved-id"}, "model and expected reportedModel must be non-empty"},
		{"empty report", "claude", a10LiveChoice{Model: "haiku"}, "model and expected reportedModel must be non-empty"},
		{"unlisted", "claude", a10LiveChoice{Model: "other", ReportedModel: "other"}, `chosen model "other" is absent from claude's discovered list`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := a10ValidateLiveChoice(tc.agent, []proto.Model{{Model: "haiku"}}, tc.choice)
			var got string
			if err != nil {
				got = err.Error()
			}
			if got != tc.want {
				t.Fatalf("error = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestA10LiveEventResult(t *testing.T) {
	for _, tc := range []struct {
		name       string
		event      host.Event
		open, done bool
		want       string
	}{
		{"ready", host.Event{Kind: host.Ready, Session: "live-session"}, true, true, ""},
		{"active", host.Event{Kind: host.Active}, true, false, ""},
		{"completed", host.Event{Kind: host.TurnEnded, Status: "completed"}, true, false, ""},
		{"closed", host.Event{}, false, true, "CLI exited before ready"},
		{"login", host.Event{Kind: host.LoginInvalid, Text: "sign in"}, true, true, "CLI login_invalid: sign in"},
		{"missing session", host.Event{Kind: host.NoSession, Text: "missing"}, true, true, "CLI no_session: missing"},
		{"failed", host.Event{Kind: host.TurnEnded, Status: "failed", Text: "refused"}, true, true, `CLI turn ended "failed": refused`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done, err := a10LiveEventResult(tc.event, tc.open, host.Ready)
			var got string
			if err != nil {
				got = err.Error()
			}
			if done != tc.done || got != tc.want {
				t.Fatalf("got (%t, %q); want (%t, %q)", done, got, tc.done, tc.want)
			}
		})
	}
}

func TestA10LiveAwaitEvent(t *testing.T) {
	events := make(chan host.Event, 2)
	events <- host.Event{Kind: host.Active}
	want := host.Event{Kind: host.Ready, Session: "live-session"}
	events <- want
	got, err := a10AwaitLiveEvent(t.Context(), events, host.Ready)
	if err != nil || got != want {
		t.Fatalf("got (%+v, %v); want (%+v, nil)", got, err, want)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = a10AwaitLiveEvent(ctx, make(chan host.Event), host.Ready)
	if err != context.Canceled {
		t.Fatalf("got %v; want context.Canceled", err)
	}
	close(events)
	_, err = a10AwaitLiveEvent(t.Context(), events, host.Ready)
	if err == nil || err.Error() != "CLI exited before ready" {
		t.Fatalf("unexpected closed-stream result: %v", err)
	}
}

func TestA10LiveObservation(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		invalid           bool
	}{
		{"report", `"resolved-id"`, "resolved-id", false},
		{"empty stream", "", "", true},
		{"malformed", "{", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &a10LiveWorld{agent: "claude", input: json.NewDecoder(strings.NewReader(tc.input))}
			var got string
			err := w.observe("unit observation", &got)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got (%q, %v); want (%q, invalid=%t)", got, err, tc.want, tc.invalid)
			}
		})
	}
}

func TestA10LivePickerComparison(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		models      []proto.Model
		invalid     bool
	}{
		{"listed", `[{"model":"one","name":"One","efforts":[],"fast":true}]`, []proto.Model{{Model: "one", Name: "One", Efforts: []string{}, Fast: true}}, false},
		{"empty", `[]`, []proto.Model{}, false},
		{"different picker", `[{"model":"one","name":"One","efforts":[],"fast":false}]`, []proto.Model{}, true},
		{"unknown field", `[{"model":"one","extra":true}]`, []proto.Model{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := json.NewDecoder(strings.NewReader(tc.input))
			input.DisallowUnknownFields()
			w := &a10LiveWorld{agent: "claude", models: tc.models, input: input}
			err := w.picker()
			if (err != nil) != tc.invalid {
				t.Fatalf("error = %v; want invalid=%t", err, tc.invalid)
			}
		})
	}
}
