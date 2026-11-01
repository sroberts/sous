package recipes

import (
	"database/sql"
	"testing"

	"github.com/sroberts/sous/internal/identity"
)

// TestLookupReservedLegacy: a library from before the system person could
// hold someone else as _sous. The username is still reserved, so the direct
// route and MCP's caller header can't act as them and pass for Sous.
func TestLookupReservedLegacy(t *testing.T) {
	for _, source := range []string{"gateway", "local"} {
		t.Run(source, func(t *testing.T) {
			s := newTestService(t)
			err := s.st.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec(`INSERT INTO people VALUES ('_sous', 'Impostor', NULL, ?, 't', 't')`, source)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if p, err := s.Lookup(ctx, identity.SystemUsername); p != nil || err != nil {
				t.Errorf("Lookup(_sous) = %+v, %v, want nil", p, err)
			}
		})
	}
}
