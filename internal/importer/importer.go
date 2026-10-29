// Package importer reads a recipe from a web page, or from pasted text,
// into a draft for a person to review (spec.md §14).
package importer

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

// MaxImageBytes is the largest photo an import downloads, the same as the
// upload limit.
const MaxImageBytes = safefetch.MaxImageBytes

const (
	// maxConcurrent imports run at once; the rest wait their turn.
	maxConcurrent = 4
	// budget bounds a whole preview: the page, its photos and storing them.
	budget = 25 * time.Second
)

// Fetcher is the safefetch client, as the importer uses it.
type Fetcher interface {
	Get(ctx context.Context, rawURL string, kind safefetch.Kind, referer string) (*safefetch.Response, error)
}

// Importer turns links and pasted text into recipe drafts.
type Importer struct {
	svc   *recipes.Service
	fetch Fetcher
	log   *slog.Logger
	sem   chan struct{}
}

func New(svc *recipes.Service, fetcher Fetcher, log *slog.Logger) *Importer {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Importer{svc: svc, fetch: fetcher, log: log, sem: make(chan struct{}, maxConcurrent)}
}

// MessageNoHost is the not_found message when the site's name doesn't
// resolve.
const MessageNoHost = "Sous can't find that site. Check the link."

const messageNoText = "Sous couldn't find ingredients or steps in that text."

// Preview reads req into a draft without saving it. Photos are downloaded
// and stored, so the caller must be a person. With Text set nothing is
// fetched; URL, if also given, is only the link the recipe is credited to.
// A failed import is a *recipes.ImportError.
func (im *Importer) Preview(ctx context.Context, c identity.Caller, req recipes.ImportRequest) (recipes.ImportPreview, error) {
	if c.Person == nil {
		return recipes.ImportPreview{}, recipes.Errorf(recipes.CodeWhoAreYou, "choose who is cooking before importing")
	}
	req.URL, req.Text, req.Title = strings.TrimSpace(req.URL), strings.TrimSpace(req.Text), strings.TrimSpace(req.Title)
	start := time.Now()
	var (
		p   recipes.ImportPreview
		err error
	)
	if req.Text != "" {
		p, err = im.fromText(ctx, c, req)
	} else if req.URL != "" {
		p, err = im.fromURL(ctx, c, req.URL)
	} else {
		return recipes.ImportPreview{}, recipes.Errorf(recipes.CodeInvalid, "send a url to read, or some text")
	}
	im.logOutcome(req, p, err, time.Since(start))
	return p, err
}

// Import previews the page at rawURL and, when save is set, creates the
// recipe. Saving a link already in Sous is CodeExists rather than a copy.
func (im *Importer) Import(ctx context.Context, c identity.Caller, rawURL string, save bool) (recipes.ImportResult, error) {
	p, err := im.Preview(ctx, c, recipes.ImportRequest{URL: rawURL})
	if err != nil {
		return recipes.ImportResult{}, err
	}
	out := recipes.ImportResult{ImportPreview: p}
	if !save {
		return out, nil
	}
	if len(p.Duplicates) > 0 {
		d := p.Duplicates[0]
		if d.Deleted {
			return out, recipes.Errorf(recipes.CodeExists, "%q (id %s) was imported from this link and is in the trash; restore it instead", d.Title, d.ID)
		}
		return out, recipes.Errorf(recipes.CodeExists, "%q (id %s) was already imported from this link", d.Title, d.ID)
	}
	rec, err := im.svc.Create(ctx, c, p.Draft)
	if err != nil {
		return out, err
	}
	out.Saved = &rec
	return out, nil
}

