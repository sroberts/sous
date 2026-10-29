package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/identity"
)

func TestServerGate(t *testing.T) {
	var logs bytes.Buffer
	s := &server{log: newLogger(&logs, nil)}
	h := s.handler()
	get := func(target string, hdr ...string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", target, nil)
		for i := 0; i < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/up"); rec.Code != 503 || rec.Body.Len() != 0 {
		t.Fatalf("/up before ready = %d %q, want bare 503", rec.Code, rec.Body)
	}
	rec := get("/v1/recipes")
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), `"code":"starting"`) {
		t.Fatalf("/v1 before ready = %d %q, want 503 starting", rec.Code, rec.Body)
	}

	s.setApp(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/panic":
			panic("boom")
		case "/stream":
			w.(http.Flusher).Flush()
		default:
			w.(interface{ SetCaller(identity.Caller) }).SetCaller(identity.Caller{
				Person: &identity.Person{Username: "scott"}, Route: identity.RouteDirect, Via: identity.ViaAPI,
			})
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	s.ready.Store(true)

	if rec := get("/up"); rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("/up when ready = %d %q, want bare 200", rec.Code, rec.Body)
	}
	logs.Reset()
	if rec := get("/v1/recipes?token=secret", "Cookie", "sous_person=scott", "X-Outpost-Identity", "secret"); rec.Code != http.StatusTeapot {
		t.Fatalf("/v1 when ready = %d", rec.Code)
	}
	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("request log %q: %v", logs.String(), err)
	}
	for k, want := range map[string]any{"msg": "request", "method": "GET", "path": "/v1/recipes",
		"status": float64(418), "person": "scott", "route": "direct"} {
		if line[k] != want {
			t.Errorf("log %s = %v, want %v", k, line[k], want)
		}
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("log has no duration_ms")
	}
	if strings.Contains(logs.String(), "secret") {
		t.Errorf("log leaks a secret: %s", logs.String())
	}

	if rec := get("/stream"); !rec.Flushed {
		t.Error("Flush did not reach the underlying writer")
	}

	logs.Reset()
	rec = get("/panic")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"code":"internal"`) {
		t.Fatalf("panic = %d %q, want 500 internal", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), `"status":500`) || !strings.Contains(logs.String(), `"route":"none"`) {
		t.Errorf("panic request log: %s", logs.String())
	}

	logs.Reset()
	get("/up")
	if logs.Len() != 0 {
		t.Errorf("/up was logged: %s", logs.String())
	}
}
