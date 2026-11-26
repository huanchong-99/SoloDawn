package store

import (
	"database/sql"
	"time"
)

// Retain expires old data and advances each group's purge mark in the
// same transaction that removes its gone markers.
func (tx *Tx) Retain(now time.Time, days int) error {
	cut := now.AddDate(0, 0, -days)
	_, err := tx.Exec(`
UPDATE grp SET purged=max(purged, (SELECT coalesce(max(seq),0) FROM row
  WHERE row.g=grp.g AND gone=1 AND mt<:cut));
DELETE FROM row WHERE gone=1 AND mt<:cut;
DELETE FROM row WHERE r='msg' AND gone=0 AND keep=0 AND pending=0 AND ts<:cut
  AND NOT EXISTS (SELECT 1 FROM row AS t WHERE t.g=row.g AND t.r='task' AND t.final=0
    AND (row.id=t.id OR row.id=t.data->>'brief'
      OR (row.data->>'kind'='confirm' AND row.data->>'$.body.what'='direct'
        AND (row.data->>'re'=t.id OR row.data->>'re'=t.data->>'brief'))));
DELETE FROM row WHERE r='task' AND final=1 AND mt<:cut
  AND NOT EXISTS (SELECT 1 FROM row AS t, json_each(t.data,'$.depends') AS d
    WHERE t.g=row.g AND t.r='task' AND t.final=0 AND d.value=row.id);
DELETE FROM inbox WHERE created<:cut;
DELETE FROM audit WHERE ts<:cut;
DELETE FROM usage WHERE day<:day;
DELETE FROM peer WHERE ok_at<:cut;
DELETE FROM processed_offer WHERE expires_ms<:now;
DELETE FROM receipt WHERE ts<:receiptCut;`,
		sql.Named("cut", cut.UnixMilli()), sql.Named("day", cut.Format(time.DateOnly)),
		sql.Named("now", now.UnixMilli()),
		sql.Named("receiptCut", now.Add(-24*time.Hour).UnixMilli()))
	return err
}
