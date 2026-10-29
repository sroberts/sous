package main

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sroberts/sous/internal/identity"
)

func newLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// recorder captures the status for the request log and the caller for its
// person and route. The API reports the caller by calling SetCaller on the
// ResponseWriter it was handed, before wrapping it.
type recorder struct {
	http.ResponseWriter
	status int
	caller identity.Caller
}

// record returns w as a recorder, wrapping it unless an outer middleware
// already did.
func record(w http.ResponseWriter) *recorder {
	if rec, ok := w.(*recorder); ok {
		return rec
	}
	return &recorder{ResponseWriter: w}
}

func (r *recorder) SetCaller(c identity.Caller) { r.caller = c }

func (r *recorder) wrote() bool { return r.status != 0 }

func (r *recorder) WriteHeader(code int) {
	// 1xx responses are informational; the final status comes later.
	if r.status == 0 && code >= 200 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Flush keeps streaming working for MCP; Unwrap lets http.ResponseController
// reach the rest of the underlying writer.
func (r *recorder) Flush() {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logRequests logs one line per request. It never logs headers, cookies,
// query strings or bodies, which is where tokens would be.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := record(w)
		start := time.Now()
		finished := false
		defer func() {
			status := rec.status
			if !finished {
				status = http.StatusInternalServerError
			} else if status == 0 {
				status = http.StatusOK
			}
			route := rec.caller.Route
			if route == "" {
				route = identity.RouteNone
			}
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", float64(time.Since(start).Microseconds())/1000,
				"person", rec.caller.Username(),
				"route", string(route),
			)
		}()
		next.ServeHTTP(rec, r)
		finished = true
	})
}
