// Command sous serves the Sous household recipe manager: web UI, REST API
// and MCP on one port, run as a service under Outpost.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sroberts/sous/internal/config"
)

// version is set at release with -ldflags "-X main.version=$TAG".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout)
	stop()
	os.Exit(code)
}

// run serves until ctx is cancelled and returns the exit code. It is the
// only place exit codes are decided.
func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) int {
	cfg, err := config.Load(args, getenv)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	_ = cfg
	<-ctx.Done()
	return 0
}
