package host

import (
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/store"
)

// seamAudit writes one row in a transaction of its own.
func seamAudit(t *testing.T, h *Host, row AuditRow) bool {
	t.Helper()
	var written bool
	err := h.Store.Write(func(tx *store.Tx) (err error) {
		written, err = h.Audit(tx, row)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return written
}

// seamRecord returns the audit record, a line per row, NULL written as "-".
func seamRecord(t *testing.T, h *Host) string {
	t.Helper()
	var record string
	err := h.Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT coalesce(group_concat(concat_ws('|', ts, coalesce(g, '-'), coalesce(agent, '-'), what,
			coalesce(item, '-'), coalesce(home, '-'), detail, coalesce(by, '-'), coalesce(result, '-')), char(10)), '')
			FROM (SELECT * FROM audit ORDER BY id)`).Scan(&record)
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestAuditHoldsAnInboxItemOnce(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	delivered := AuditRow{Group: "g1", Agent: "claude", What: AuditDelivered, Item: 4411, Home: "home-device", Detail: "assign"}
	if !seamAudit(t, h, delivered) {
		t.Fatal("the first row of an item was not written")
	}
	// The home sent the item again: nothing is written, whatever the row says.
	again := delivered
	again.What, again.Detail = AuditRefused, "sent twice"
	if seamAudit(t, h, again) {
		t.Fatal("an item that is in the record was written again")
	}
	// The same item id of another home, and another item of the same home,
	// are other items.
	elsewhere, next := delivered, delivered
	elsewhere.Home, next.Item = "other-home", 4412
	if !seamAudit(t, h, elsewhere) || !seamAudit(t, h, next) {
		t.Fatal("another item was taken for one in the record")
	}
	want := strings.Join([]string{
		"1763600000000|g1|claude|delivered|4411|home-device|assign|-|-",
		"1763600002000|g1|claude|delivered|4411|other-home|assign|-|-",
		"1763600003000|g1|claude|delivered|4412|home-device|assign|-|-",
	}, "\n")
	if got := seamRecord(t, h); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestAuditRowsWithoutAnItem(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	decided := AuditRow{Group: "g1", Agent: "codex", What: AuditApproval, Detail: "git push origin other-branch", By: "owner-device", Result: "approved"}
	// Rows that name no item never stand in each other's way.
	if !seamAudit(t, h, decided) || !seamAudit(t, h, decided) || !seamAudit(t, h, AuditRow{What: AuditSetting}) {
		t.Fatal("a row without an item was not written")
	}
	want := strings.Join([]string{
		"1763600000000|g1|codex|approval|-|-|git push origin other-branch|owner-device|approved",
		"1763600001000|g1|codex|approval|-|-|git push origin other-branch|owner-device|approved",
		"1763600002000|-|-|setting|-|-||-|-",
	}, "\n")
	if got := seamRecord(t, h); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	whats := []string{AuditDelivered, AuditRefused, AuditRequested, AuditApproval, AuditTakeover, AuditHandback, AuditMerge, AuditSetting}
	if got := strings.Join(whats, " "); got != "delivered refused requested approval takeover handback merge setting" {
		t.Fatalf("the constants spell %q, not the values of contracts/storage.md section 3", got)
	}
}

func TestAuditCommitsWithItsTransaction(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	failed := h.Store.Write(func(tx *store.Tx) error {
		if written, err := h.Audit(tx, AuditRow{What: AuditMerge, Detail: "t12 9f2c1ab"}); err != nil || !written {
			t.Fatalf("got %v and %v", written, err)
		}
		_, err := tx.Exec("INSERT INTO no_such_table VALUES(1)")
		return err
	})
	if failed == nil || seamRecord(t, h) != "" {
		t.Fatalf("a row outlived the transaction that failed (%v): %q", failed, seamRecord(t, h))
	}
	seamSQL(t, h.Store, "DROP TABLE audit")
	err := h.Store.Write(func(tx *store.Tx) error {
		written, err := h.Audit(tx, AuditRow{What: AuditMerge})
		if written {
			t.Fatal("a row that could not be stored counts as written")
		}
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "no such table: audit") {
		t.Fatalf("got %v, want the store's error", err)
	}
}
