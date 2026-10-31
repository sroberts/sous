package conformance

import (
	"encoding/json"
	"testing"
	"time"
)

// sampleCount is the number of embedded samples; internal/samples pins it
// too.
const sampleCount = 12

func listCount(t *testing.T, p *proc, path string) int {
	t.Helper()
	resp, body := p.do("GET", path, "", nil, nil)
	expectStatus(t, path, resp, body, 200)
	var page struct{ Items []json.RawMessage }
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("%s: %v: %s", path, err, body)
	}
	return len(page.Items)
}

// TestSamples: a new library starts with the samples, by no one in the
// household; they are added once, so deleting one sticks across restarts.
func TestSamples(t *testing.T) {
	skipShort(t)
	sc := newScratch(t)
	p := startReady(t, sc)
	if got := p.waitLog("samples added", 2*time.Second)["count"]; got != float64(sampleCount) {
		t.Fatalf("samples added count = %v, want %d", got, sampleCount)
	}
	if n := listCount(t, p, "/v1/recipes"); n != sampleCount {
		t.Fatalf("%d recipes, want %d", n, sampleCount)
	}
	if n := listCount(t, p, "/v1/people"); n != 0 {
		t.Fatalf("%d people, want none", n)
	}

	resp, body := p.doJSON("POST", "/v1/people", "", nil, map[string]string{"name": "Test"})
	expectStatus(t, "create person", resp, body, 201)
	resp, body = p.do("GET", "/v1/recipes?limit=1", "", nil, nil)
	var page struct{ Items []struct{ ID string } }
	json.Unmarshal(body, &page)
	if len(page.Items) != 1 {
		t.Fatalf("no recipe to delete: %s", body)
	}
	path := "/v1/recipes/" + page.Items[0].ID
	resp, body = p.do("GET", path, "", nil, nil)
	expectStatus(t, "get sample", resp, body, 200)
	resp, body = p.doJSON("DELETE", path, "test", map[string]string{"If-Match": resp.Header.Get("ETag")}, nil)
	expectStatus(t, "delete sample", resp, body, 204)
	p.stop(7 * time.Second)

	p = startReady(t, sc)
	p.waitLog("ready", 2*time.Second)
	if logged(p, "samples added") {
		t.Error("samples added again on restart")
	}
	if n := listCount(t, p, "/v1/recipes"); n != sampleCount-1 {
		t.Errorf("%d recipes after restart, want %d", n, sampleCount-1)
	}
	if n := listCount(t, p, "/v1/trash"); n != 1 {
		t.Errorf("%d in the trash, want 1", n)
	}
	p.stop(7 * time.Second)
}

func TestSamplesOff(t *testing.T) {
	skipShort(t)
	p := start(t, newScratch(t), freePort(t), "SOUS_SAMPLES=false")
	p.waitUp(10 * time.Second)
	p.waitLog("ready", 2*time.Second)
	if logged(p, "samples added") {
		t.Error("samples added with SOUS_SAMPLES=false")
	}
	if n := listCount(t, p, "/v1/recipes"); n != 0 {
		t.Errorf("%d recipes, want none", n)
	}
	p.stop(7 * time.Second)

	expectExit1(t, start(t, newScratch(t), freePort(t), "SOUS_SAMPLES=maybe"))
}
