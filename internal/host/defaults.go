package host

import (
	"context"
	"encoding/json"
	"log"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	defaultsSettingSQL = `SELECT coalesce((SELECT value FROM setting WHERE key = 'defaults'), '{}')`
	defaultsUpsertSQL  = `INSERT INTO setting(key, value) VALUES('defaults', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`
)

func init() {
	Register(Surface, "agent.defaults", handleAgentDefaults)
}

func handleAgentDefaults(_ context.Context, h *Host, _ Owner, _ string, data json.RawMessage) (any, error) {
	cmd, err := decodeAgentDefaults(data)
	if err != nil {
		return nil, err
	}
	if err := writeAgentDefault(h.Store, cmd.Agent, cmd.Profile); err != nil {
		return nil, err
	}
	syncAgentDefaults(h, cmd.Agent)
	return nil, nil
}

func decodeAgentDefaults(data json.RawMessage) (proto.AgentDefaults, error) {
	var cmd proto.AgentDefaults
	if err := proto.Decode(data, &cmd); err != nil {
		return proto.AgentDefaults{}, &Error{Code: "bad_request", Message: err.Error()}
	}
	if !agtValidAgent(cmd.Agent) {
		return proto.AgentDefaults{}, &Error{Code: "bad_request", Message: "unknown agent: " + cmd.Agent}
	}
	if !proto.ValidProfile(cmd.Profile) {
		return proto.AgentDefaults{}, &Error{Code: "bad_request", Message: "invalid profile"}
	}
	return cmd, nil
}

func readDefaultsTx(tx *store.Tx) (map[string]proto.Profile, error) {
	var raw string
	if err := tx.QueryRow(defaultsSettingSQL).Scan(&raw); err != nil {
		return nil, err
	}
	defaults := map[string]proto.Profile{}
	if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
		return nil, err
	}
	return defaults, nil
}

func readDefaults(db *store.Store) (map[string]proto.Profile, error) {
	var defaults map[string]proto.Profile
	read := func(tx *store.Tx) error {
		var err error
		defaults, err = readDefaultsTx(tx)
		return err
	}
	return defaults, db.Write(read)
}

func writeAgentDefault(db *store.Store, agent string, p proto.Profile) error {
	write := func(tx *store.Tx) error {
		defaults, err := readDefaultsTx(tx)
		if err != nil {
			return err
		}
		defaults[agent] = p
		encoded, _ := json.Marshal(defaults)
		_, err = tx.Exec(defaultsUpsertSQL, string(encoded))
		return err
	}
	return db.Write(write)
}

func defaultProfileOf(defaults map[string]proto.Profile, agent string) proto.Profile {
	if p, ok := defaults[agent]; ok {
		return p
	}
	return proto.Profile{Tier: "standard"}
}

func profileSetFrom(p proto.Profile) proto.ProfileSet {
	return proto.ProfileSet{
		Model:  &p.Model,
		Effort: &p.Effort,
		Tier:   &p.Tier,
		Skills: &p.Skills,
	}
}

func syncAgentDefaults(h *Host, agent string) {
	d := deliv(h)
	for _, group := range agentDefaultGroups(h, agent) {
		h.bg(d.wg.Go, "defaults", func() { followDefaults(h, group) }, "g", group)
	}
}

func agentDefaultGroups(h *Host, agent string) []string {
	seen := map[string]bool{}
	for _, s := range h.Sessions.All() {
		if s.Task == "" && s.Agent == agent {
			seen[s.Group] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

type followerEntry struct {
	session Session
	row     proto.AIMemberRow
}

func followDefaults(h *Host, group string) {
	followers := followerSessions(h, group)
	if len(followers) == 0 {
		return
	}
	defaults, err := readDefaults(h.Store)
	if err != nil {
		log.Printf("host: read defaults: %v", err)
		return
	}
	for _, f := range followers {
		syncFollowerSession(h, f.session, f.row, defaults)
	}
}

func followerSessions(h *Host, group string) []followerEntry {
	var out []followerEntry
	for _, s := range h.Sessions.All() {
		if !eligibleGroupSession(s, group) {
			continue
		}
		if ai, ok := lookupRow[proto.AIMemberRow](h, s.Group, "member", s.Member); ok && bool(ai.Follow) {
			out = append(out, followerEntry{session: s, row: ai})
		}
	}
	return out
}

func eligibleGroupSession(s Session, group string) bool {
	return s.Group == group && s.Task == "" && s.Agent != externalAgent
}

func syncFollowerSession(h *Host, s Session, ai proto.AIMemberRow, defaults map[string]proto.Profile) {
	want := defaultProfileOf(defaults, s.Agent)
	if ai.Profile == want {
		return
	}
	cmd := proto.MemberSet{
		MemberCommand: proto.MemberCommand{
			HomeCommand: homeHeader(h, s.Group, "", "member"),
			Op:          "set",
			ID:          proto.ID(s.Member),
		},
		ProfileSet: profileSetFrom(want),
		Follow:     true,
	}
	sendHostCommand(context.Background(), h, s.Home, marshalWire(cmd))
}
