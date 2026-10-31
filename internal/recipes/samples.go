package recipes

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/storage"
)

// Sample is a recipe a new library starts with. Recipe.Images must be
// empty; Image, if set, becomes its only image.
type Sample struct {
	Recipe RecipeInput
	Image  []byte
}

// systemCaller is Sous adding something on its own.
var systemCaller = identity.Caller{
	Person: &identity.Person{Username: identity.SystemUsername, Name: identity.SystemName},
	Route:  identity.RouteNone,
	Via:    identity.ViaSystem,
}

// SeedSamples adds samples, by the system person, to a library that has
// never been used: no changes logged and no recipes, deleted ones included.
// Anything else is left alone, so deleting the samples is final. It returns
// how many it added, which is 0 or len(samples): they go in one transaction
// that also adds the system person, and log one "Added" change each.
func (s *Service) SeedSamples(ctx context.Context, samples []Sample) (int, error) {
	for _, sm := range samples {
		if err := validateInput(sm.Recipe); err != nil {
			return 0, fmt.Errorf("sample %q: %w", sm.Recipe.Title, err)
		}
		if len(sm.Recipe.Images) > 0 {
			return 0, fmt.Errorf("sample %q: images come from Image, not hashes", sm.Recipe.Title)
		}
	}
	// Checked first so an established library never gets image files.
	var used bool
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		used, err = libraryUsed(ctx, tx)
		return err
	})
	if err != nil || used || len(samples) == 0 {
		return 0, err
	}

	metas := make([]*storage.ImageMeta, len(samples))
	for i, sm := range samples {
		if sm.Image == nil {
			continue
		}
		meta, err := s.st.PutImage(bytes.NewReader(sm.Image))
		if err != nil {
			return 0, fmt.Errorf("sample %q: %w", sm.Recipe.Title, err)
		}
		metas[i] = &meta
	}

	now := s.stamp()
	added := 0
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		// Someone may have started using the library since the check.
		if used, err := libraryUsed(ctx, tx); err != nil || used {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO people (username, name, source, created_at, updated_at)
			VALUES (?, ?, 'system', ?, ?) ON CONFLICT (username) DO NOTHING`,
			identity.SystemUsername, identity.SystemName, now, now); err != nil {
			return err
		}
		var source string
		if err := tx.QueryRowContext(ctx, `SELECT source FROM people WHERE username = ?`,
			identity.SystemUsername).Scan(&source); err != nil {
			return err
		}
		if source != "system" {
			return errors.New("recipes: the username " + identity.SystemUsername + " belongs to a " + source + " person")
		}
		for i, sm := range samples {
			in := sm.Recipe
			if m := metas[i]; m != nil {
				if err := insertImage(ctx, tx, *m, identity.SystemUsername, now); err != nil {
					return err
				}
				in.Images = []string{m.Hash}
			}
			if _, err := s.createTx(ctx, tx, systemCaller, in, now); err != nil {
				return fmt.Errorf("sample %q: %w", in.Title, err)
			}
		}
		added = len(samples)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

// libraryUsed reports whether anything has ever been written: a change, or
// a recipe (even one hard-deleted later leaves its change behind).
func libraryUsed(ctx context.Context, tx *sql.Tx) (bool, error) {
	var used bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM changes) OR EXISTS (SELECT 1 FROM recipes)`).Scan(&used)
	return used, err
}
