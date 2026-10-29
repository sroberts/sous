package recipes

import (
	"context"

	"github.com/sroberts/sous/internal/identity"
)

// ListPeople returns every household member, ordered by name.
func (s *Service) ListPeople(ctx context.Context, c identity.Caller) ([]Person, error) {
	panic("TODO")
}

// CreatePerson adds a local person named name. It needs no caller. The
// username is name lower-cased, each run of [^a-z0-9-] replaced with "-",
// and "-" trimmed from both ends. An empty name or username is CodeInvalid;
// a taken username is CodeExists. It logs a person create change.
func (s *Service) CreatePerson(ctx context.Context, c identity.Caller, name string) (Person, error) {
	panic("TODO")
}

// Lookup returns the person with username, or (nil, nil) if there is none.
// It implements identity.Directory, so it takes no Caller.
func (s *Service) Lookup(ctx context.Context, username string) (*identity.Person, error) {
	panic("TODO")
}

// UpsertGateway creates or updates p with source "gateway", only writing
// when the stored name or email differs. It implements identity.Directory,
// so it takes no Caller.
func (s *Service) UpsertGateway(ctx context.Context, p identity.Person) error {
	panic("TODO")
}
