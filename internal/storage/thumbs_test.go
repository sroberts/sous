package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func put(t *testing.T, s *Store, b []byte) string {
	t.Helper()
	m, err := s.PutImage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return m.Hash
}

func readAll(t *testing.T, f *os.File) []byte {
	t.Helper()
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// insertSegment puts a marker segment straight after a JPEG's SOI.
func insertSegment(jpg []byte, marker byte, payload []byte) []byte {
	seg := []byte{0xFF, marker, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, jpg[2:]...)
}

// exifWithOrientation is an APP1 payload holding one IFD0 entry.
func exifWithOrientation(o int, bo binary.ByteOrder) []byte {
	t := make([]byte, 8+2+12+4)
	if bo == binary.LittleEndian {
		copy(t, "II")
	} else {
		copy(t, "MM")
	}
	bo.PutUint16(t[2:], 42)
	bo.PutUint32(t[4:], 8)
	bo.PutUint16(t[8:], 1)
	bo.PutUint16(t[10:], 0x0112)
	bo.PutUint16(t[12:], 3)
	bo.PutUint32(t[14:], 1)
	bo.PutUint16(t[18:], uint16(o))
	return append([]byte("Exif\x00\x00"), t...)
}

func near(a, b color.Color, tol int) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) bool { v := int(x>>8) - int(y>>8); return v >= -tol && v <= tol }
	return d(ar, br) && d(ag, bg) && d(ab, bb)
}

func TestThumbScalesAndKeepsIt(t *testing.T) {
	s := openTest(t, t.TempDir())
	src := image.NewRGBA(image.Rect(0, 0, 2000, 1500))
	for i := range src.Pix {
		src.Pix[i] = 0x80
	}
	h := put(t, s, encodeJPEG(t, src))

	f, meta, err := s.OpenThumb(h, 640)
	if err != nil {
		t.Fatal(err)
	}
	b := readAll(t, f)
	if meta.ContentType != "image/jpeg" || meta.Hash != h || meta.Bytes != int64(len(b)) {
		t.Errorf("meta = %+v (read %d bytes)", meta, len(b))
	}
	got := decode(t, b)
	if got.Bounds().Dx() != 640 || got.Bounds().Dy() != 480 {
		t.Errorf("size = %v, want 640x480", got.Bounds())
	}
	// A flat grey stays the same grey: averaging neither darkens nor shifts.
	if c := got.At(320, 240); !near(c, color.RGBA{0x80, 0x80, 0x80, 0xff}, 3) {
		t.Errorf("centre = %v, want about #808080", c)
	}
	kept := s.thumbPath(h, 640, "jpg")
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("thumbnail not kept: %v", err)
	}

	// Asked again, it is the kept file, not made afresh.
	if err := os.WriteFile(kept, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, _, err = s.OpenThumb(h, 640)
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, f); string(got) != "kept" {
		t.Errorf("second ask made it again")
	}
	// tmp/ holds nothing afterwards.
	if es, _ := os.ReadDir(filepath.Join(s.Dir, "tmp")); len(es) != 0 {
		t.Errorf("tmp has %d entries", len(es))
	}
}

