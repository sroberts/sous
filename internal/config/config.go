// Package config reads Sous's configuration from flags and the environment
// (spec.md §2). Nothing is read from a file and nothing prompts.
package config

import "log/slog"

// Config is the parsed, validated configuration.
type Config struct {
	Port           int        // --port, else OUTPOST_PORT; 1..65535
	StorageDir     string     // OUTPOST_STORAGE_DIR, made absolute
	Auth           string     // OUTPOST_AUTH: "" or "gateway"
	Issuer         string     // OUTPOST_ISSUER; required in gateway mode
	JWKSURL        string     // OUTPOST_JWKS_URL; required in gateway mode
	RequireGateway bool       // SOUS_REQUIRE_GATEWAY
	LogLevel       slog.Level // SOUS_LOG_LEVEL: debug, info (default), warn, error
}

// Gateway reports whether gateway token verification is on.
func (c Config) Gateway() bool { return c.Auth == "gateway" }

// Load parses args (without the program name) and reads getenv.
//
// --port wins over OUTPOST_PORT, and one of them is required. Positional
// arguments are an error. OUTPOST_STORAGE_DIR is required; it is made
// absolute and created with MkdirAll(0700). OUTPOST_AUTH other than unset or
// "gateway" is an error; gateway mode requires OUTPOST_ISSUER and
// OUTPOST_JWKS_URL. SOUS_REQUIRE_GATEWAY=true without gateway mode is an
// error. Every error message says which setting is wrong.
func Load(args []string, getenv func(string) string) (Config, error) {
	panic("TODO")
}
