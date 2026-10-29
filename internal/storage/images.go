package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg" // registers DecodeConfig
	_ "image/png"
	"io"
	"os"
	"path/filepath"
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

var imageExts = []struct{ contentType, ext string }{
	{"image/jpeg", "jpg"},
	{"image/png", "png"},
	{"image/webp", "webp"},
	{"image/heic", "heic"},
}

func extFor(contentType string) string {
	for _, e := range imageExts {
		if e.contentType == contentType {
			return e.ext
		}
	}
	return ""
}

// PutImage streams r into tmp/ through a SHA-256 tee, sniffs the type
// (HEIC from the ftyp brands heic, heix, mif1, msf1, hevc), reads the
// dimensions from the header, fsyncs, renames to images/ab/<hash>.<ext> and
// fsyncs the directory. If that file already exists the temp file is
// dropped. Unsupported bytes return ErrUnsupportedImage.
func (s *Store) PutImage(r io.Reader) (ImageMeta, error) {
	f, err := os.CreateTemp(filepath.Join(s.Dir, "tmp"), "upload-*")
	if err != nil {
		return ImageMeta{}, err
	}
	tmp := f.Name()
	committed := false
	defer func() {
		f.Close()
		if !committed {
			os.Remove(tmp)
		}
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return ImageMeta{}, err
	}
	meta := ImageMeta{Hash: hex.EncodeToString(h.Sum(nil)), Bytes: n}

	head := make([]byte, 256)
	k, err := f.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return ImageMeta{}, err
	}
	meta.ContentType = sniff(head[:k])
	switch meta.ContentType {
	case "":
		return ImageMeta{}, ErrUnsupportedImage
	case "image/jpeg", "image/png":
		cfg, _, err := image.DecodeConfig(io.NewSectionReader(f, 0, n))
		if err != nil {
			return ImageMeta{}, ErrUnsupportedImage
		}
		meta.Width, meta.Height = &cfg.Width, &cfg.Height
	case "image/webp":
		w, h, ok := webpSize(head[:k])
		if !ok {
			return ImageMeta{}, ErrUnsupportedImage
		}
		meta.Width, meta.Height = &w, &h
	}

	if err := f.Sync(); err != nil {
		return ImageMeta{}, err
	}
	if err := f.Close(); err != nil {
		return ImageMeta{}, err
	}

	images := filepath.Join(s.Dir, "images")
	sub := filepath.Join(images, meta.Hash[:2])
	final := filepath.Join(sub, meta.Hash+"."+extFor(meta.ContentType))
	if _, err := os.Stat(final); err == nil {
		return meta, nil
	}
	created := false
	if _, err := os.Stat(sub); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(sub, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return ImageMeta{}, err
		}
		created = true
	}
	if err := os.Rename(tmp, final); err != nil {
		return ImageMeta{}, err
	}
	committed = true
	if err := syncDir(sub); err != nil {
		return ImageMeta{}, err
	}
	if created {
		if err := syncDir(images); err != nil {
			return ImageMeta{}, err
		}
	}
	return meta, nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// OpenImage opens the stored file for hash. The returned meta has Hash,
// ContentType (from the extension) and Bytes; Width and Height are nil.
// A missing file returns ErrImageNotFound.
func (s *Store) OpenImage(hash string) (*os.File, ImageMeta, error) {
	if !validHash(hash) {
		return nil, ImageMeta{}, ErrImageNotFound
	}
	for _, e := range imageExts {
		f, err := os.Open(filepath.Join(s.Dir, "images", hash[:2], hash+"."+e.ext))
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, ImageMeta{}, err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, ImageMeta{}, err
		}
		return f, ImageMeta{Hash: hash, ContentType: e.contentType, Bytes: info.Size()}, nil
	}
	return nil, ImageMeta{}, ErrImageNotFound
}

// validHash also keeps path separators out of the file name.
func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

var heicBrands = map[string]bool{"heic": true, "heix": true, "mif1": true, "msf1": true, "hevc": true}

func sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	case isHEIC(b):
		return "image/heic"
	}
	return ""
}

// isHEIC reads the ftyp box: size, "ftyp", major brand, minor version, then
// compatible brands. AVIF also lists mif1, so an avif brand disqualifies.
func isHEIC(b []byte) bool {
	if len(b) < 16 || string(b[4:8]) != "ftyp" {
		return false
	}
	size := int(binary.BigEndian.Uint32(b[0:4]))
	if size < 16 || size > len(b) {
		size = len(b)
	}
	brands := []string{string(b[8:12])}
	for i := 16; i+4 <= size; i += 4 {
		brands = append(brands, string(b[i:i+4]))
	}
	found := false
	for _, br := range brands {
		if br == "avif" || br == "avis" {
			return false
		}
		found = found || heicBrands[br]
	}
	return found
}

// webpSize reads the canvas size from the first chunk after the RIFF
// header (VP8, VP8L or VP8X), avoiding a dependency on x/image.
func webpSize(b []byte) (w, h int, ok bool) {
	if len(b) < 30 {
		return 0, 0, false
	}
	d := b[20:]
	switch string(b[12:16]) {
	case "VP8 ":
		if d[3] != 0x9d || d[4] != 0x01 || d[5] != 0x2a {
			return 0, 0, false
		}
		w = int(binary.LittleEndian.Uint16(d[6:8]) & 0x3fff)
		h = int(binary.LittleEndian.Uint16(d[8:10]) & 0x3fff)
	case "VP8L":
		if d[0] != 0x2f {
			return 0, 0, false
		}
		v := binary.LittleEndian.Uint32(d[1:5])
		w = int(v&0x3fff) + 1
		h = int(v>>14&0x3fff) + 1
	case "VP8X":
		w = int(uint32(d[4])|uint32(d[5])<<8|uint32(d[6])<<16) + 1
		h = int(uint32(d[7])|uint32(d[8])<<8|uint32(d[9])<<16) + 1
	default:
		return 0, 0, false
	}
	return w, h, true
}