func TestThumbAveragesDetail(t *testing.T) {
	s := openTest(t, t.TempDir())
	// One-pixel black and white columns average to mid grey, not to
	// whichever column a sampler lands on.
	src := image.NewGray(image.Rect(0, 0, 1280, 64))
	for y := range 64 {
		for x := range 1280 {
			if x%2 == 0 {
				src.SetGray(x, y, color.Gray{255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	f, meta, err := s.OpenThumb(put(t, s, buf.Bytes()), 320)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ContentType != "image/jpeg" {
		t.Errorf("an opaque PNG's thumbnail is %s, want image/jpeg", meta.ContentType)
	}
	got := decode(t, readAll(t, f))
	if got.Bounds().Dx() != 320 || got.Bounds().Dy() != 16 {
		t.Fatalf("size = %v, want 320x16", got.Bounds())
	}
	if c := got.At(160, 8); !near(c, color.Gray{128}, 4) {
		t.Errorf("centre = %v, want about grey 128", c)
	}
}

func TestThumbNeverEnlarges(t *testing.T) {
	s := openTest(t, t.TempDir())
	orig := testJPEG(t, 300, 200)
	h := put(t, s, orig)
	for _, w := range []int{300, 640} {
		f, meta, err := s.OpenThumb(h, w)
		if err != nil {
			t.Fatal(err)
		}
		if got := readAll(t, f); !bytes.Equal(got, orig) || meta.ContentType != "image/jpeg" {
			t.Errorf("w=%d: not the original (%d bytes, %s)", w, len(got), meta.ContentType)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "thumbs")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("thumbs/ made for an image that needed none: %v", err)
	}
}

// The thumbnail is drawn the way the photo is meant to be seen, for every
// EXIF orientation, read in either byte order.
func TestThumbFollowsEXIFOrientation(t *testing.T) {
	red, green, blue, white := color.RGBA{220, 30, 30, 255}, color.RGBA{30, 200, 30, 255}, color.RGBA{30, 30, 220, 255}, color.RGBA{250, 250, 250, 255}
	type grid [2][2]color.RGBA // [row][col] as stored
	stored := grid{{red, green}, {blue, white}}
	// The EXIF definitions, as operations on the stored picture.
	rot90 := func(g grid) grid { return grid{{g[1][0], g[0][0]}, {g[1][1], g[0][1]}} } // clockwise
	flipH := func(g grid) grid { return grid{{g[0][1], g[0][0]}, {g[1][1], g[1][0]}} }
	flipV := func(g grid) grid { return grid{g[1], g[0]} }
	want := map[int]grid{
		1: stored,
		2: flipH(stored),
		3: rot90(rot90(stored)),
		4: flipV(stored),
		5: rot90(rot90(rot90(flipH(stored)))), // mirror, then 270° clockwise
		6: rot90(stored),
		7: rot90(flipH(stored)), // mirror, then 90° clockwise
		8: rot90(rot90(rot90(stored))),
	}
	// Stored 1600x800: each quadrant 800x400.
	src := image.NewRGBA(image.Rect(0, 0, 1600, 800))
	for y := range 800 {
		for x := range 1600 {
			src.Set(x, y, stored[y/400][x/800])
		}
	}
	base := encodeJPEG(t, src)

	for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for o := 1; o <= 8; o++ {
			s := openTest(t, t.TempDir())
			h := put(t, s, insertSegment(base, 0xE1, exifWithOrientation(o, bo)))
			f, _, err := s.OpenThumb(h, 320)
			if err != nil {
				t.Fatal(err)
			}
			got := decode(t, readAll(t, f))
			wantW, wantH := 320, 160
			if o >= 5 {
				wantH = 640 // shown 800 wide and 1600 tall, scaled to 320 wide
			}
			if got.Bounds().Dx() != wantW || got.Bounds().Dy() != wantH {
				t.Errorf("%v orientation %d: size %v, want %dx%d", bo, o, got.Bounds(), wantW, wantH)
				continue
			}
			for r := range 2 {
				for c := range 2 {
					x, y := wantW/4+c*wantW/2, wantH/4+r*wantH/2
					if px := got.At(x, y); !near(px, want[o][r][c], 24) {
						t.Errorf("%v orientation %d: quadrant row %d col %d = %v, want %v", bo, o, r, c, px, want[o][r][c])
					}
				}
			}
		}
	}
}

func TestThumbKeepsTransparency(t *testing.T) {
	s := openTest(t, t.TempDir())
	src := image.NewNRGBA(image.Rect(0, 0, 800, 800))
	// An opaque red square on a ground of clear white, its edge part-way
	// through a thumbnail pixel (800/320 = 2.5 source pixels each; 201 is
	// 80.4). The clear pixels' white must not tint the red's edge.
	for i := 0; i < len(src.Pix); i += 4 {
		copy(src.Pix[i:i+4], []byte{255, 255, 255, 0})
	}
	for y := 201; y < 601; y++ {
		for x := 201; x < 601; x++ {
			src.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	f, meta, err := s.OpenThumb(put(t, s, buf.Bytes()), 320)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ContentType != "image/png" {
		t.Fatalf("content type %s, want image/png", meta.ContentType)
	}
	got := decode(t, readAll(t, f))
	if _, _, _, a := got.At(10, 10).RGBA(); a != 0 {
		t.Errorf("corner alpha = %d, want clear", a>>8)
	}
	// The colour of clear pixels (black, here) doesn't darken the edge.
	if c := color.NRGBAModel.Convert(got.At(160, 160)).(color.NRGBA); c.R < 250 || c.A != 255 {
		t.Errorf("centre = %v, want opaque red", c)
	}
	// The edge pixel is part clear: semi-transparent, and still red.
	c := color.NRGBAModel.Convert(got.At(80, 160)).(color.NRGBA)
	if c.A == 0 || c.A == 255 {
		t.Errorf("edge alpha = %d, want part-transparent", c.A)
	}
	if c.R < 245 || c.G > 10 || c.B > 10 {
		t.Errorf("edge = %v: the clear white bled into the red", c)
	}
}

func TestThumbKeepsColourProfile(t *testing.T) {
	s := openTest(t, t.TempDir())
	icc := append([]byte("ICC_PROFILE\x00\x01\x01"), bytes.Repeat([]byte("p3"), 200)...)
	orig := insertSegment(testJPEG(t, 1000, 500), 0xE2, icc)
	f, _, err := s.OpenThumb(put(t, s, orig), 320)
	if err != nil {
		t.Fatal(err)
	}
	b := readAll(t, f)
	if !bytes.Contains(b, icc) {
		t.Errorf("the colour profile was dropped")
	}
	if got := decode(t, b); got.Bounds().Dx() != 320 {
		t.Errorf("width %d, want 320", got.Bounds().Dx())
	}
}

func TestThumbPassesThroughWhatItCantScale(t *testing.T) {
	s := openTest(t, t.TempDir())
	webp := riff("VP8L", []byte{0x2f, 0xff, 0x3f, 0x00, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	// A JPEG that says it is 2000 wide but whose image data is cut off.
	good := testJPEG(t, 2000, 1000)
	broken := good[:len(good)/3]
	for name, b := range map[string][]byte{"webp": webp, "truncated jpeg": broken} {
		m, err := s.PutImage(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f, meta, err := s.OpenThumb(m.Hash, 640)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := readAll(t, f); !bytes.Equal(got, b) || meta.ContentType != m.ContentType {
			t.Errorf("%s: not the original (%s)", name, meta.ContentType)
		}
	}
}

func TestThumbNotFound(t *testing.T) {
	s := openTest(t, t.TempDir())
	for _, h := range []string{"", "00", "../../sous.db", "0000000000000000000000000000000000000000000000000000000000000000"} {
		if _, _, err := s.OpenThumb(h, 640); !errors.Is(err, ErrImageNotFound) {
			t.Errorf("OpenThumb(%q) err = %v", h, err)
		}
	}
	if _, _, err := s.OpenThumb(put(t, s, testJPEG(t, 10, 10)), 0); err == nil {
		t.Errorf("width 0 accepted")
	}
}

// Many asks at once for one thumbnail all get it, and it is made once.
func TestThumbConcurrent(t *testing.T) {
	s := openTest(t, t.TempDir())
	h := put(t, s, testJPEG(t, 1600, 1200))
	thumbsMade.Store(0)
	var wg sync.WaitGroup
	results := make([][]byte, 16)
	errs := make([]error, 16)
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, _, err := s.OpenThumb(h, 320)
			if err != nil {
				errs[i] = err
				return
			}
			defer f.Close()
			results[i], errs[i] = io.ReadAll(f)
		}()
	}
	wg.Wait()
	for i := range 16 {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if !bytes.Equal(results[i], results[0]) {
			t.Errorf("ask %d got different bytes", i)
		}
	}
	if decode(t, results[0]).Bounds().Dx() != 320 {
		t.Errorf("not scaled")
	}
	if n := thumbsMade.Load(); n != 1 {
		t.Errorf("made %d times, want once", n)
	}
	es, err := os.ReadDir(filepath.Join(s.Dir, "thumbs", h[:2]))
	if err != nil || len(es) != 1 {
		t.Errorf("thumbs/%s holds %d files (%v), want 1", h[:2], len(es), err)
	}
}

func TestEXIFOrientationParsing(t *testing.T) {
	jpg := testJPEG(t, 8, 8)
	cases := map[string]struct {
		b    []byte
		want int
	}{
		"none":         {jpg, 1},
		"6 II":         {insertSegment(jpg, 0xE1, exifWithOrientation(6, binary.LittleEndian)), 6},
		"8 MM":         {insertSegment(jpg, 0xE1, exifWithOrientation(8, binary.BigEndian)), 8},
		"out of range": {insertSegment(jpg, 0xE1, exifWithOrientation(9, binary.LittleEndian)), 1},
		"short exif":   {insertSegment(jpg, 0xE1, []byte("Exif\x00\x00II*\x00")), 1},
		"bad offset":   {insertSegment(jpg, 0xE1, append([]byte("Exif\x00\x00II*\x00"), 0xff, 0xff, 0xff, 0x7f)), 1},
		"xmp app1":     {insertSegment(jpg, 0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x/>")), 1},
		"not a jpeg":   {[]byte("nope"), 1},
		"cut segment":  {append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x40, 0x00}, "Exif"...), 1},
	}
	for name, c := range cases {
		if got := jpegOrientation(c.b); got != c.want {
			t.Errorf("%s: orientation %d, want %d", name, got, c.want)
		}
	}
}
