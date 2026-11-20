package store

import "encoding/json"

// Outcome contains only replayable fields: notably no request id,
// pairing secret or terminal handle.
type Outcome struct {
	OK     bool   `json:"ok,omitempty"`
	Num    int    `json:"num,omitempty"`
	Passed *bool  `json:"passed,omitempty"`
	Round  int    `json:"round,omitempty"`
	Code   string `json:"code,omitempty"`
}

func (tx *Tx) PutReceipt(signer, nonce string, ts int64, result Outcome) error {
	data, _ := json.Marshal(result) // only booleans, integers and strings
	_, err := tx.Exec("INSERT INTO receipt(k,n,ts,answer) VALUES(?,?,?,?)", signer, nonce, ts, string(data))
	return err
}

// Receipt returns sql.ErrNoRows if the command has not been accepted.
func (tx *Tx) Receipt(signer, nonce string) (Outcome, error) {
	var result Outcome
	var data []byte
	if err := tx.QueryRow("SELECT answer FROM receipt WHERE k=? AND n=?", signer, nonce).Scan(&data); err != nil {
		return result, err
	}
	err := json.Unmarshal(data, &result)
	return result, err
}
