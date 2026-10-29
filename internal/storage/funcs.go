package storage

import (
	"database/sql/driver"
	"strings"

	"modernc.org/sqlite"
)

// fold(x) lower-cases with Go's Unicode rules. SQLite's lower() and NOCASE
// fold ASCII only, so "Émincé" would never match a search for "émincé".
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("fold", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		switch v := args[0].(type) {
		case string:
			return strings.ToLower(v), nil
		case []byte:
			return strings.ToLower(string(v)), nil
		default:
			return v, nil
		}
	})
}
