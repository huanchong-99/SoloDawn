package home

import (
	"encoding/json"
	"errors"
	"maps"

	"github.com/huanchong-99/SoloDawn/internal/store"
)

func (l *Loop) loadMoved() error {
	return l.cfg.Store.Write(func(tx *store.Tx) error {
		raw, err := tx.MovedGroups()
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &l.moved)
	})
}

func (l *Loop) replyMoved(c *Conn, re int64, group string) bool {
	l.mu.Lock()
	to := l.moved[group]
	l.mu.Unlock()
	if to == "" {
		return false
	}
	reply := runErr(re, "moved", "The group has moved.")
	reply.Err.Home = to
	c.send(reply)
	return true
}

func (l *Loop) askHome(c *Conn, re int64, group string, step func(*runGroup)) {
	answer := func(g *runGroup) {
		if l.replyMoved(c, re, group) {
			return
		}
		if g == nil {
			c.send(runErr(re, "not_found", "this node is not home to the group"))
			return
		}
		step(g)
	}
	if !l.ask(group, answer) {
		answer(nil)
	}
}

// ExportGroup takes the cut on the group's goroutine.
func (l *Loop) ExportGroup(group, file, to string) error {
	if to == l.cfg.Home {
		return errors.New("the group is already on this node")
	}
	err := errors.New("this node is not home to the group")
	l.ask(group, func(g *runGroup) {
		if g.state.Group.ID != "" {
			err = g.exportGroup(file, to)
		}
	})
	return err
}

func (g *runGroup) exportGroup(file, to string) error {
	l, group := g.loop, g.state.ID
	if err := l.cfg.Store.ExportGroup(group, file, to); err != nil || to == "" {
		return err
	}
	g.resetGroup(l.fresh(group))
	l.mu.Lock()
	l.moved[group] = to
	delete(l.views, group)
	l.mu.Unlock()
	l.announce()
	return nil
}

// ImportGroup validates and installs the group before advertising it.
func (l *Loop) ImportGroup(group, file string, load func(*store.Tx) error, installed func()) error {
	done := make(chan error, 1)
	if !l.route(group, func(g *runGroup) { done <- g.importGroup(file, load, installed) }) {
		return errors.New("the home is closed")
	}
	return <-done
}

func (g *runGroup) importGroup(file string, load func(*store.Tx) error, installed func()) error {
	l, group := g.loop, g.state.ID
	var state *State
	subs := map[string]wpushSub{}
	read := func(tx *store.Tx) error {
		var err error
		state, err = l.importState(tx, group)
		return errors.Join(err, wpushReadSubs(tx, subs), load(tx))
	}
	if err := l.cfg.Store.ImportGroup(group, file, l.cfg.OwnerFn(), l.cfg.Home, read); err != nil {
		return err
	}
	installed()
	wpushMu.Lock()
	maps.Copy(wpushFor(l.cfg.Home).subs, subs)
	wpushMu.Unlock()
	g.resetGroup(state)
	g.publish()
	l.mu.Lock()
	delete(l.moved, group)
	l.mu.Unlock()
	l.announce()
	g.deliver(l.cfg.Now())
	return nil
}

func (l *Loop) importState(tx *store.Tx, group string) (*State, error) {
	held, readErr := runReadGroup(tx, group)
	state, err := l.restoreState(held)
	err = errors.Join(readErr, err)
	if err == nil && state.Group.ID != group {
		return nil, errors.New("the file has no group row")
	}
	return state, err
}

func (g *runGroup) resetGroup(state *State) {
	state.Online = g.state.Online
	g.state, g.next = state, runNever
	for _, seat := range g.seats {
		*seat = runSeat{inbox: seat.inbox}
	}
}
