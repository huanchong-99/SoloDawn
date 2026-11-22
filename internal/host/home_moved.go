package host

import (
	"encoding/json"
	"log"

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
		ensureGroupSynced(h, d, device, group)
	}
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
			gs.refresh = true
		} else {
			gs.live = false
			sync = append(sync, group)
		}
	}
	return sync
}

func settleHomeSync(h *Host, d *delivery, device, group string, answer json.RawMessage, ok, refresh bool) {
	if ok {
		d.wg.Go(func() { afterGroupState(h, d, group) })
	} else if target := movedHome(answer); target != "" {
		if err := followMovedHome(h, device, group, target); err != nil {
			log.Printf("host: follow moved group: %v", err)
		}
		return
	}
	if refresh {
		ensureGroupSynced(h, d, device, group)
	}
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
