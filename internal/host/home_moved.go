package host

import (
	"encoding/json"
	"log"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func pushHomeGroups(h *Host, device string, raw json.RawMessage) {
	var frame proto.Groups
	if proto.Decode(raw, &frame) != nil {
		return
	}
	present := make(map[string]bool, len(frame.Groups))
	for _, group := range frame.Groups {
		present[group.Group] = true
	}
	d := deliv(h)
	for _, group := range d.announcedGroups(device, present) {
		if !present[group] && ownerLeft(h, group) {
			h.bg(d.wg.Go, "group_left", func() { leaveGroup(h, d, group) }, "g", group)
		} else {
			ensureGroupSynced(h, d, device, group)
		}
	}
}

// ownerLeft reports whether the rows of a group that its home no longer lists
// show that the node's owner is no longer a member: a home pushes the rows of
// a removal first (contracts/step-2/people.md 2.4). The rows of a group that
// moved still have the owner.
func ownerLeft(h *Host, group string) bool {
	_, known := joinCachedGroup(h, group)
	owner, err := loadOwner(h.Store)
	if !known || err != nil {
		return false
	}
	isOwner := func(raw json.RawMessage) bool {
		var m proto.HumanMemberRow
		return proto.Decode(raw, &m) == nil && m.Kind == "human" && m.User == owner
	}
	return !slices.ContainsFunc(h.Rows.Of(group, "member"), isOwner)
}

// leaveGroup ends what this node has of a group its owner is no longer in: the
// sessions of its AIs, as after host.leave, the rows and the remembered home.
func leaveGroup(h *Host, d *delivery, group string) {
	for _, s := range h.Sessions.All() {
		if s.Group == group {
			leaveGoneMember(h, group, s.Member)
		}
	}
	joinCommitLeft(h, group, "", "", "")
	d.forget(group)
	h.Rows.Reset(group)
	joinSyncNow(h, joinStateOf(h))
}

func (d *delivery) announcedGroups(device string, present map[string]bool) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var sync []string
	for group, gs := range d.groups {
		if gs.home != device || gs.live == present[group] && !gs.syncing {
			continue
		}
		if gs.syncing {
			gs.refresh, gs.unlisted = true, !present[group]
		} else {
			gs.live = false
			sync = append(sync, group)
		}
	}
	return sync
}

func settleHomeSync(h *Host, d *delivery, device, group string, answer json.RawMessage, ok, refresh bool) {
	if ok {
		h.bg(d.wg.Go, "group_state", func() { afterGroupState(h, d, group) }, "g", group)
	} else if settleRefusedSync(h, d, device, group, answer) {
		return
	}
	if refresh {
		ensureGroupSynced(h, d, device, group)
	}
}

// settleRefusedSync acts on a home's refusal to show a group: it has moved, or
// the node's owner is not a member.
func settleRefusedSync(h *Host, d *delivery, device, group string, answer json.RawMessage) bool {
	if target := movedHome(answer); target != "" {
		if err := followMovedHome(h, device, group, target); err != nil {
			log.Printf("host: follow moved group: %v", err)
		}
		return true
	}
	var refused proto.ErrorResponse
	if proto.Decode(answer, &refused) != nil || refused.Err.Code != "forbidden" || d.groupHome(group) != device {
		return false
	}
	leaveGroup(h, d, group)
	return true
}

func movedHome(answer json.RawMessage) string {
	var refused proto.ErrorResponse
	if proto.Decode(answer, &refused) == nil && refused.Err.Code == "moved" && identity.IsDevice(refused.Err.Home) {
		return refused.Err.Home
	}
	return ""
}

func followMovedHome(h *Host, old, group, target string) error {
	homes, err := joinSavedHomes(h.Store)
	if err != nil {
		return err
	}
	for _, entry := range homes {
		if entry.Group == proto.ID(group) && entry.Home == old {
			addrs, err := movedHomeAddrs(h.Store, old)
			if err != nil {
				return err
			}
			entry.Home = target
			return h.RecordHome(entry, addrs)
		}
	}
	return nil
}

func movedHomeAddrs(db *store.Store, device string) (addrs []string, err error) {
	err = db.Write(func(tx *store.Tx) error {
		rows, err := tx.Query("SELECT url FROM peer WHERE device=? ORDER BY url", device)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var addr string
			if err := rows.Scan(&addr); err != nil {
				return err
			}
			addrs = append(addrs, addr)
		}
		return rows.Err()
	})
	return addrs, err
}
