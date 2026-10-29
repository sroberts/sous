package importer

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sroberts/sous/internal/recipes"
)

var updateFixtures = flag.Bool("update", false, "rewrite testdata/pages/*.want.json from the current extraction")

const pagesDir = "testdata/pages"

// maxFixtureBytes keeps fixtures to extracted data, never a whole page.
const maxFixtureBytes = 200 << 10

// fixtureResult is what Extract and Classify make of a recorded page.
type fixtureResult struct {
	Code   string               `json:"code,omitempty"`
	Draft  *recipes.RecipeInput `json:"draft,omitempty"`
	Images []Candidate          `json:"images,omitempty"`
	Report *Report              `json:"report,omitempty"`
	Page   *recipes.ImportPage  `json:"page,omitempty"`
}

func extractFixture(p Page) fixtureResult {
	ok := p.Status == 0 || (p.Status >= 200 && p.Status <= 299)
	h := http.Header{}
	if p.CFMitigated != "" {
		h.Set("Cf-Mitigated", p.CFMitigated)
	}
	var (
		in    recipes.RecipeInput
		cands []Candidate
		rep   Report
		err   error
	)
	if ok {
		in, cands, rep, err = Extract(p)
	}
	if code := Classify(p.Status, h, p, ok && err == nil); code != "" {
		return fixtureResult{Code: code, Page: FallbackPage(p)}
	}
	return fixtureResult{Draft: &in, Images: cands, Report: &rep}
}

func marshalFixture(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

// pageFixtures lists the recorded pages, skipping the test when there are
// none yet.
func pageFixtures(t *testing.T) []string {
	t.Helper()
	all, _ := filepath.Glob(filepath.Join(pagesDir, "*.json"))
	var pages []string
	for _, f := range all {
		if !strings.HasSuffix(f, ".want.json") {
			pages = append(pages, f)
		}
	}
	if len(pages) == 0 {
		t.Skip("no recorded pages in " + pagesDir + "; record them with go test -tags live -run Live -record ./internal/importer")
	}
	return pages
}

func TestFixtures(t *testing.T) {
	for _, f := range pageFixtures(t) {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var p Page
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatal(err)
			}
			got := marshalFixture(extractFixture(p))
			wantFile := strings.TrimSuffix(f, ".json") + ".want.json"
			if *updateFixtures {
				if err := os.WriteFile(wantFile, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(wantFile)
			if err != nil {
				t.Fatalf("%v (rerun with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("extraction changed; if intended, rerun with -update\ngot:\n%s", got)
			}
		})
	}
}

var wholePageRE = regexp.MustCompile(`(?i)<(?:html|head|body)[\s>]`)

func TestFixturesHoldNoPages(t *testing.T) {
	pageFixtures(t)
	all, _ := filepath.Glob(filepath.Join(pagesDir, "*.json"))
	for _, f := range all {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) >= maxFixtureBytes {
			t.Errorf("%s is %d bytes; fixtures hold extracted data only (limit %d)", f, len(b), maxFixtureBytes)
		}
		// Check the decoded strings too: JSON escapes < as <.
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		decoded, _ := json.Marshal(v)
		decoded = bytes.ReplaceAll(decoded, []byte(`<`), []byte("<"))
		if m := wholePageRE.Find(append(b, decoded...)); m != nil {
			t.Errorf("%s contains %q; fixtures must not hold a whole page", f, m)
		}
	}
}
