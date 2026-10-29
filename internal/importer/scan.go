package importer

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Page is what one pass over a fetched page keeps: enough to extract the
// recipe, classify a failure and offer a fallback, and small enough to
// commit as a test fixture. It never holds the whole document.
type Page struct {
	// SourceURL is the link as given; FetchedURL is where the page was
	// read from. Scan sets both to base; the caller corrects SourceURL.
	SourceURL   string `json:"source_url"`
	FetchedURL  string `json:"fetched_url"`
	Status      int    `json:"status,omitempty"`
	Server      string `json:"server,omitempty"`
	CFMitigated string `json:"cf_mitigated,omitempty"`
	// Size is the length of the fetched body. The caller sets it; 0 means
	// unknown and counts as small.
	Size int `json:"size,omitempty"`

	Title         string `json:"title,omitempty"`
	OGTitle       string `json:"og_title,omitempty"`
	OGSiteName    string `json:"og_site_name,omitempty"`
	OGImage       string `json:"og_image,omitempty"`
	OGImageWidth  int    `json:"og_image_width,omitempty"`
	OGImageHeight int    `json:"og_image_height,omitempty"`
	TwitterImage  string `json:"twitter_image,omitempty"`
	Canonical     string `json:"canonical,omitempty"`

	// Markers are bot-wall signs found in the page, plus MarkerNotFree.
	Markers []string `json:"markers,omitempty"`

	// JSONLD is the ld+json blocks that hold a Recipe, with unrelated
	// @graph members stubbed. Microdata and HRecipe are the outer HTML of
	// the first Recipe itemscope and .h-recipe, without scripts or styles.
	JSONLD    []json.RawMessage `json:"jsonld,omitempty"`
	Microdata string            `json:"microdata,omitempty"`
	HRecipe   string            `json:"h_recipe,omitempty"`
}

// MarkerNotFree is the marker for a page whose ld+json says
// isAccessibleForFree is false.
const MarkerNotFree = "not-free"

// botMarkers are matched case-insensitively against the page. "challenge"
// is only checked on small pages with no recipe.
var botMarkers = []string{"just a moment", "cf-chl", "captcha", "px-captcha", "datadome", "enable javascript"}

// strongMarkers name a bot wall outright. The others ("captcha", "enable
// javascript", "challenge") also turn up in ordinary recipe pages: a
// comment form's captcha, a noscript notice.
var strongMarkers = map[string]bool{"just a moment": true, "cf-chl": true, "px-captcha": true, "datadome": true}

const challengeMaxBytes = 50 << 10

func (p Page) has(marker string) bool {
	for _, m := range p.Markers {
		if m == marker {
			return true
		}
	}
	return false
}

// botWall reports whether the markers say the page is a bot challenge.
// Generic markers count only when no recipe was read and the page is small
// or wasn't a plain 200.
func (p Page) botWall(status int, extracted bool) bool {
	generic := !extracted && (p.Size < challengeMaxBytes || (status != 0 && status != 200))
	for _, m := range p.Markers {
		switch {
		case m == MarkerNotFree:
		case strongMarkers[m]:
			return true
		case generic:
			return true
		}
	}
	return false
}

