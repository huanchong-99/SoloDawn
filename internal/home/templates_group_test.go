package home_test

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestCustomGroupTemplatesStoredAndUpdated(t *testing.T) {
	p := grpCreatePayload(groupID, groupID)
	p.Forge = "custom"
	p.Templates = &proto.ForgeTemplates{Push: []string{"forge"}, Open: []string{"forge"}, Checks: []string{"forge"}, Merge: []string{"forge"}}
	s := empty(t)
	res := grpApply(t, s, disk{}, receipts{}, browser, p)
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	equal(t, s.Group.Templates, p.Templates)
	set := grpSetCmd(187)
	set.Templates = &proto.ForgeTemplates{Push: []string{"new-forge"}, Open: []string{"new-forge"}, Checks: []string{"new-forge"}, Merge: []string{"new-forge"}}
	res = grpApply(t, s, disk{}, receipts{}, browser, set)
	equal(t, res.Answer, store.Outcome{OK: true})
	s.Commit(res)
	equal(t, s.Group.Templates, set.Templates)
	set.Templates.Push = []string{"forge", "{title}"}
	res = grpApply(t, s, disk{}, receipts{}, browser, set)
	equal(t, res.Answer.Code, "bad_request")
	res = grpApply(t, group(t), disk{}, receipts{}, browser, set)
	equal(t, res.Answer.Code, "bad_request")
	p.Forge = "github"
	res = grpApply(t, empty(t), disk{}, receipts{}, browser, p)
	equal(t, res.Answer.Code, "bad_request")
}
