package host

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type bulletinArgs struct {
	Group    *string  `json:"group,omitempty"`
	Text     string   `json:"text"`
	Mentions []string `json:"mentions,omitempty"`
}

func callBulletin(ctx context.Context, h *Host, c Caller, a bulletinArgs) (any, error) {
	if !c.Secretary {
		return nil, &Error{Code: "forbidden", Message: "only a secretary posts to a team group"}
	}
	home, err := bulletinHome(h, *a.Group)
	if err != nil {
		return nil, err
	}
	if err := bulletinTeam(ctx, h, home); err != nil {
		return nil, err
	}
	mentions, err := bulletinMentions(h.Rows, *a.Group, a.Mentions)
	if err != nil {
		return nil, err
	}
	return sendBulletin(ctx, h, c, a, home, mentions)
}

func sendBulletin(ctx context.Context, h *Host, c Caller, a bulletinArgs, home proto.Home, mentions []proto.ID) (any, error) {
	source, err := tskGroupRow(h, c.Group)
	if err != nil {
		return nil, err
	}
	hdr := secHomeHeader(h, c, "post")
	hdr.Group, hdr.As = proto.ID(*a.Group), ""
	payload := marshalWire(proto.Post[proto.BulletinSigned]{
		HomeCommand: hdr, Kind: "bulletin", Text: a.Text, Mentions: mentions,
		Body: proto.BulletinSigned{From: proto.ID(c.Group), Group: source.Name, AI: c.Name},
	})
	if _, err := h.Command(ctx, home.Home, payload); err != nil {
		return nil, err
	}
	return idResult{ID: string(hdr.Nonce)}, nil
}

func bulletinHome(h *Host, group string) (proto.Home, error) {
	homes, err := bulletinHomes(h.Store)
	if err != nil {
		return proto.Home{}, err
	}
	valid := []string{}
	for _, home := range homes {
		if string(home.Group) == group {
			return home, nil
		}
		valid = append(valid, fmt.Sprintf("%s (%s)", home.Group, home.Name))
	}
	slices.Sort(valid)
	return proto.Home{}, &Error{Code: "not_found", Message: "unknown group; valid groups: " + strings.Join(valid, ", ")}
}

func bulletinHomes(db *store.Store) ([]proto.Home, error) {
	var raw string
	err := db.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT json_group_array(json(home)) FROM (
			SELECT json_object('g', g, 'home', data->>'home', 'name', data->>'name') AS home
			FROM row WHERE r='group' AND gone=0
			UNION ALL
			SELECT value FROM json_each(coalesce((SELECT value FROM setting WHERE key='homes'), '[]'))
		)`).Scan(&raw)
	})
	if err != nil {
		return nil, err
	}
	var homes []proto.Home
	err = json.Unmarshal([]byte(raw), &homes)
	return homes, err
}

func bulletinTeam(ctx context.Context, h *Host, home proto.Home) error {
	group, ok := joinCachedGroup(h, string(home.Group))
	if !ok {
		conn, err := joinWaitHome(ctx, h, joinStateOf(h), home.Home)
		if err != nil {
			return err
		}
		group, err = joinFetchGroup(ctx, h, conn, home.Home, string(home.Group))
		if err != nil {
			return err
		}
	}
	if group.Kind != "team" {
		return &Error{Code: "conflict", Message: "the destination is not a team group"}
	}
	return nil
}

func bulletinMentions(rows *Rows, group string, names []string) ([]proto.ID, error) {
	dir := loadSecDirectory(rows, group)
	ids := []proto.ID{}
	for _, name := range names {
		person, err := dir.person(name)
		if err != nil {
			return nil, err
		}
		ids = appendUnique(ids, proto.ID(person.ID))
	}
	return ids, nil
}
