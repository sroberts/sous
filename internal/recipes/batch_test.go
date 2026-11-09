package recipes

import (
	"errors"
	"fmt"
	"testing"
)

func TestBatch(t *testing.T) {
	s := newTestService(t)
	scott := person(t, s, "Scott")
	a := create(t, s, scott, RecipeInput{Title: "Toast"})
	b := create(t, s, scott, RecipeInput{Title: "Eggs"})
	gone := create(t, s, scott, RecipeInput{Title: "Gone"})
	if err := s.Delete(ctx, scott, gone.ID, gone.Version); err != nil {
		t.Fatal(err)
	}

	got, err := s.Batch(ctx, scott, []string{b.ID, gone.ID, "nope", a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
		t.Errorf("Batch = %v", got)
	}
	if got, err := s.Batch(ctx, scott, nil); err != nil || len(got) != 0 {
		t.Errorf("Batch(nil) = %v, %v", got, err)
	}

	many := make([]string, MaxBatch+1)
	for i := range many {
		many[i] = fmt.Sprint(i)
	}
	var e *Error
	if _, err := s.Batch(ctx, scott, many); !errors.As(err, &e) || e.Code != CodeInvalid {
		t.Errorf("Batch(%d ids) err = %v, want invalid", len(many), err)
	}
	if _, err := s.Batch(ctx, scott, many[:MaxBatch]); err != nil {
		t.Errorf("Batch(%d ids) err = %v", MaxBatch, err)
	}
}
