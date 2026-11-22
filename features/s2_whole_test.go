//go:build acceptance

package features

import "fmt"

func (w *m5World) verifyNoConfirmCard() error {
	for _, row := range w.browserMessages() {
		if row.Kind == "confirm" {
			return fmt.Errorf("owner saw unexpected confirm card %s", row.ID)
		}
	}
	return nil
}