func (im *Importer) fromText(ctx context.Context, c identity.Caller, req recipes.ImportRequest) (recipes.ImportPreview, error) {
	source := ""
	if req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			source = CleanURL(req.URL)
		}
	}
	title, ingredients, steps := SplitText(req.Text)
	if t := NormalizeTitle(req.Title); t != "" {
		title = t
	}
	if len(ingredients) == 0 && len(steps) == 0 {
		var page *recipes.ImportPage
		if source != "" {
			page = &recipes.ImportPage{SourceURL: source}
			if title != "" {
				page.Title = &title
			}
		}
		return recipes.ImportPreview{}, &recipes.ImportError{Code: recipes.CodeNoRecipe, Message: messageNoText, Page: page}
	}
	draft := recipes.RecipeInput{Title: title, Ingredients: ingredients, Steps: steps}
	if source != "" {
		draft.SourceURL = &source
	}
	rep := recipes.ImportReport{Method: MethodText, SourceURL: source, Found: []string{}, Missing: []string{}, Warnings: []string{}}
	for _, f := range []struct {
		name string
		ok   bool
	}{{"title", title != ""}, {"ingredients", len(ingredients) > 0}, {"steps", len(steps) > 0}} {
		if f.ok {
			rep.Found = append(rep.Found, f.name)
		} else {
			rep.Missing = append(rep.Missing, f.name)
		}
	}
	rep.Missing = append(rep.Missing, "times", "yield", "image")
	dups := []recipes.DuplicateRef{}
	if source != "" {
		var err error
		if dups, err = im.svc.FindBySource(ctx, c, req.URL, source); err != nil {
			return recipes.ImportPreview{}, err
		}
	}
	return recipes.ImportPreview{
		Draft:       draft,
		Ingredients: previewLines(draft.Ingredients),
		Images:      []recipes.ImageCandidate{},
		Report:      rep,
		Duplicates:  dups,
	}, nil
}

func (im *Importer) fromURL(ctx context.Context, c identity.Caller, rawURL string) (recipes.ImportPreview, error) {
	bctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	select {
	case im.sem <- struct{}{}:
		defer func() { <-im.sem }()
	case <-bctx.Done():
		return recipes.ImportPreview{}, importErr(recipes.CodeTimeout, "", &recipes.ImportPage{SourceURL: CleanURL(rawURL)})
	}
	f, err := im.fetchPage(bctx, rawURL)
	if err != nil {
		return recipes.ImportPreview{}, err
	}
	return im.fromPage(bctx, ctx, c, f)
}

// fetched is a page as read, before extraction.
type fetched struct {
	input    string // the link as given
	final    string // where the page was read from, after redirects
	page     Page
	resp     *safefetch.Response
	archived bool
}

// fetchPage reads the page at rawURL (for a web.archive.org link, the
// archived copy without the toolbar) and scans it. Failures that leave no
// page, and non-HTML pages, are import errors.
func (im *Importer) fetchPage(ctx context.Context, rawURL string) (fetched, error) {
	fallback := &recipes.ImportPage{SourceURL: CleanURL(rawURL)}
	fetchURL := rawURL
	ts, orig, archived := WaybackParts(rawURL)
	if archived {
		fetchURL = WaybackFetchURL(ts, orig)
		fallback.SourceURL = CleanURL(orig)
	}
	resp, err := im.fetch.Get(ctx, fetchURL, safefetch.KindPage, "")
	if err != nil {
		return fetched{}, fetchError(err, fallback)
	}
	ct := resp.Header.Get("Content-Type")
	if resp.Status >= 200 && resp.Status <= 299 && !isHTML(ct) {
		return fetched{}, importErr(recipes.CodeUnsupportedURL, "That link isn't a web page.", fallback)
	}
	// The id_ copy is the original markup, so its relative links belong to
	// the original site, not to web.archive.org.
	base := resp.URL
	if archived {
		if u, err := url.Parse(orig); err == nil {
			base = u
		}
	}
	page := Scan(resp.Body, ct, base)
	page.FetchedURL = resp.URL.String()
	page.SourceURL = rawURL
	page.Status = resp.Status
	page.Server = resp.Header.Get("Server")
	page.CFMitigated = resp.Header.Get("Cf-Mitigated")
	page.Size = len(resp.Body)
	return fetched{input: rawURL, final: resp.URL.String(), page: page, resp: resp, archived: archived}, nil
}

