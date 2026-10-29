package recipes

import (
	"reflect"
	"testing"
)

func TestNormalizeSourceURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/a":                           "https://example.com/a",
		"http://example.com/a":                            "https://example.com/a",
		"HTTP://WWW.Example.COM/A/":                       "https://example.com/A",
		"https://example.com/a#step-2":                    "https://example.com/a",
		"https://example.com/a?utm_source=x&id=5":         "https://example.com/a?id=5",
		"https://example.com/a?fbclid=1&gclid=2&mc_cid=3": "https://example.com/a",
		"https://example.com/a?b=2&a=1":                   "https://example.com/a?a=1&b=2",
		"https://example.com/":                            "https://example.com",
		"https://example.com":                             "https://example.com",
		"  https://example.com/a/  ":                      "https://example.com/a",
		"not a url":                                       "not a url",
		"  ":                                              "",
		"ftp://example.com/a":                             "ftp://example.com/a",
		"https://exa mple.com/%zz":                        "https://exa mple.com/%zz",
	} {
		if got := NormalizeSourceURL(in); got != want {
			t.Errorf("NormalizeSourceURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindBySource(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	mk := func(title, src string) Recipe {
		return create(t, s, scott, RecipeInput{Title: title, SourceURL: sp(src), Ingredients: []string{"1 egg"}, Steps: []string{"Cook."}})
	}
	live := mk("Live", "https://example.com/soup")
	gone := mk("Gone", "https://www.example.com/soup/?utm_source=a")
	mk("Other", "https://example.com/stew")
	create(t, s, scott, RecipeInput{Title: "No source", Ingredients: []string{"1 egg"}, Steps: []string{"Cook."}})
	if err := s.Delete(ctx, scott, gone.ID, gone.Version); err != nil {
		t.Fatal(err)
	}

	got, err := s.FindBySource(ctx, scott, "http://EXAMPLE.com/soup#top", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []DuplicateRef{
		{ID: live.ID, Title: "Live", Version: live.Version},
		{ID: gone.ID, Title: "Gone", Version: gone.Version + 1, Deleted: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	for _, urls := range [][]string{{"https://example.com/other"}, {""}, nil} {
		got, err := s.FindBySource(ctx, scott, urls...)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("FindBySource(%q) = %#v, want empty non-nil", urls, got)
		}
	}
}
