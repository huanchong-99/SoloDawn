package home_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func devAddRow(t *testing.T, signer, user identity.Key, ver int, dev identity.Key, name string) proto.DeviceRow {
	t.Helper()
	p, err := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: proto.ID(id(byte(40 + ver))), Timestamp: clock},
		DeviceChange:  proto.DeviceChange{User: user.ID(), Version: ver, Op: "add", Device: dev.ID(), Kind: "browser", Name: name},
	})
	if err != nil {
		t.Fatal(err)
	}
	return proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", user.ID(), ver)},
		DeviceChange: proto.DeviceChange{User: user.ID(), Version: ver, Op: "add", Device: dev.ID(), Kind: "browser", Name: name},
		Env:          signer.Sign(string(p)),
	}
}

func devRevokeRow(t *testing.T, signer, user identity.Key, ver int, dev identity.Key) proto.DeviceRow {
	t.Helper()
	p, err := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: proto.ID(id(byte(60 + ver))), Timestamp: clock},
		DeviceChange:  proto.DeviceChange{User: user.ID(), Version: ver, Op: "revoke", Device: dev.ID()},
	})
	if err != nil {
		t.Fatal(err)
	}
	return proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", user.ID(), ver)},
		DeviceChange: proto.DeviceChange{User: user.ID(), Version: ver, Op: "revoke", Device: dev.ID()},
		Env:          signer.Sign(string(p)),
	}
}

func TestDeviceEventFiltersAndAppendsNewOwnerRows(t *testing.T) {
	handle := home.Events["device"]
	s := home.NewState(groupID, node.ID(), browser.ID())
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: groupID}, Name: "Website"}
	s.People[id(30)] = person(id(30), guest, "member")

	equal(t, len(handle(s, nil, clock, "wrong").Rows), 0)
	equal(t, len(handle(s, nil, clock, []proto.DeviceRow{}).Rows), 0)
	equal(t, len(handle(s, nil, clock, []proto.DeviceRow{devAddRow(t, guest, guest, 1, guest, "g")}).Rows), 0)

	r1 := devAddRow(t, browser, browser, 1, browser, "phone")
	res := handle(s, nil, clock, []proto.DeviceRow{r1})
	equal(t, len(res.Rows), 1)
	equal(t, res.GroupsChanged, false)
	s.Commit(s.Seal(res))

	// Version equal to held is skipped; version held+1 for an online unserved device flips GroupsChanged.
	s.Online[laptop.ID()] = true
	r2 := devAddRow(t, browser, browser, 2, laptop, "laptop")
	res = handle(s, nil, clock, []proto.DeviceRow{r1, r2})
	equal(t, len(res.Rows), 1)
	equal(t, res.Rows[0].Value().(proto.DeviceRow).ID, browser.ID()+":2")
	equal(t, res.GroupsChanged, true)
	s.Commit(s.Seal(res))

	// Renaming an already-served online device keeps GroupsChanged false.
	r3 := devAddRow(t, browser, browser, 3, laptop, "laptop-renamed")
	res = handle(s, nil, clock, []proto.DeviceRow{r3})
	equal(t, len(res.Rows), 1)
	equal(t, res.GroupsChanged, false)
	s.Commit(s.Seal(res))

	// Revoking an online device that was never added keeps GroupsChanged false.
	s.Online[nobody.ID()] = true
	r4 := devRevokeRow(t, browser, browser, 4, nobody)
	res = handle(s, nil, clock, []proto.DeviceRow{r4})
	equal(t, len(res.Rows), 1)
	equal(t, res.GroupsChanged, false)
	s.Commit(s.Seal(res))

	// Revoking an online served device flips GroupsChanged true; offline revoke keeps it false.
	r5 := devRevokeRow(t, browser, browser, 5, laptop)
	res = handle(s, nil, clock, []proto.DeviceRow{r5})
	equal(t, len(res.Rows), 1)
	equal(t, res.GroupsChanged, true)

	delete(s.Online, laptop.ID())
	resOffline := handle(s, nil, clock, []proto.DeviceRow{r5})
	equal(t, len(resOffline.Rows), 1)
	equal(t, resOffline.GroupsChanged, false)
}
