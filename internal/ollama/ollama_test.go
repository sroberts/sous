package ollama

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sroberts/sous/internal/recipes"
)

func TestDecide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"model":"laya","answers":{"q":{"type":"choice","choice":"a","probabilities":{"a":0.7,"b":0.3},"confidence":0.2}}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "g", "laya")
	got, err := c.Decide(context.Background(), "state", []recipes.Question{{ID: "q", Ask: "?", Choices: []recipes.Choice{{Label: "a", Means: "A"}, {Label: "b", Means: "B"}}}})
	if err != nil || got["q"].Probabilities["a"] != 0.7 {
		t.Fatalf("decide = %v, %v", got, err)
	}
	if _, err := c.Decide(context.Background(), "s", []recipes.Question{{ID: "missing"}}); err == nil {
		t.Error("an unanswered question should fail")
	}
}

// No connection is unreachable (try later); a slow model is the recipe's
// failure; a missing model is unreachable, as installing it fixes it.
func TestFailures(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	if _, err := New("http://"+addr, "g", "d").Generate(context.Background(), "p", nil); !errors.Is(err, recipes.ErrModelsUnavailable) {
		t.Errorf("refused: %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer slow.Close()
	c := New(slow.URL, "g", "d")
	c.HTTP.Timeout = 20 * time.Millisecond
	if _, err := c.Generate(context.Background(), "p", nil); err == nil || errors.Is(err, recipes.ErrModelsUnavailable) || !strings.Contains(err.Error(), "didn't answer") {
		t.Errorf("slow: %v", err)
	}

	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model \"g\" not found, try pulling it first"}`))
	}))
	defer missing.Close()
	if _, err := New(missing.URL, "g", "d").Generate(context.Background(), "p", nil); !errors.Is(err, recipes.ErrModelsUnavailable) {
		t.Errorf("missing model: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := New(bad.URL, "g", "d").Generate(context.Background(), "p", nil); err == nil || errors.Is(err, recipes.ErrModelsUnavailable) {
		t.Errorf("server error: %v", err)
	}
}
