package recipes

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"strings"

	"github.com/sroberts/sous/internal/identity"
)

// NormalizeSourceURL reduces a link to the form used to decide whether two
// links name the same page: http equals https, the host is lower-case without
// "www.", the fragment, tracking parameters and trailing slash are gone, and
// the remaining parameters are sorted. Input that doesn't parse is returned
// trimmed.
func NormalizeSourceURL(u string) string {
	u = strings.TrimSpace(u)
	p, err := url.Parse(u)
	if err != nil || p.Host == "" || (p.Scheme != "http" && p.Scheme != "https") {
		return u
	}
	host := strings.TrimPrefix(strings.ToLower(p.Host), "www.")
	q := p.Query()
	for k := range q {
		l := strings.ToLower(k)
		if strings.HasPrefix(l, "utm_") || strings.HasPrefix(l, "mc_") || l == "fbclid" || l == "gclid" {
			delete(q, k)
		}
	}
	out := "https://" + host + strings.TrimRight(p.EscapedPath(), "/")
	if enc := q.Encode(); enc != "" { // Encode sorts by key
		out += "?" + enc
	}
	return out
}

// FindBySource returns the recipes whose source link matches any of urls once
// normalized, trashed ones included (Deleted is set). Live recipes come
// first, then the most recently updated. The result is never nil.
func (s *Service) FindBySource(ctx context.Context, c identity.Caller, urls ...string) ([]DuplicateRef, error) {
	want := map[string]bool{}
	for _, u := range urls {
		if n := NormalizeSourceURL(u); n != "" {
			want[n] = true
		}
	}
	out := []DuplicateRef{}
	if len(want) == 0 {
		return out, nil
	}
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, title, version, deleted_at, source_url FROM recipes
			WHERE source_url IS NOT NULL ORDER BY deleted_at IS NOT NULL, updated_at DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d DuplicateRef
			var deleted, src sql.NullString
			if err := rows.Scan(&d.ID, &d.Title, &d.Version, &deleted, &src); err != nil {
				return err
			}
			if want[NormalizeSourceURL(src.String)] {
				d.Deleted = deleted.Valid
				out = append(out, d)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return slices.Clip(out), nil
}
