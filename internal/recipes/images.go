package recipes

import (
	"context"
	"io"
	"os"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/storage"
)

// UploadImage stores the bytes from r and records them in the images table
// (once per hash). The type is sniffed, not taken from the request: anything
// but jpeg, png, webp or heic is CodeUnsupportedMediaType. Not logged as a
// change.
func (s *Service) UploadImage(ctx context.Context, c identity.Caller, r io.Reader) (ImageRef, error) {
	panic("TODO")
}

// Image opens the stored image for hash. The caller closes the file. An
// unknown hash is CodeNotFound.
func (s *Service) Image(ctx context.Context, c identity.Caller, hash string) (*os.File, storage.ImageMeta, error) {
	panic("TODO")
}
