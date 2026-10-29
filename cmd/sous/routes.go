package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"
)

// server is the top-level handler. It serves before storage is open, so the
// real routes arrive later through app; ready gates them.
type server struct {
	ready atomic.Bool
	app   atomic.Pointer[http.Handler]
	log   *slog.Logger
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	// Bare on purpose: one atomic load, no middleware, never logged.
	mux.HandleFunc("GET /up", func(w http.ResponseWriter, _ *http.Request) {
		if s.ready.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	mux.Handle("/", recoverPanics(s.log, logRequests(s.log, http.HandlerFunc(s.gate))))
	return mux
}

func (s *server) gate(w http.ResponseWriter, r *http.Request) {
	app := s.app.Load()
	if !s.ready.Load() || app == nil {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "starting", "Sous is not ready")
		return
	}
	(*app).ServeHTTP(w, r)
}

func (s *server) setApp(h http.Handler) { s.app.Store(&h) }

func recoverPanics(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := record(w)
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p)
			}
			log.Error("panic", "method", r.Method, "path", r.URL.Path,
				"panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			if !rec.wrote() {
				writeError(rec, http.StatusInternalServerError, "internal", "internal error")
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%q,"message":%q}}`+"\n", code, message)
}
