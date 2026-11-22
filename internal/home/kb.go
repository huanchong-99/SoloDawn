package home

import "strings"

// Knowledge base: a note of type kb is written by anyone through the typed
// note write of preset.go, and fetch kind kb with q searches the documents
// (planning/contracts/step-3/knowledge.md sections 2, 3 and 6).

const kbType = "kb"

func kbSearchOK(req Fetch) bool {
	return req.Kind == kbType && req.Before == 0 && fetPageOK(req) && strings.TrimSpace(*req.Query) != ""
}
