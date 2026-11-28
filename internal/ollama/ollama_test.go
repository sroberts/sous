package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	full := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"model requires 16.9 GiB but only 11.3 GiB are available (after 512.0 MiB overhead)"}`))
	}))
	defer full.Close()
	if _, err := New(full.URL, "g", "d").Generate(context.Background(), "p", nil); !errors.Is(err, recipes.ErrModelsUnavailable) {
		t.Errorf("out of memory: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := New(bad.URL, "g", "d").Generate(context.Background(), "p", nil); err == nil || errors.Is(err, recipes.ErrModelsUnavailable) {
		t.Errorf("server error: %v", err)
	}
}

// Under Outpost: its paths, the model kinds, the token as a bearer and
// nowhere else, and a model not given or unavailable read as "wait".
func TestOutpost(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer oai_secret" {
			http.Error(w, `{"error":"no token"}`, http.StatusUnauthorized)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		seen = append(seen, r.URL.Path+" "+fmt.Sprint(body["model"]))
		switch r.URL.Path {
		case "/openai/v1/models":
			w.Write([]byte(`{"data":[{"id":"system1"}]}`))
		case "/openai/v1/chat/completions":
			if body["response_format"] == nil {
				t.Error("no response_format")
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"serve\":\"x\"}"}}]}`))
		case "/systemone/v1/systemone":
			w.Write([]byte(`{"answers":{"q":{"probabilities":{"a":1}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewOutpost(srv.URL+"/", "oai_secret")
	if llm, s1 := c.Granted(context.Background()); llm || !s1 {
		t.Errorf("granted llm %v system1 %v", llm, s1)
	}
	if out, err := c.Generate(context.Background(), "p", map[string]any{"type": "object"}); err != nil || string(out) != `{"serve":"x"}` {
		t.Errorf("generate: %s %v", out, err)
	}
	if _, err := c.Decide(context.Background(), "s", []recipes.Question{{ID: "q", Choices: []recipes.Choice{{Label: "a"}}}}); err != nil {
		t.Errorf("decide: %v", err)
	}
	want := []string{"/openai/v1/models <nil>", "/openai/v1/chat/completions llm", "/systemone/v1/systemone system1"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("requests %v, want %v", seen, want)
	}

	for _, code := range []int{http.StatusForbidden, http.StatusBadGateway, http.StatusServiceUnavailable} {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"not for you"}`, code)
		}))
		_, err := NewOutpost(bad.URL, "t").Generate(context.Background(), "p", nil)
		if !errors.Is(err, recipes.ErrModelsUnavailable) {
			t.Errorf("%d: %v", code, err)
		}
		bad.Close()
	}
}
