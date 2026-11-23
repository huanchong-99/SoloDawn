package home

func externalToolValidate(cli string, tool *string, clear bool) *Refusal {
	if tool == nil {
		return nil
	}
	if cli != "external" {
		return &Refusal{"bad_request", "tool is only for an external AI"}
	}
	if (!clear && *tool == "") || !boundedMemberText(tool, 32) {
		return &Refusal{"bad_request", "tool must have 1 to 32 code points without control characters"}
	}
	return nil
}
