package storage

import (
	"errors"
	"io"
	"os"
)

// ErrUnsupportedImage is returned by PutImage when the bytes are not JPEG,
// PNG, WebP or HEIC.
var ErrUnsupportedImage = errors.New("storage: unsupported image type")

// ErrImageNotFound is returned by OpenImage when no file has the hash.
var ErrImageNotFound = errors.New("storage: image not found")

// ImageMeta describes a stored image file.
type ImageMeta struct {
	Hash        string // lower-case hex SHA-256 of the bytes
	ContentType string // image/jpeg, image/png, image/webp or image/heic
	Width       *int   // nil when unknown (always for HEIC)
	Height      *int
	Bytes       int64
}

// PutImage streams r into tmp/ through a SHA-256 tee, sniffs the type
// (HEIC from the ftyp brands heic, heix, mif1, msf1, hevc), reads the
// dimensions from the header, fsyncs, renames to images/ab/<hash>.<ext> and
// fsyncs the directory. If that file already exists the temp file is
// dropped. Unsupported bytes return ErrUnsupportedImage.
func (s *Store) PutImage(r io.Reader) (ImageMeta, error) {
	panic("TODO")
}

// OpenImage opens the stored file for hash. The returned meta has Hash,
// ContentType (from the extension) and Bytes; Width and Height are nil.
// A missing file returns ErrImageNotFound.
func (s *Store) OpenImage(hash string) (*os.File, ImageMeta, error) {
	panic("TODO")
}
