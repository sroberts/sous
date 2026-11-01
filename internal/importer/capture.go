package importer

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
)

// Capture reads the page at rawURL and saves it in one step, for a shortcut
// with no one to review a draft (issue #33). A link already in Sous is
// answered with that copy, restored if it was in the trash, and is looked
// up before anything is fetched. A recipe is saved even when the page lacked
// steps or ingredients; Summary says so. A failed import is a
// *recipes.ImportError, as from Preview.
func (im *Importer) Capture(ctx context.Context, c identity.Caller, rawURL string) (recipes.CaptureResult, error) {
	if c.Person == nil {
		return recipes.CaptureResult{}, recipes.Errorf(recipes.CodeWhoAreYou, "choose who is cooking before importing")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return recipes.CaptureResult{}, recipes.Errorf(recipes.CodeInvalid, "send a url to read")
	}
	if links := knownAs(rawURL); len(links) > 0 {
		dups, err := im.svc.FindBySource(ctx, c, links...)
		if err != nil {
			return recipes.CaptureResult{}, err
		}
		if len(dups) > 0 {
			return im.existing(ctx, c, dups[0])
		}
	}

	p, err := im.Preview(ctx, c, recipes.ImportRequest{URL: rawURL})
	if err != nil {
		return recipes.CaptureResult{}, err
	}
	// The page's canonical link or a redirect can name a copy the link as
	// given didn't.
	if len(p.Duplicates) > 0 {
		return im.existing(ctx, c, p.Duplicates[0])
	}
	draft := p.Draft
	warnings := append([]string{}, p.Report.Warnings...)
	if strings.TrimSpace(draft.Title) == "" {
		draft.Title = "Recipe from " + hostOf(p.Report.SourceURL, rawURL)
		warnings = append(warnings, "The page had no title, so it was saved as "+draft.Title+".")
	}
	// The page read took seconds, so another capture of the same link (a
	// shortcut run twice) may have saved it since; look again and create
	// under one lock so only one copy is made.
	im.saving.Lock()
	defer im.saving.Unlock()
	links := []string{rawURL, p.Report.SourceURL}
	if p.Report.FetchedURL != nil {
		links = append(links, *p.Report.FetchedURL)
	}
	if dups, err := im.svc.FindBySource(ctx, c, links...); err != nil {
		return recipes.CaptureResult{}, err
	} else if len(dups) > 0 {
		return im.existing(ctx, c, dups[0])
	}
	rec, err := im.svc.Create(ctx, c, draft)
	if err != nil {
		return recipes.CaptureResult{}, err
	}
	return recipes.CaptureResult{
		Summary:  savedSummary(rec.Title, p.Report.Missing),
		ID:       rec.ID,
		Title:    rec.Title,
		Version:  rec.Version,
		Path:     "/r/" + rec.ID,
		Missing:  append([]string{}, p.Report.Missing...),
		Warnings: warnings,
	}, nil
}

// existing answers a capture with the copy already in Sous. FindBySource
// puts live copies first, so d is in the trash only when no live copy is;
// then it is restored, as the caller.
func (im *Importer) existing(ctx context.Context, c identity.Caller, d recipes.DuplicateRef) (recipes.CaptureResult, error) {
	out := recipes.CaptureResult{
		Summary:   "Already in Sous: " + d.Title + ".",
		ID:        d.ID,
		Title:     d.Title,
		Version:   d.Version,
		Path:      "/r/" + d.ID,
		Duplicate: true,
		Missing:   []string{},
		Warnings:  []string{},
	}
	if !d.Deleted {
		return out, nil
	}
	rec, err := im.svc.Restore(ctx, c, d.ID)
	if err != nil {
		return recipes.CaptureResult{}, err
	}
	out.Title, out.Version, out.Restored = rec.Title, rec.Version, true
	out.Summary = "Restored " + rec.Title + " from the trash."
	return out, nil
}

// knownAs is every form of rawURL a saved recipe's source could match
// before the page is read: the link itself and, for an archive copy, the
// original page. Anything that isn't an http or https link matches nothing.
func knownAs(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	links := []string{rawURL}
	if _, orig, ok := WaybackParts(rawURL); ok {
		links = append(links, orig)
	}
	return links
}

// savedSummary says the recipe was saved and, of what the page lacked, only
// what matters for cooking it: ingredients and steps.
func savedSummary(title string, missing []string) string {
	var gaps []string
	for _, m := range []string{"ingredients", "steps"} {
		if slices.Contains(missing, m) {
			gaps = append(gaps, m)
		}
	}
	if len(gaps) == 0 {
		return "Saved " + title + "."
	}
	return "Saved " + title + ", but no " + strings.Join(gaps, " or ") + " were found."
}

// hostOf is the first link's host, for naming a recipe
// whose page had no title.
func hostOf(links ...string) string {
	for _, l := range links {
		if h := recipes.SourceHost(l); h != nil {
			return *h
		}
	}
	return "the web"
}
