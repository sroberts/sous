package importer

import (
	"mime"
	"net/http"
	"strings"

	"github.com/sroberts/sous/internal/recipes"
)

var messages = map[string]string{
	recipes.CodeBlocked:        "The site turned Sous away. Paste the recipe instead.",
	recipes.CodePaywalled:      "The site keeps this recipe for subscribers. Paste the recipe instead.",
	recipes.CodePageNotFound:   "The site says that page doesn't exist.",
	recipes.CodeNoRecipe:       "Sous couldn't find a recipe on that page. Paste the recipe instead.",
	recipes.CodeTimeout:        "The site took too long to answer.",
	recipes.CodePageTooLarge:   "The page is too large to read.",
	recipes.CodeUnsupportedURL: "Sous can only read web pages at http or https links.",
	recipes.CodeSiteError:      "The site had a problem serving that page. Try again later.",
}

// spoken holds a sentence per code for a shortcut to read aloud after a
// failed capture: no advice to paste, since there is nowhere to paste it.
var spoken = map[string]string{
	recipes.CodeBlocked:        "The site turned Sous away, so nothing was saved.",
	recipes.CodePaywalled:      "That recipe is only for the site's subscribers, so nothing was saved.",
	recipes.CodePageNotFound:   "The site says that page doesn't exist, so nothing was saved.",
	recipes.CodeNoRecipe:       "Sous couldn't find a recipe on that page, so nothing was saved.",
	recipes.CodeTimeout:        "The site took too long to answer. Try again later.",
	recipes.CodePageTooLarge:   "That page is too large for Sous to read, so nothing was saved.",
	recipes.CodeUnsupportedURL: "Sous can only save recipes from web links, so nothing was saved.",
	recipes.CodeSiteError:      "The site had a problem serving that page. Try again later.",
}

// MessageLocalAddress is the unsupported_url message for an address on
// the local network.
const MessageLocalAddress = "Sous won't fetch addresses on the local network."

// Message is the person-facing message for an import error code.
func Message(code string) string { return messages[code] }

// Spoken is the sentence to read aloud for an import error code.
func Spoken(code string) string { return spoken[code] }

// Classify maps a fetched page that yielded no recipe to an import error
// code, or "" when extracted is true and the page is usable. Fetch errors
// (bad URL, blocked address, timeout, size cap, DNS) are mapped by the
// caller before a page exists.
func Classify(status int, header http.Header, p Page, extracted bool) (code string) {
	cfMitigated := header.Get("Cf-Mitigated") != "" || p.CFMitigated != ""
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		return recipes.CodePageNotFound
	case status == http.StatusUnauthorized || status == http.StatusForbidden ||
		status == http.StatusTooManyRequests || status == http.StatusUnavailableForLegalReasons:
		return recipes.CodeBlocked
	case status == http.StatusServiceUnavailable && (cfMitigated || p.botWall(status, extracted)):
		return recipes.CodeBlocked
	case status >= 500:
		return recipes.CodeSiteError
	case status != 0 && (status < 200 || status > 299):
		if cfMitigated {
			return recipes.CodeBlocked
		}
		return recipes.CodeNoRecipe
	}
	if !isHTML(header.Get("Content-Type")) {
		return recipes.CodeUnsupportedURL
	}
	if extracted {
		return ""
	}
	switch {
	case cfMitigated || p.botWall(status, extracted):
		return recipes.CodeBlocked
	case p.has(MarkerNotFree):
		return recipes.CodePaywalled
	}
	return recipes.CodeNoRecipe
}

func isHTML(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	t, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return t == "text/html" || t == "application/xhtml+xml"
}