// Scan reads a fetched page in one parse.
func Scan(body []byte, contentType string, base *url.URL) Page {
	var p Page
	if base != nil {
		p.SourceURL = base.String()
		p.FetchedURL = base.String()
	}
	body = decodeCharset(body, contentType)
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return p
	}
	resolve := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" || base == nil {
			return s
		}
		r, err := base.Parse(s)
		if err != nil {
			return s
		}
		return r.String()
	}
	var (
		ldTexts         []string
		micro, hrec     *html.Node
		ogImage, ogURL  string
		ogW, ogH        string
		inSVG           int
		titleSet, canon bool
	)
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Svg:
				inSVG++
				defer func() { inSVG-- }()
			case atom.Title:
				if !titleSet && inSVG == 0 {
					p.Title = cleanText(text(n))
					titleSet = true
				}
			case atom.Meta:
				key := strings.ToLower(attr(n, "property"))
				if key == "" {
					key = strings.ToLower(attr(n, "name"))
				}
				content := strings.TrimSpace(attr(n, "content"))
				set := func(dst *string) {
					if *dst == "" {
						*dst = content
					}
				}
				switch key {
				case "og:title":
					set(&p.OGTitle)
				case "og:site_name":
					set(&p.OGSiteName)
				case "og:image":
					set(&ogImage)
				case "og:image:url", "og:image:secure_url":
					set(&ogURL)
				case "og:image:width":
					set(&ogW)
				case "og:image:height":
					set(&ogH)
				case "twitter:image", "twitter:image:src":
					set(&p.TwitterImage)
				}
			case atom.Link:
				if !canon && hasToken(attr(n, "rel"), "canonical") && attr(n, "href") != "" {
					p.Canonical = resolve(attr(n, "href"))
					canon = true
				}
			case atom.Script:
				if t, _, _ := mime.ParseMediaType(attr(n, "type")); t == "application/ld+json" {
					ldTexts = append(ldTexts, text(n))
				}
			}
			if micro == nil && hasAttr(n, "itemscope") && isRecipeItemtype(attr(n, "itemtype")) {
				micro = n
			}
			if hrec == nil && (hasToken(attr(n, "class"), "h-recipe") || hasToken(attr(n, "class"), "hrecipe")) {
				hrec = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if ogImage == "" {
		ogImage = ogURL
	}
	p.OGImage = resolve(ogImage)
	p.OGImageWidth, _ = strconv.Atoi(ogW)
	p.OGImageHeight, _ = strconv.Atoi(ogH)
	p.TwitterImage = resolve(p.TwitterImage)
	p.OGTitle = cleanText(p.OGTitle)
	p.OGSiteName = cleanText(p.OGSiteName)

	free := true
	for _, t := range ldTexts {
		v := parseJSONLD(t)
		if v == nil {
			continue
		}
		if notFree(v) {
			free = false
		}
		if !containsRecipe(v) {
			continue
		}
		if b, err := json.Marshal(pruneJSONLD(v)); err == nil {
			p.JSONLD = append(p.JSONLD, b)
		}
	}
	if micro != nil {
		p.Microdata = outerHTML(micro)
	}
	if hrec != nil {
		p.HRecipe = outerHTML(hrec)
	}

	lower := bytes.ToLower(body)
	for _, m := range botMarkers {
		if bytes.Contains(lower, []byte(m)) {
			p.Markers = append(p.Markers, m)
		}
	}
	noRecipe := len(p.JSONLD) == 0 && p.Microdata == "" && p.HRecipe == ""
	if noRecipe && len(body) < challengeMaxBytes && bytes.Contains(lower, []byte("challenge")) {
		p.Markers = append(p.Markers, "challenge")
	}
	if !free {
		p.Markers = append(p.Markers, MarkerNotFree)
	}
	return p
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return true
		}
	}
	return false
}

func hasToken(list, tok string) bool {
	for _, f := range strings.Fields(list) {
		if strings.EqualFold(f, tok) {
			return true
		}
	}
	return false
}

func isRecipeItemtype(s string) bool {
	for _, f := range strings.Fields(strings.ToLower(s)) {
		if strings.HasSuffix(strings.TrimSuffix(f, "/"), "schema.org/recipe") {
			return true
		}
	}
	return false
}

// text is the concatenated text under n.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// outerHTML renders n without its scripts, styles and comments. It
// detaches them from the tree, which Scan no longer needs.
func outerHTML(n *html.Node) string {
	var strip func(*html.Node)
	strip = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.CommentNode || (c.Type == html.ElementNode && (c.DataAtom == atom.Script || c.DataAtom == atom.Style)) {
				n.RemoveChild(c)
			} else {
				strip(c)
			}
			c = next
		}
	}
	strip(n)
	var b bytes.Buffer
	if err := html.Render(&b, n); err != nil {
		return ""
	}
	return b.String()
}

var metaCharsetRE = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_:.-]+)`)

// decodeCharset turns a page declared as Windows-1252 or Latin-1 into
// UTF-8. Anything else is passed through: a page that is valid UTF-8
// despite its declaration is taken as UTF-8.
func decodeCharset(body []byte, contentType string) []byte {
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	cs := ""
	if _, params, err := mime.ParseMediaType(contentType); err == nil {
		cs = params["charset"]
	}
	if cs == "" {
		head := body
		if len(head) > 1024 {
			head = head[:1024]
		}
		if m := metaCharsetRE.FindSubmatch(head); m != nil {
			cs = string(m[1])
		}
	}
	switch strings.ToLower(strings.TrimSpace(cs)) {
	case "windows-1252", "cp1252", "x-cp1252", "iso-8859-1", "iso8859-1", "latin1", "latin-1", "l1", "us-ascii", "ascii":
	default:
		return body
	}
	if utf8.Valid(body) {
		return body
	}
	return decode1252(body)
}

// cp1252 is Windows-1252 at 0x80–0x9F; browsers read Latin-1 as it too.
var cp1252 = [32]rune{
	'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8d, 'Ž', 0x8f,
	0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9d, 'ž', 'Ÿ',
}

func decode1252(b []byte) []byte {
	out := make([]byte, 0, len(b)+len(b)/8)
	for _, c := range b {
		switch {
		case c < 0x80:
			out = append(out, c)
		case c < 0xa0:
			out = utf8.AppendRune(out, cp1252[c-0x80])
		default:
			out = utf8.AppendRune(out, rune(c))
		}
	}
	return out
}
