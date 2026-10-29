package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func riff(chunk string, data []byte) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBP" + chunk + "\x00\x00\x00\x00")
	b = append(b, data...)
	return append(b, make([]byte, 16)...)
}

func TestPutImageDedupes(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	defer s.Close()
	data := testPNG(t, 3, 2)

	m1, err := s.PutImage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	m2, err := s.PutImage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	if m1.Hash != want || m2.Hash != want {
		t.Fatalf("hashes %s, %s, want %s", m1.Hash, m2.Hash, want)
	}
	if m1.ContentType != "image/png" || m1.Bytes != int64(len(data)) {
		t.Errorf("meta = %+v", m1)
	}
	if m1.Width == nil || *m1.Width != 3 || *m1.Height != 2 {
		t.Errorf("dimensions = %v x %v", m1.Width, m1.Height)
	}

	files, err := filepath.Glob(filepath.Join(dir, "images", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != filepath.Join(dir, "images", want[:2], want+".png") {
		t.Fatalf("image files = %v", files)
	}
	if tmp, _ := os.ReadDir(filepath.Join(dir, "tmp")); len(tmp) != 0 {
		t.Errorf("tmp left with %d entries", len(tmp))
	}

	f, meta, err := s.OpenImage(want)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _ := io.ReadAll(f)
	if !bytes.Equal(got, data) || meta.ContentType != "image/png" || meta.Bytes != int64(len(data)) {
		t.Errorf("OpenImage meta = %+v, %d bytes", meta, len(got))
	}
}

func TestPutImageRejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	defer s.Close()
	for name, data := range map[string][]byte{
		"text":  []byte("hello, this is not an image at all"),
		"gif":   []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"),
		"avif":  append([]byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avifmif1miaf"), make([]byte, 32)...),
		"empty": nil,
	} {
		if _, err := s.PutImage(bytes.NewReader(data)); !errors.Is(err, ErrUnsupportedImage) {
			t.Errorf("%s: err = %v, want ErrUnsupportedImage", name, err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "images", "*", "*"))
	tmp, _ := os.ReadDir(filepath.Join(dir, "tmp"))
	if len(files) != 0 || len(tmp) != 0 {
		t.Errorf("left files behind: images %v, tmp %d", files, len(tmp))
	}
}

func TestPutImageTypes(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()

	// VP8L: signature 0x2f, then 14-bit width-1 and height-1.
	v := uint32(10-1) | uint32(7-1)<<14
	vp8l := []byte{0x2f, byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
	// VP8: 3-byte frame tag, start code, 16-bit LE width and height.
	vp8 := []byte{0, 0, 0, 0x9d, 0x01, 0x2a, 20, 0, 15, 0}
	// VP8X: 4 flag bytes, then 24-bit LE canvas width-1 and height-1.
	vp8x := []byte{0, 0, 0, 0, 99, 0, 0, 49, 0, 0}
	heic := append([]byte("\x00\x00\x00\x18ftypmif1\x00\x00\x00\x00mif1heic"), make([]byte, 32)...)

	cases := []struct {
		name string
		data []byte
		ct   string
		w, h int // 0 means nil
	}{
		{"jpeg", testJPEG(t, 5, 4), "image/jpeg", 5, 4},
		{"png", testPNG(t, 2, 9), "image/png", 2, 9},
		{"vp8l", riff("VP8L", vp8l), "image/webp", 10, 7},
		{"vp8", riff("VP8 ", vp8), "image/webp", 20, 15},
		{"vp8x", riff("VP8X", vp8x), "image/webp", 100, 50},
		{"heic", heic, "image/heic", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := s.PutImage(bytes.NewReader(c.data))
			if err != nil {
				t.Fatal(err)
			}
			if m.ContentType != c.ct {
				t.Errorf("ContentType = %q, want %q", m.ContentType, c.ct)
			}
			if c.w == 0 {
				if m.Width != nil || m.Height != nil {
					t.Errorf("dimensions = %v x %v, want nil", m.Width, m.Height)
				}
			} else if m.Width == nil || m.Height == nil || *m.Width != c.w || *m.Height != c.h {
				t.Errorf("dimensions wrong: %+v", m)
			}
			f, meta, err := s.OpenImage(m.Hash)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			if meta.ContentType != c.ct || !strings.HasSuffix(f.Name(), "."+extFor(c.ct)) {
				t.Errorf("OpenImage = %+v, %s", meta, f.Name())
			}
		})
	}
}

func TestOpenImageNotFound(t *testing.T) {
	s := openTest(t, t.TempDir())
	defer s.Close()
	for _, h := range []string{strings.Repeat("a", 64), "../../sous.db", strings.Repeat("A", 64)} {
		if _, _, err := s.OpenImage(h); !errors.Is(err, ErrImageNotFound) {
			t.Errorf("OpenImage(%q) err = %v", h, err)
		}
	}
}
