package recipes

import (
	"context"

	"github.com/sroberts/sous/internal/identity"
)

// Get returns the full recipe with the caller's Mine flags. A deleted recipe
// is returned with DeletedAt set; an unknown id is CodeNotFound.
func (s *Service) Get(ctx context.Context, c identity.Caller, id string) (Recipe, error) {
	panic("TODO")
}

// Create stores a new recipe at version 1 with a UUIDv7 id, parsing
// ingredients and steps, deriving servings, creating unknown tags and
// checking image hashes exist. A blank title is CodeInvalid.
func (s *Service) Create(ctx context.Context, c identity.Caller, in RecipeInput) (Recipe, error) {
	panic("TODO")
}

// Update applies patch if the recipe is still at version, bumping it.
// A stale version is CodeStale with Current set (read in the same
// transaction); a deleted recipe is CodeDeleted. The change summary names
// the changed fields: "Edited Pad Thai: ingredients, steps".
func (s *Service) Update(ctx context.Context, c identity.Caller, id string, version int, patch RecipePatch) (Recipe, error) {
	panic("TODO")
}

// Delete soft-deletes the recipe at version, setting deleted_at and bumping
// the version. Stale is CodeStale with Current; already deleted is
// CodeDeleted.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id string, version int) error {
	panic("TODO")
}

// Restore undeletes a recipe, bumping its version. Restoring a recipe that
// isn't deleted is a no-op that returns it unchanged.
func (s *Service) Restore(ctx context.Context, c identity.Caller, id string) (Recipe, error) {
	panic("TODO")
}

// SetMine merges p into the caller's flags on the recipe and returns the
// result. It is not recorded in the change log.
func (s *Service) SetMine(ctx context.Context, c identity.Caller, id string, p MinePatch) (Mine, error) {
	panic("TODO")
}
