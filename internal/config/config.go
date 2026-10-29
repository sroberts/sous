// Package config reads Sous's configuration from flags and the environment
// (spec.md §2). Nothing is read from a file and nothing prompts.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
	var c Config

	fs := flag.NewFlagSet("sous", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	portFlag := fs.String("port", "", "port to listen on (overrides OUTPOST_PORT)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, errors.New("usage: sous [--port N]")
		}
		return c, fmt.Errorf("flags: %w", err)
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("flags: unexpected argument %q (usage: sous [--port N])", fs.Arg(0))
	}

	portSrc, portVal := "--port", *portFlag
	if portVal == "" {
		portSrc, portVal = "OUTPOST_PORT", getenv("OUTPOST_PORT")
	}
	if portVal == "" {
		return c, errors.New("port is required: set --port or OUTPOST_PORT")
	}
	port, err := strconv.Atoi(portVal)
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("%s: %q is not a port between 1 and 65535", portSrc, portVal)
	}
	c.Port = port

	dir := getenv("OUTPOST_STORAGE_DIR")
	if dir == "" {
		return c, errors.New("OUTPOST_STORAGE_DIR is required")
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return c, fmt.Errorf("OUTPOST_STORAGE_DIR: %w", err)
	}
	c.StorageDir = dir

	switch c.Auth = getenv("OUTPOST_AUTH"); c.Auth {
	case "":
	case "gateway":
		c.Issuer = getenv("OUTPOST_ISSUER")
		if c.Issuer == "" {
			return c, errors.New("OUTPOST_ISSUER is required when OUTPOST_AUTH=gateway")
		}
		c.JWKSURL = getenv("OUTPOST_JWKS_URL")
		if c.JWKSURL == "" {
			return c, errors.New("OUTPOST_JWKS_URL is required when OUTPOST_AUTH=gateway")
		}
		u, err := url.Parse(c.JWKSURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, fmt.Errorf("OUTPOST_JWKS_URL: %q is not an http(s) URL", c.JWKSURL)
		}
	default:
		return c, fmt.Errorf("OUTPOST_AUTH: %q is not supported (leave it unset or use \"gateway\")", c.Auth)
	}

	switch v := getenv("SOUS_REQUIRE_GATEWAY"); v {
	case "", "false":
	case "true":
		c.RequireGateway = true
	default:
		return c, fmt.Errorf("SOUS_REQUIRE_GATEWAY: %q is not \"true\" or \"false\"", v)
	}
	if c.RequireGateway && !c.Gateway() {
		return c, errors.New("SOUS_REQUIRE_GATEWAY=true needs OUTPOST_AUTH=gateway")
	}

	switch v := getenv("SOUS_LOG_LEVEL"); strings.ToLower(v) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "", "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		return c, fmt.Errorf("SOUS_LOG_LEVEL: %q is not debug, info, warn or error", v)
	}

	// Created last so a configuration error leaves nothing behind.
	if err := os.MkdirAll(c.StorageDir, 0o700); err != nil {
		return c, fmt.Errorf("OUTPOST_STORAGE_DIR: %w", err)
	}
	return c, nil
}
