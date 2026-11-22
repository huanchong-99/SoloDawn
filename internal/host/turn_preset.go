package host

import (
	"context"
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func (c *turnCtx) appendTaskPreset(lines []string, name string) ([]string, error) {
	if name == "" {
		return lines, nil
	}
	text, ok, err := c.fetchPresetText(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return append(lines, "Style preset \""+name+"\": deleted since the brief was proposed."), nil
	}
	return append(lines, "Style preset \""+name+"\":", text), nil
}

func appendReviewPreset(lines []string, name string) []string {
	if name == "" {
		return lines
	}
	return append(lines, "Style preset: \""+name+"\"")
}

func (c *turnCtx) fetchPresetText(name string) (string, bool, error) {
	for before := int64(0); ; {
		page, err := c.fetchPresetPage(before)
		if err != nil {
			return "", false, err
		}
		text, found, next, err := scanPresetPage(page.Rows, name)
		if stopPresetScan(found, err, page) {
			return text, found, err
		}
		before = next
	}
}

func stopPresetScan(found bool, err error, page proto.Fetched) bool {
	return found || err != nil || !page.More
}

func (c *turnCtx) fetchPresetPage(before int64) (proto.Fetched, error) {
	build := func(id int64) any {
		return proto.FetchKind{FetchPage: c.fetchPage(id, before, notePageLimit), Kind: "preset"}
	}
	return fetchFrom(context.Background(), c.h, c.s.Home, build)
}

func scanPresetPage(rawRows []json.RawMessage, name string) (string, bool, int64, error) {
	var before int64
	for _, raw := range rawRows {
		var n proto.NoteRow
		if proto.Decode(raw, &n) != nil {
			return "", false, 0, &Error{"bad_request", "invalid preset row"}
		}
		if n.Name == name {
			return n.Text, true, n.Pos, nil
		}
		before = n.Pos
	}
	return "", false, before, nil
}
