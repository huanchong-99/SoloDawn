package host

import (
	"cmp"
	"context"
	"encoding/json"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var (
	kbPutSchema = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"text":{"type":"string"}},"required":["name","text"],"additionalProperties":false}`)

	kbSearchSchema = json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","default":3,"minimum":1,"maximum":20},"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)
)

const (
	kbDefaultLimit = 3
	kbMaxLimit     = 20
)

func init() {
	Register(Tools, "kb_put", Tool{
		Description: "Write or replace a named document in the group's knowledge base. A document of the same name is replaced whole and its old text is gone: search for it first and keep what you are not changing.",
		Schema:      kbPutSchema,
		Call:        callKBPut,
	})
	Register(Tools, "kb_search", Tool{
		Description: "Search the group's knowledge base and return whole matching documents.",
		Schema:      kbSearchSchema,
		Call:        callKBSearch,
	})
}

type kbPutArgs struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type kbPutResult struct {
	ID       string `json:"id"`
	Replaced bool   `json:"replaced"`
}

type kbSearchArgs struct {
	Query string `json:"query"`
	Limit *int   `json:"limit,omitempty"`
}

type kbDocumentItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
	By   string `json:"by"`
	At   int64  `json:"at"`
}

type kbSearchResult struct {
	Documents []kbDocumentItem `json:"documents"`
	More      bool             `json:"more"`
}

func callKBPut(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a kbPutArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	if err := checkKBPutSecrets(a); err != nil {
		return nil, err
	}
	cmd := proto.Note{
		HomeCommand: secHomeHeader(h, c, "note"),
		ID:          proto.NewNonce(),
		NoteType:    "kb",
		Name:        a.Name,
		Text:        a.Text,
	}
	ans, err := h.Command(ctx, c.Home, marshalWire(cmd))
	if err != nil {
		return nil, err
	}
	var res kbPutResult
	json.Unmarshal(ans, &res)
	return res, nil
}

func checkKBPutSecrets(a kbPutArgs) error {
	if err := CheckOutboundSecret(a.Name); err != nil {
		return err
	}
	return CheckOutboundSecret(a.Text)
}

func callKBSearch(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var a kbSearchArgs
	if err := decodeTalkArgs(raw, &a); err != nil {
		return nil, err
	}
	limit, err := kbSearchParams(a)
	if err != nil {
		return nil, err
	}
	return fetchKBSearch(ctx, h, c, a.Query, limit)
}

func kbSearchParams(a kbSearchArgs) (int, error) {
	if strings.TrimSpace(a.Query) == "" {
		return 0, badTalkRequest("query must not be empty")
	}
	limit := kbDefaultLimit
	if a.Limit != nil {
		limit = *a.Limit
	}
	if limit < 1 || limit > kbMaxLimit {
		return 0, badTalkRequest("limit must be 1 to 20")
	}
	return limit, nil
}

func fetchKBSearch(ctx context.Context, h *Host, c Caller, query string, limit int) (any, error) {
	page, err := fetchFrom(ctx, h, c.Home, func(id int64) any {
		return proto.FetchKind{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: 0, Limit: limit},
			Kind:      "kb",
			Query:     query,
		}
	})
	if err != nil {
		return nil, err
	}
	return kbSearchResult{
		Documents: decodeKBDocuments(page.Rows, memberNamesByID(h, c.Group)),
		More:      page.More,
	}, nil
}

func decodeKBDocuments(rows []json.RawMessage, names map[string]string) []kbDocumentItem {
	docs := make([]kbDocumentItem, len(rows))
	for i, rawRow := range rows {
		var row proto.NoteRow
		proto.Decode(rawRow, &row)
		docs[i] = kbDocumentItem{
			ID:   row.ID,
			Name: row.Name,
			Text: row.Text,
			By:   cmp.Or(names[row.By], row.By),
			At:   row.Timestamp,
		}
	}
	return docs
}
