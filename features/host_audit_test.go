//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_audit"] = hostAuditSteps
}

type hostAuditState struct {
	db     *store.Store
	h      *host.Host
	nowMS  int64
	rawRes []byte
	callEr error
}

func (s *hostAuditState) open(cleanup func(func())) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize)))
	s.nowMS = 1_763_600_000_000
	h, _, err := host.New(db, key, func() time.Time { return time.UnixMilli(s.nowMS) })
	if err != nil {
		return err
	}
	s.db, s.h = db, h
	return nil
}

func (s *hostAuditState) writeEntry(row host.AuditRow, ts int64) error {
	s.nowMS = ts
	write := func(tx *store.Tx) error {
		written, err := s.h.Audit(tx, row)
		if err != nil {
			return err
		}
		if !written {
			return errors.New("audit row was not written")
		}
		return nil
	}
	return s.db.Write(write)
}

func (s *hostAuditState) callAudit(raw string) error {
	handle := host.Surface["audit"]
	if handle == nil {
		return errors.New("Surface[\"audit\"] is nil")
	}
	res, err := handle(context.Background(), s.h, nil, "owner-device", json.RawMessage(raw))
	s.callEr = err
	s.rawRes = nil
	if err != nil {
		return nil
	}
	encoded, marshalErr := json.Marshal(res)
	if marshalErr != nil {
		return marshalErr
	}
	s.rawRes = encoded
	return nil
}

func (s *hostAuditState) decodeAnswer(re int64) (proto.AuditAnswer, error) {
	if s.callEr != nil {
		return proto.AuditAnswer{}, fmt.Errorf("unexpected audit error: %v", s.callEr)
	}
	var fields struct {
		Rows json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(s.rawRes, &fields); err != nil {
		return proto.AuditAnswer{}, err
	}
	wire, err := json.Marshal(struct {
		Re   int64           `json:"re"`
		OK   bool            `json:"ok"`
		Rows json.RawMessage `json:"rows"`
	}{Re: re, OK: true, Rows: fields.Rows})
	if err != nil {
		return proto.AuditAnswer{}, err
	}
	if nulls := proto.UnexpectedNulls(wire); len(nulls) != 0 {
		return proto.AuditAnswer{}, fmt.Errorf("unexpected nulls in %s: %v", wire, nulls)
	}
	var ans proto.AuditAnswer
	if err := proto.Decode(wire, &ans); err != nil {
		return proto.AuditAnswer{}, err
	}
	return ans, nil
}

func hostAuditSteps(sc *godog.ScenarioContext, _ fixtures) {
	cleanup := afterScenario(sc)
	st := &hostAuditState{}

	sc.Step(`^an empty audit host store$`, func() error {
		return st.open(cleanup)
	})

	sc.Step(`^the audit host records an? "([^"]+)" entry in group "([^"]*)" for agent "([^"]*)" with detail "([^"]*)" by "([^"]*)" with result "([^"]*)" at (\d+)$`,
		func(what, group, agent, detail, by, result string, ts int64) error {
			return st.writeEntry(host.AuditRow{
				Group:  group,
				Agent:  agent,
				What:   what,
				Detail: detail,
				By:     by,
				Result: result,
			}, ts)
		})

	sc.Step(`^(\d+) setting audit entries recorded at identical timestamp (\d+)$`, func(count int, ts int64) error {
		for i := 1; i <= count; i++ {
			row := host.AuditRow{What: host.AuditSetting, Detail: fmt.Sprintf("setting-%d", i)}
			if err := st.writeEntry(row, ts); err != nil {
				return err
			}
		}
		return nil
	})

	sc.Step("^an owner device sends audit surface request `([^`]+)`$", func(raw string) error {
		return st.callAudit(raw)
	})

	sc.Step(`^the audit surface answers request (\d+) with (\d+) rows:$`, func(re int64, wantCount int, table *godog.Table) error {
		ans, err := st.decodeAnswer(re)
		if err != nil {
			return err
		}
		if len(ans.Rows) != wantCount || len(table.Rows)-1 != wantCount {
			return fmt.Errorf("got %d rows, want %d", len(ans.Rows), wantCount)
		}
		for i, cells := range table.Rows[1:] {
			wantID, err := strconv.ParseInt(cells.Cells[0].Value, 10, 64)
			if err != nil {
				return err
			}
			wantTS, err := strconv.ParseInt(cells.Cells[1].Value, 10, 64)
			if err != nil {
				return err
			}
			want := proto.AuditRow{
				ID:     wantID,
				TS:     wantTS,
				Group:  proto.ID(cells.Cells[2].Value),
				AI:     cells.Cells[3].Value,
				What:   cells.Cells[4].Value,
				Detail: cells.Cells[5].Value,
				By:     cells.Cells[6].Value,
				Result: cells.Cells[7].Value,
			}
			if ans.Rows[i] != want {
				return fmt.Errorf("row %d: got %+v, want %+v", i, ans.Rows[i], want)
			}
		}
		return nil
	})

	sc.Step(`^paging the audit surface with limit (\d+) from before (\d+) yields row ids "([^"]*)"$`, func(limit int, before int64, wantCSV string) error {
		if err := st.callAudit(fmt.Sprintf(`{"t":"audit","id":1,"before":%d,"limit":%d}`, before, limit)); err != nil {
			return err
		}
		ans, err := st.decodeAnswer(1)
		if err != nil {
			return err
		}
		parts := make([]string, 0, len(ans.Rows))
		for _, r := range ans.Rows {
			parts = append(parts, strconv.FormatInt(r.ID, 10))
		}
		if got := strings.Join(parts, ","); got != wantCSV {
			return fmt.Errorf("before=%d limit=%d: got ids %q, want %q", before, limit, got, wantCSV)
		}
		return nil
	})

	sc.Step("^the audit surface raw result is `([^`]+)`$", func(want string) error {
		if _, err := st.decodeAnswer(9); err != nil {
			return err
		}
		if string(st.rawRes) != want {
			return fmt.Errorf("got %s, want %s", st.rawRes, want)
		}
		return nil
	})

	sc.Step("^the audit surface outcome is `([^`]+)`$", func(want string) error {
		if st.callEr != nil {
			var hostErr *host.Error
			if !errors.As(st.callEr, &hostErr) {
				return fmt.Errorf("expected *host.Error, got %v", st.callEr)
			}
			if hostErr.Code != want {
				return fmt.Errorf("got code %q, want %q", hostErr.Code, want)
			}
			return nil
		}
		ans, err := st.decodeAnswer(1)
		if err != nil {
			return err
		}
		got := fmt.Sprintf("ok:%d", len(ans.Rows))
		if got != want {
			return fmt.Errorf("got %q, want %q", got, want)
		}
		return nil
	})
}
