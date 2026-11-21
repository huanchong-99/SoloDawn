package home

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
	"weak"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Daily retention in the home (planning/contracts/storage.md sections 2 and 4).
// The timer runs at most once every 24 hours per group state and returns a
// Write so the rule stays pure; the hook records the run time only after the
// transaction commits.

// RetainEvent names the retention rule in Timers, Events and After.
const RetainEvent = "retention"

const (
	retDayMS       = 86_400_000
	retSettingKey  = "retention_days"
	retDefaultDays = 90
)

// retMu guards retRuns only and calls nothing while held, so both the store's
// writer (retDaily) and a group's goroutine (retNoted) may take it without
// waiting on any other lock.
var (
	retMu   sync.Mutex
	retRuns = map[weak.Pointer[State]]int64{}
)

func init() {
	Timers.Add(RetainEvent, retDaily)
	Events.Add(RetainEvent, retRunNow)
	After.Add(RetainEvent, retNoted)
}

// RetainWrite returns a Write that reads retention_days from setting (90 when
// absent) and expires old store data at now, in Unix milliseconds.
func RetainWrite(now int64) Write {
	return retWrite(now)
}

type retWrite int64

func (w retWrite) Put(tx *store.Tx) error {
	days, err := retDays(tx)
	if err != nil {
		return err
	}
	return tx.Retain(time.UnixMilli(int64(w)), days)
}

func retDaily(s *State, _ Disk, now int64) Result {
	if last, ran := retLast(s); ran && now < last+retDayMS {
		return Result{}
	}
	return Result{Writes: []Write{RetainWrite(now)}}
}

func retRunNow(_ *State, _ Disk, now int64, _ any) Result {
	return Result{Writes: []Write{RetainWrite(now)}}
}

func retNoted(s *State, res Result) {
	for _, w := range res.Writes {
		if rw, ok := w.(retWrite); ok {
			retRecord(s, int64(rw))
		}
	}
}

func retLast(s *State) (int64, bool) {
	retMu.Lock()
	defer retMu.Unlock()
	last, ok := retRuns[weak.Make(s)]
	return last, ok
}

func retRecord(s *State, now int64) {
	retMu.Lock()
	defer retMu.Unlock()
	retRuns[weak.Make(s)] = now
}

func retDays(tx *store.Tx) (int, error) {
	var raw string
	err := tx.QueryRow("SELECT value FROM setting WHERE key=?", retSettingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return retDefaultDays, nil
	}
	if err != nil {
		return 0, err
	}
	return retParseDays(raw)
}

func retParseDays(raw string) (int, error) {
	var days int
	if err := proto.Decode([]byte(raw), &days); err != nil {
		return 0, fmt.Errorf("setting %s: %w", retSettingKey, err)
	}
	if days < 1 {
		return 0, fmt.Errorf("setting %s must be at least 1, got %d", retSettingKey, days)
	}
	return days, nil
}
