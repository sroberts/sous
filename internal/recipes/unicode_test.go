package recipes

import (
	"slices"
	"strings"
	"testing"
)

func TestUnicodeCaseInFilters(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	create(t, s, scott, RecipeInput{
		Title: "Veal", Tags: []string{"Été"},
		Ingredients: []string{"500 g Émincé de veau"},
	})
	if got := list(t, s, scott, ListQuery{Ingredient: []string{"émincé"}}); !slices.Equal(got, []string{"Veal"}) {
		t.Errorf("ingredient émincé = %v", got)
	}
	if got := list(t, s, scott, ListQuery{Tag: []string{"été"}}); !slices.Equal(got, []string{"Veal"}) {
		t.Errorf("tag été = %v", got)
	}
}

func TestNonLatinNames(t *testing.T) {
	s := newTestService(t)
	p, err := s.CreatePerson(ctx, person(t, s, "Scott"), "李明")
	if err != nil {
		t.Fatalf("CreatePerson(李明): %v", err)
	}
	if !strings.HasPrefix(p.Username, "p-") || p.Name != "李明" {
		t.Errorf("person = %+v", p)
	}
	if slug("李明") != p.Username {
		t.Error("slug is not stable")
	}
}
