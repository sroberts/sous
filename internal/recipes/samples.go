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

// ErrSystemUsernameTaken is SeedSamples finding someone other than Sous
// itself holding its username, as a library from before the system person
// could. The samples are not added, rather than credited to them.
var ErrSystemUsernameTaken = errors.New("recipes: the username " + identity.SystemUsername + " is taken")

// SeedSamples adds samples, by the system person, to a library that has
// never been used: no changes logged and no recipes, deleted ones included.
// Anything else is left alone, so deleting the samples is final. It returns
// how many it added, which is 0 or len(samples): they go in one transaction
// that also adds the system person, and log one "Added" change each. If
// another person holds the system username it adds none and returns an
// error wrapping ErrSystemUsernameTaken.
func (s *Service) SeedSamples(ctx context.Context, samples []Sample) (int, error) {
	for _, sm := range samples {
		if err := validateInput(sm.Recipe); err != nil {
			return 0, fmt.Errorf("sample %q: %w", sm.Recipe.Title, err)
		}
		if len(sm.Recipe.Images) > 0 {
			return 0, fmt.Errorf("sample %q: images come from Image, not hashes", sm.Recipe.Title)
		}
	}
	// Checked before any image is written, so an established library, or
	// one where seeding can't succeed, never gets image files.
	var used bool
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		if used, err = libraryUsed(ctx, tx); err != nil || used {
			return err
		}
		return systemUsernameFree(ctx, tx)
	})
	if err != nil || used || len(samples) == 0 {
		return 0, err
	}

	// The images go in before the transaction, as uploads do: PutImage
	// writes files, not rows. If the transaction then fails they are left
	// orphaned, which spec §3 allows.
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
		if err := systemUsernameFree(ctx, tx); err != nil {
			return err
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

// LibraryUsed reports whether the library has ever been used, in the sense
// SeedSamples needs: a change logged or a recipe added, deleted ones
// included. A caller can check it before going to the trouble of loading
// samples.
func (s *Service) LibraryUsed(ctx context.Context) (bool, error) {
	var used bool
	err := s.st.Read(ctx, func(tx *sql.Tx) error {
		var err error
		used, err = libraryUsed(ctx, tx)
		return err
	})
	return used, err
}

// systemUsernameFree returns an error wrapping ErrSystemUsernameTaken if
// someone other than Sous itself holds its username.
func systemUsernameFree(ctx context.Context, tx *sql.Tx) error {
	var source string
	err := tx.QueryRowContext(ctx, `SELECT source FROM people WHERE username = ?`,
		identity.SystemUsername).Scan(&source)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	case source != "system":
		return fmt.Errorf("%w by a %s person", ErrSystemUsernameTaken, source)
	}
	return nil
}

// libraryUsed reports whether anything has ever been written: a change, or
// a recipe (even one hard-deleted later leaves its change behind).
func libraryUsed(ctx context.Context, tx *sql.Tx) (bool, error) {
	var used bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM changes) OR EXISTS (SELECT 1 FROM recipes)`).Scan(&used)
	return used, err
}
