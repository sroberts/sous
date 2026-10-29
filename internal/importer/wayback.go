package importer

import (
	"net/url"
	"regexp"
	"strings"
)

var waybackRE = regexp.MustCompile(`^(?:https?:)?(?://(?:web\.)?archive\.org)?/web/(\d{14})[a-z_]*/(.+)$`)

// WaybackParts splits a web.archive.org/web/<timestamp>[flags]/<original>
// URL. A root-relative /web/… path counts too, since archived pages carry
// those. ok is false for anything else.
func WaybackParts(raw string) (ts, orig string, ok bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "/web/") {
		u, err := url.Parse(raw)
		if err != nil || !isArchiveHost(u.Hostname()) {
			return "", "", false
		}
	}
	m := waybackRE.FindStringSubmatch(raw)
	if m == nil {
		return "", "", false
	}
	orig = m[2]
	// Wayback collapses "https://" to "https:/" in some links.
	for _, s := range []string{"http:/", "https:/"} {
		if strings.HasPrefix(orig, s) && !strings.HasPrefix(orig, s+"/") {
			orig = s + "/" + orig[len(s):]
		}
	}
	if !strings.HasPrefix(orig, "http://") && !strings.HasPrefix(orig, "https://") {
		orig = "http://" + orig
	}
	return m[1], orig, true
}

func isArchiveHost(h string) bool {
	h = strings.ToLower(h)
	return h == "web.archive.org" || h == "archive.org"
}

// WaybackFetchURL is the archived copy of orig without the Wayback toolbar
// or rewritten links.
func WaybackFetchURL(ts, orig string) string {
	return "https://web.archive.org/web/" + ts + "id_/" + orig
}

// RewriteArchived points an image URL from an archived page at the
// archive's raw image copy: an archive URL with any flag becomes
// /web/<ts>im_/<asset>, an absolute URL is wrapped, and a relative one is
// resolved against the original page first.
func RewriteArchived(ts, orig, u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if ats, asset, ok := WaybackParts(u); ok {
		return WaybackImageURL(ats, asset)
	}
	ref, err := url.Parse(u)
	if err != nil {
		return u
	}
	if !ref.IsAbs() {
		base, err := url.Parse(orig)
		if err != nil {
			return u
		}
		ref = base.ResolveReference(ref)
	}
	if isArchiveHost(ref.Hostname()) {
		return ref.String()
	}
	return WaybackImageURL(ts, ref.String())
}

// WaybackImageURL is the archive's raw copy of an image.
func WaybackImageURL(ts, asset string) string {
	return "https://web.archive.org/web/" + ts + "im_/" + asset
}
