package recipes

import (
	"context"
	"database/sql"
	"errors"
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
	who, err := author(c)
	if err != nil {
		return ImageRef{}, err
	}
	meta, err := s.st.PutImage(r)
	if errors.Is(err, storage.ErrUnsupportedImage) {
		return ImageRef{}, Errorf(CodeUnsupportedMediaType, "images must be JPEG, PNG, WebP or HEIC")
	} else if err != nil {
		return ImageRef{}, err
	}
	now := s.stamp()
	err = s.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO images (hash, content_type, width, height, bytes, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (hash) DO NOTHING`,
			meta.Hash, meta.ContentType, meta.Width, meta.Height, meta.Bytes, now, who)
		return err
	})
	if err != nil {
		return ImageRef{}, err
	}
	return ImageRef{Hash: meta.Hash}, nil
}

// Image opens the stored image for hash. The caller closes the file. An
// unknown hash is CodeNotFound.
func (s *Service) Image(ctx context.Context, c identity.Caller, hash string) (*os.File, storage.ImageMeta, error) {
	f, meta, err := s.st.OpenImage(hash)
	if errors.Is(err, storage.ErrImageNotFound) {
		return nil, storage.ImageMeta{}, Errorf(CodeNotFound, "no such image")
	} else if err != nil {
		return nil, storage.ImageMeta{}, err
	}
	err = s.st.Read(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT width, height FROM images WHERE hash = ?`, hash).Scan(&meta.Width, &meta.Height)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		f.Close()
		return nil, storage.ImageMeta{}, err
	}
	return f, meta, nil
}
