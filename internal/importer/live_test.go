//go:build live

package importer

// The live check reads the links in testdata/testset.json from the real
// sites and prints how each went:
//
//	go test -tags live -run Live -v ./internal/importer
//
// With -record it also writes each page's extracted data to
// testdata/pages/NN-slug.json, and what Extract makes of it to
// NN-slug.want.json, for TestFixtures to replay offline.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
	"github.com/sroberts/sous/internal/storage"
)

var record = flag.Bool("record", false, "write testdata/pages fixtures from the live pages")

type testLink struct {
	N         int    `json:"n"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Publisher string `json:"publisher"`
	Expect    string `json:"expect"`
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugRE.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// sourceHas reports whether the recipe data on the page mentions any of
// keys, so a missing field counts against the import only when the site
// actually gave it.
func sourceHas(p Page, keys ...string) bool {
	var all strings.Builder
	for _, b := range p.JSONLD {
		all.Write(b)
	}
	all.WriteString(p.Microdata)
	all.WriteString(p.HRecipe)
	s := all.String()
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func TestLive(t *testing.T) {
	b, err := os.ReadFile("testdata/testset.json")
	if err != nil {
		t.Fatal(err)
	}
	var links []testLink
	if err := json.Unmarshal(b, &links); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := recipes.New(st, slog.New(slog.DiscardHandler))
	p, err := svc.CreatePerson(ctx, identity.Caller{Via: identity.ViaAPI}, "Live")
	if err != nil {
		t.Fatal(err)
	}
	who := identity.Caller{Person: &identity.Person{Username: p.Username, Name: p.Name}, Via: identity.ViaAPI}
	im := New(svc, safefetch.New(safefetch.Options{UserAgent: "Sous/live (+https://github.com/sroberts/sous)"}), nil)
	if *record {
		if err := os.MkdirAll(pagesDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(tw, "#\t| host\t| outcome\t| method\t| ingr\t| steps\t| sections\t| prep/cook/total\t| yield\t| image\t| error")
	counts := map[string]int{}
	for _, l := range links {
		row := liveRow(ctx, t, im, who, l)
		counts[row[1]]++
		u, _ := url.Parse(l.URL)
		host := ""
		if u != nil {
			host = strings.TrimPrefix(u.Hostname(), "www.")
		}
		fmt.Fprintf(tw, "%d\t| %s\t| %s\n", l.N, host, strings.Join(row[1:], "\t| "))
	}
	tw.Flush()
	fmt.Printf("\npass %d · partial %d · fail %d\n", counts["pass"], counts["partial"], counts["fail"])
}

// liveRow imports one link and returns the table cells after the host:
// outcome, method, ingr, steps, sections, times, yield, image, error. The
// first cell is unused.
func liveRow(ctx context.Context, t *testing.T, im *Importer, who identity.Caller, l testLink) []string {
	bctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	fail := func(err error) []string {
		code := err.Error()
		var ie *recipes.ImportError
		if errors.As(err, &ie) {
			code = ie.Code
			if ie.Page == nil {
				code += " (no fallback)"
			}
		}
		return []string{"", "fail", "", "", "", "", "", "", "", code}
	}
	f, err := im.fetchPage(bctx, l.URL)
	if err != nil {
		return fail(err)
	}
	if *record {
		base := filepath.Join(pagesDir, fmt.Sprintf("%02d-%s", l.N, slug(l.Title)))
		if err := os.WriteFile(base+".json", marshalFixture(f.page), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(base+".want.json", marshalFixture(extractFixture(f.page)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := im.fromPage(bctx, ctx, who, f)
	if err != nil {
		return fail(err)
	}
	d := p.Draft
	sections := 0
	for _, s := range append(append([]string{}, d.Ingredients...), d.Steps...) {
		if strings.HasPrefix(s, "#") {
			sections++
		}
	}
	mins := func(v *int) string {
		if v == nil {
			return "-"
		}
		return fmt.Sprint(*v)
	}
	times := mins(d.PrepMinutes) + "/" + mins(d.CookMinutes) + "/" + mins(d.TotalMinutes)
	if d.TimeText != nil && d.PrepMinutes == nil && d.CookMinutes == nil && d.TotalMinutes == nil {
		times = *d.TimeText
	}
	yield := "-"
	if d.YieldText != nil {
		yield = *d.YieldText
	}
	image, problem := "no", ""
	if len(d.Images) > 0 {
		image = "yes"
	} else {
		for _, c := range p.Images {
			if c.Problem != nil {
				problem = *c.Problem
				break
			}
		}
	}
	hasTimes := d.PrepMinutes != nil || d.CookMinutes != nil || d.TotalMinutes != nil || d.TimeText != nil
	outcome := "pass"
	switch {
	case d.Title == "", len(d.Ingredients) == 0, len(d.Steps) == 0, len(d.Images) == 0:
		outcome = "partial"
	case !hasTimes && sourceHas(f.page, "prepTime", "cookTime", "totalTime", "dt-duration"):
		outcome = "partial"
	case d.YieldText == nil && sourceHas(f.page, "recipeYield", "p-yield"):
		outcome = "partial"
	}
	return []string{"", outcome, p.Report.Method, fmt.Sprint(len(d.Ingredients)), fmt.Sprint(len(d.Steps)),
		fmt.Sprint(sections), times, yield, image, problem}
}
