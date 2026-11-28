package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A redirect isn't followed, so neither the recipe nor Outpost's token
// goes anywhere but BaseURL; and a long failure is cut short.
func TestNoRedirectsAndClippedErrors(t *testing.T) {
	var elsewhere bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere = true }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/long/openai/v1/chat/completions" {
			http.Error(w, strings.Repeat("x", 10000), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := NewOutpost(srv.URL, "oai_secret")
	if _, err := c.Generate(context.Background(), "p", map[string]any{}); err == nil || elsewhere {
		t.Errorf("redirect: err %v, followed %v", err, elsewhere)
	}
	c = NewOutpost(srv.URL+"/long", "oai_secret")
	_, err := c.Generate(context.Background(), "p", map[string]any{})
	if err == nil || len(err.Error()) > maxErrorText+200 || strings.Contains(err.Error(), "oai_secret") {
		t.Errorf("long error: %d bytes", len(err.Error()))
	}
}
