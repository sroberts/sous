// Command sous serves the Sous household recipe manager: web UI, REST API
// and MCP on one port, run as a service under Outpost.
package main

import "os"

// version is set at release with -ldflags "-X main.version=$TAG".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout))
}
