package store

const processedOffers = `CREATE TABLE processed_offer (
  signer TEXT NOT NULL,
  conn TEXT NOT NULL,
  expires_ms INTEGER NOT NULL,
  PRIMARY KEY (signer, conn)
) STRICT;`

// RememberOffer atomically claims the offer before any connection is retired.
func (s *Store) RememberOffer(signer, conn string, ts, now int64) (bool, error) {
	var inserted bool
	err := s.Write(func(tx *Tx) error {
		if _, err := tx.Exec("DELETE FROM processed_offer WHERE expires_ms < ?", now); err != nil {
			return err
		}
		r, err := tx.Exec("INSERT INTO processed_offer VALUES (?,?,?) ON CONFLICT DO NOTHING", signer, conn, ts+120000)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		inserted = n == 1
		return err
	})
	return inserted, err
}
func (s *Store) HasOffer(signer, conn string, now int64) (bool, error) {
	var found bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM processed_offer WHERE signer=? AND conn=? AND expires_ms>=?)", signer, conn, now).Scan(&found)
	return found, err
}
