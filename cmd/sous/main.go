// Command sous serves the Sous household recipe manager: web UI, REST API
// and MCP on one port, run on its own or as a service under Outpost.
package main

import "os"

// version is set at release with -ldflags "-X main.version=$TAG".
var version = "dev"

const usage = `Sous serves a household recipe library: the web UI, a REST API and MCP,
on one port of 127.0.0.1.

Usage:
  sous --port N --data DIR
  sous --version

Flags:
  --port N      port to listen on (or OUTPOST_PORT)
  --data DIR    where Sous keeps its library; created if missing
                (Outpost sets OUTPOST_STORAGE_DIR instead)

Environment:
  SOUS_LOG_LEVEL        debug, info (default), warn or error
  OUTPOST_AUTH=gateway  take who is signed in from the Outpost login gateway;
                        Outpost sets this, with OUTPOST_ISSUER and
                        OUTPOST_JWKS_URL
  SOUS_REQUIRE_GATEWAY  true refuses requests that skip the gateway

Run on its own, Sous asks each device who is cooking. Under Outpost, its
login says who you are. https://github.com/sroberts/sous
`

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout))
}
