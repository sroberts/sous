package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sroberts/sous/internal/api"
	"github.com/sroberts/sous/internal/config"
	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/importer"
	"github.com/sroberts/sous/internal/mcp"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
	"github.com/sroberts/sous/internal/storage"
	"github.com/sroberts/sous/internal/ui"
)

// shutdownGrace leaves room inside Outpost's 10s SIGTERM-to-SIGKILL window
// for the WAL checkpoint after in-flight requests finish.
const shutdownGrace = 6 * time.Second

// run starts Sous, serves until SIGTERM or SIGINT, and returns the exit
// code. It is the only place exit codes are decided, and it never returns
// 0 after an error: under Outpost's on-failure policy that would leave Sous
// down.
func run(args []string, getenv func(string) string, stdout io.Writer) int {
	cfg, err := config.Load(args, getenv)
	switch {
	case errors.Is(err, config.ErrHelp):
		fmt.Fprint(stdout, usage)
		return 0
	case errors.Is(err, config.ErrVersion):
		fmt.Fprintln(stdout, "sous", version)
		return 0
	}
	level := new(slog.LevelVar)
	log := newLogger(stdout, level)
	if err != nil {
		log.Error("invalid configuration", "error", err.Error())
		return 1
	}
	level.Set(cfg.LogLevel)
	auth := "none"
	if cfg.Gateway() {
		auth = "gateway"
	}
	log.Info("starting", "version", version, "port", cfg.Port, "storage", cfg.StorageDir, "auth", auth)

	// Registered before listening so a signal during startup is never the
	// default kill.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sigs)

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port)))
	if err != nil {
		log.Error("listen failed", "port", cfg.Port, "error", err.Error())
		return 1
	}

	base, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	s := &server{log: log}
	srv := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return base },
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	type openResult struct {
		store *storage.Store
		err   error
	}
	opened := make(chan openResult, 1)
	go func() {
		st, err := storage.Open(base, cfg.StorageDir, log)
		opened <- openResult{st, err}
	}()

	var store *storage.Store
	for store == nil {
		select {
		case o := <-opened:
			if o.err != nil {
				log.Error("storage failed to open", "storage", cfg.StorageDir, "error", o.err.Error())
				srv.Close()
				return 1
			}
			store = o.store
		case err := <-served:
			log.Error("server failed", "error", err.Error())
			cancelBase()
			if o := <-opened; o.err == nil {
				closeStore(log, o.store)
			}
			return 1
		case sig := <-sigs:
			// Stopped before ready: wait for Open so its database is
			// closed cleanly rather than abandoned mid-migration.
			log.Info("shutting down", "signal", sig.String())
			shutdownServer(log, srv, cancelBase)
			o := <-opened
			if o.err != nil {
				if errors.Is(o.err, context.Canceled) {
					log.Info("stopped")
					return 0
				}
				log.Error("storage failed to open", "storage", cfg.StorageDir, "error", o.err.Error())
				return 1
			}
			return stop(log, o.store)
		}
	}

	svc := recipes.New(store, log)
	var jwks *identity.JWKS
	if cfg.Gateway() {
		jwks = identity.NewJWKS(cfg.JWKSURL, nil)
	}
	resolver := identity.NewResolver(identity.ResolverConfig{
		Gateway:        cfg.Gateway(),
		Issuer:         cfg.Issuer,
		RequireGateway: cfg.RequireGateway,
		JWKS:           jwks,
		Directory:      svc,
	})
	fetcher := safefetch.New(safefetch.Options{UserAgent: "Sous/" + version + " (+https://github.com/sroberts/sous)"})
	im := importer.New(svc, fetcher, log)
	app := http.NewServeMux()
	app.Handle("/v1/", api.New(api.Deps{
		Service:  svc,
		Resolver: resolver,
		JWKS:     jwks,
		Log:      log,
		Version:  version,
		Auth:     auth,
		Portal:   cfg.Issuer,
		Importer: im,
	}))
	app.Handle("/mcp", api.WithIdentity(resolver, identity.ViaMCP, mcp.New(svc, log, version, mcp.WithImporter(im))))
	app.Handle("/", ui.Handler(log))
	s.setApp(app)
	s.ready.Store(true)
	log.Info("ready", "port", cfg.Port, "storage", cfg.StorageDir, "version", version,
		"schema_version", store.SchemaVersion(), "auth", auth)

	select {
	case sig := <-sigs:
		log.Info("shutting down", "signal", sig.String())
	case err := <-served:
		log.Error("server failed", "error", err.Error())
		s.ready.Store(false)
		cancelBase()
		closeStore(log, store)
		return 1
	}
	// Further signals stay queued and unread: the shutdown below is already
	// bounded, and Outpost follows up with SIGKILL anyway.
	s.ready.Store(false)
	shutdownServer(log, srv, cancelBase)
	return stop(log, store)
}

// shutdownServer lets in-flight requests finish for up to shutdownGrace,
// then cancels every request context and drops what is left.
func shutdownServer(log *slog.Logger, srv *http.Server, cancelBase context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	err := srv.Shutdown(ctx)
	cancelBase()
	if err != nil {
		log.Warn("shutdown deadline passed; closing remaining connections", "error", err.Error())
		srv.Close()
	}
}

func stop(log *slog.Logger, store *storage.Store) int {
	if !closeStore(log, store) {
		log.Info("stopped", "exit_code", 1)
		return 1
	}
	log.Info("stopped")
	return 0
}

func closeStore(log *slog.Logger, store *storage.Store) bool {
	if err := store.Close(); err != nil {
		log.Error("closing storage failed", "error", err.Error())
		return false
	}
	return true
}