// fromPage extracts the recipe from a fetched page, stores its photo and
// looks for copies already in Sous. fetchCtx carries the import's budget.
func (im *Importer) fromPage(fetchCtx, ctx context.Context, c identity.Caller, f fetched) (recipes.ImportPreview, error) {
	var (
		draft recipes.RecipeInput
		cands []Candidate
		rep   Report
		err   error
	)
	extracted := false
	if f.resp.Status >= 200 && f.resp.Status <= 299 {
		draft, cands, rep, err = Extract(f.page)
		extracted = err == nil
	}
	if code := Classify(f.resp.Status, f.resp.Header, f.page, extracted); code != "" {
		return recipes.ImportPreview{}, importErr(code, "", FallbackPage(f.page))
	}

	source := ""
	if draft.SourceURL != nil {
		source = *draft.SourceURL
	}
	out := recipes.ImportPreview{
		Report: recipes.ImportReport{
			Method:    rep.Method,
			SourceURL: source,
			Archived:  f.archived,
			Found:     rep.Found,
			Missing:   rep.Missing,
			Warnings:  rep.Warnings,
		},
	}
	if f.final != f.input {
		out.Report.FetchedURL = &f.final
	}
	out.Images = im.storeImages(fetchCtx, ctx, c, cands, f.final, &draft)
	out.Draft = draft
	out.Ingredients = previewLines(draft.Ingredients)
	out.Duplicates, err = im.svc.FindBySource(ctx, c, f.input, f.final, f.page.Canonical, source)
	if err != nil {
		return recipes.ImportPreview{}, err
	}
	return out, nil
}

func previewLines(lines []string) []recipes.ParsedLine {
	if out := recipes.PreviewLines(lines); out != nil {
		return out
	}
	return []recipes.ParsedLine{}
}

func importErr(code, message string, page *recipes.ImportPage) *recipes.ImportError {
	if message == "" {
		message = Message(code)
	}
	return &recipes.ImportError{Code: code, Message: message, Page: page}
}

// fetchError maps a safefetch failure, which leaves no page to scan, to an
// import error.
func fetchError(err error, page *recipes.ImportPage) error {
	var blocked *safefetch.BlockedError
	var dns *net.DNSError
	switch {
	case errors.As(err, &blocked):
		return importErr(recipes.CodeUnsupportedURL, MessageLocalAddress, page)
	case errors.Is(err, safefetch.ErrUnsupportedURL):
		return importErr(recipes.CodeUnsupportedURL, "", page)
	case errors.Is(err, safefetch.ErrTimeout), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return importErr(recipes.CodeTimeout, "", page)
	case errors.Is(err, safefetch.ErrTooLarge):
		return importErr(recipes.CodePageTooLarge, "", page)
	case errors.As(err, &dns) && dns.IsNotFound:
		return importErr(recipes.CodePageNotFound, MessageNoHost, page)
	}
	return importErr(recipes.CodeSiteError, "", page)
}

// logOutcome writes the one log line an import gets. It names the host and
// path but never the query string, which can carry tokens.
func (im *Importer) logOutcome(req recipes.ImportRequest, p recipes.ImportPreview, err error, d time.Duration) {
	host, path := "", ""
	if u, perr := url.Parse(req.URL); perr == nil {
		host, path = u.Hostname(), u.EscapedPath()
	}
	method := p.Report.Method
	outcome := "ok"
	var ie *recipes.ImportError
	var re *recipes.Error
	switch {
	case err == nil:
	case errors.As(err, &ie):
		outcome = ie.Code
	case errors.As(err, &re):
		outcome = re.Code
	default:
		outcome = "error"
	}
	if req.Text != "" {
		method = MethodText
	}
	attrs := []any{"host", host, "path", path, "outcome", outcome, "method", method, "duration_ms", d.Milliseconds()}
	if outcome == "error" {
		im.log.Warn("import", append(attrs, "error", err.Error())...)
		return
	}
	im.log.Info("import", attrs...)
}
