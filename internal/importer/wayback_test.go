package importer

import "testing"

func TestWaybackParts(t *testing.T) {
	for _, tc := range []struct {
		in, ts, orig string
		ok           bool
	}{
		{"https://web.archive.org/web/20231120222809/https://www.myrecipes.com/recipe/curried-lentil-tomato-soup", "20231120222809", "https://www.myrecipes.com/recipe/curried-lentil-tomato-soup", true},
		{"https://web.archive.org/web/20231120222809id_/https://www.myrecipes.com/recipe/x", "20231120222809", "https://www.myrecipes.com/recipe/x", true},
		{"http://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/a.jpg", "20231120222809", "https://imagesvc.meredithcorp.io/a.jpg", true},
		{"/web/20231120222809/https://imagesvc.meredithcorp.io/a.jpg?w=1200", "20231120222809", "https://imagesvc.meredithcorp.io/a.jpg?w=1200", true},
		{"https://web.archive.org/web/20231120222809/https:/www.example.com/x", "20231120222809", "https://www.example.com/x", true},
		{"https://web.archive.org/web/20231120222809/www.example.com/x", "20231120222809", "http://www.example.com/x", true},
		{"https://web.archive.org/web/2023/https://example.com/", "", "", false},
		{"https://example.com/web/20231120222809/https://x.com/", "", "", false},
		{"https://www.myrecipes.com/recipe/x", "", "", false},
		{"", "", "", false},
	} {
		ts, orig, ok := WaybackParts(tc.in)
		if ts != tc.ts || orig != tc.orig || ok != tc.ok {
			t.Errorf("WaybackParts(%q) = %q, %q, %v; want %q, %q, %v", tc.in, ts, orig, ok, tc.ts, tc.orig, tc.ok)
		}
	}
}

func TestRewriteArchived(t *testing.T) {
	const ts, orig = "20231120222809", "https://www.myrecipes.com/recipe/curried-lentil-tomato-soup"
	for _, tc := range []struct{ in, want string }{
		// What the real page showed: already rewritten, without im_.
		{"/web/20231120222809/https://imagesvc.meredithcorp.io/v3/mm/image?url=x", "https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/v3/mm/image?url=x"},
		{"https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/a.jpg", "https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/a.jpg"},
		{"https://web.archive.org/web/20220101000000id_/https://cdn.example.com/a.jpg", "https://web.archive.org/web/20220101000000im_/https://cdn.example.com/a.jpg"},
		{"https://imagesvc.meredithcorp.io/a.jpg", "https://web.archive.org/web/20231120222809im_/https://imagesvc.meredithcorp.io/a.jpg"},
		{"/sites/default/files/soup.jpg", "https://web.archive.org/web/20231120222809im_/https://www.myrecipes.com/sites/default/files/soup.jpg"},
		{"soup.jpg", "https://web.archive.org/web/20231120222809im_/https://www.myrecipes.com/recipe/soup.jpg"},
		{"", ""},
	} {
		if got := RewriteArchived(ts, orig, tc.in); got != tc.want {
			t.Errorf("RewriteArchived(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
	if got := WaybackFetchURL(ts, orig); got != "https://web.archive.org/web/20231120222809id_/"+orig {
		t.Errorf("WaybackFetchURL = %q", got)
	}
}
