package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
)

// thumbVersion is part of every thumbnail's file name. Change it when the
// way thumbnails are made changes, so old ones are made again rather than
// served. Old files are only ever a little disk; nothing reads them.
const thumbVersion = "v1"

// maxThumbSource is the largest original, in pixels, a thumbnail is made
// from. Decoding takes about 1.5 bytes a pixel for a JPEG and 4 for a PNG;
// anything larger is served as it is.
const maxThumbSource = 100_000_000

// thumbJPEGQuality balances size against the photo's texture at 2x.
const thumbJPEGQuality = 82

// One decode at a time: a 40-megapixel photo takes about 60 MB to decode,
// and a page full of new tickets asks for many at once.
var thumbSlot = make(chan struct{}, 1)

// Requests for the same thumbnail while it is being made wait for that one.
var (
	thumbMu       sync.Mutex
	thumbInFlight = map[string]*thumbCall{}
)

// thumbsMade counts thumbnails written, for the tests.
var thumbsMade atomic.Int64

type thumbCall struct {
	done chan struct{}
	err  error
}

// OpenThumb opens the image for hash scaled to width pixels wide, keeping
// its proportions and the orientation its EXIF data asks for. It is made
// the first time it is asked for and kept under thumbs/, in the storage
// directory, from then on: it is derived from the content-addressed
// original, so a restore never makes one wrong.
//
// The original itself is returned (with its own meta) when no thumbnail
// would help or can be made: it is already no wider than width, it isn't
// JPEG or PNG (WebP and HEIC are passed through), it is larger than
// maxThumbSource, or it doesn't decode. A missing original returns
// ErrImageNotFound. width must be positive.
func (s *Store) OpenThumb(hash string, width int) (*os.File, ImageMeta, error) {
	if width <= 0 {
		return nil, ImageMeta{}, fmt.Errorf("storage: thumbnail width %d", width)
	}
	if !validHash(hash) {
		return nil, ImageMeta{}, ErrImageNotFound
	}
	if f, meta, ok := s.openStoredThumb(hash, width); ok {
		return f, meta, nil
	}

	key := hash + "-" + strconv.Itoa(width)
	thumbMu.Lock()
	if c, ok := thumbInFlight[key]; ok {
		thumbMu.Unlock()
		<-c.done
	} else {
		c := &thumbCall{done: make(chan struct{})}
		thumbInFlight[key] = c
		thumbMu.Unlock()
		c.err = s.makeThumb(hash, width)
		thumbMu.Lock()
		delete(thumbInFlight, key)
		thumbMu.Unlock()
		close(c.done)
	}
	if f, meta, ok := s.openStoredThumb(hash, width); ok {
		return f, meta, nil
	}
	// No thumbnail was kept: the original is the answer.
	return s.OpenImage(hash)
}

func (s *Store) thumbPath(hash string, width int, ext string) string {
	return filepath.Join(s.Dir, "thumbs", hash[:2], fmt.Sprintf("%s-%d.%s.%s", hash, width, thumbVersion, ext))
}

func (s *Store) openStoredThumb(hash string, width int) (*os.File, ImageMeta, bool) {
	for _, e := range []struct{ ct, ext string }{{"image/jpeg", "jpg"}, {"image/png", "png"}} {
		f, err := os.Open(s.thumbPath(hash, width, e.ext))
		if err != nil {
			continue
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}
		return f, ImageMeta{Hash: hash, ContentType: e.ct, Bytes: info.Size()}, true
	}
	return nil, ImageMeta{}, false
}

// errNoThumb means the original should be served as it is.
var errNoThumb = errors.New("storage: no thumbnail for this image")

// makeThumb writes the thumbnail for hash at width, or returns errNoThumb
// (or another error) having written nothing.
func (s *Store) makeThumb(hash string, width int) error {
	f, meta, err := s.OpenImage(hash)
	if err != nil {
		return err
	}
	defer f.Close()
	if meta.ContentType != "image/jpeg" && meta.ContentType != "image/png" {
		return errNoThumb
	}
	src, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxThumbSource {
		return errNoThumb
	}
	orient := 1
	if meta.ContentType == "image/jpeg" {
		orient = jpegOrientation(src)
	}
	dispW, dispH := cfg.Width, cfg.Height
	if orient >= 5 {
		dispW, dispH = dispH, dispW
	}
	if dispW <= width {
		return errNoThumb // never enlarged: the original is already small enough
	}

	thumbSlot <- struct{}{}
	out, ext, err := renderThumb(src, meta.ContentType, orient, width, int(math.Round(float64(dispH)*float64(width)/float64(dispW))))
	<-thumbSlot
	if err != nil {
		return errNoThumb
	}
	return s.writeThumb(hash, width, ext, out)
}

// renderThumb decodes src, scales it so that once turned by orient it is
// w×h, turns it, and encodes it: JPEG (with the original's colour profile)
// unless it has transparency, then PNG.
func renderThumb(src []byte, contentType string, orient, w, h int) ([]byte, string, error) {
	if h < 1 {
		h = 1
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", err
	}
	// Scale in the original's orientation, then turn.
	sw, sh := w, h
	if orient >= 5 {
		sw, sh = h, w
	}
	var scaled image.Image
	if ycc, ok := img.(*image.YCbCr); ok {
		scaled = orientYCbCr(scaleYCbCr(ycc, sw, sh), orient)
	} else {
		b := img.Bounds()
		rgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		scaled = orientNRGBA(scaleNRGBA(rgba, sw, sh), orient)
	}

	var buf bytes.Buffer
	if n, ok := scaled.(*image.NRGBA); ok && !n.Opaque() {
		if err := png.Encode(&buf, n); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "png", nil
	}
	if err := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: thumbJPEGQuality}); err != nil {
		return nil, "", err
	}
	out := buf.Bytes()
	if contentType == "image/jpeg" {
		out = withICC(out, jpegICC(src))
	}
	return out, "jpg", nil
}

// writeThumb stages the bytes in tmp/ and renames them into thumbs/, so a
// crash never leaves half a thumbnail under its final name.
func (s *Store) writeThumb(hash string, width int, ext string, b []byte) error {
	final := s.thumbPath(hash, width, ext)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Join(s.Dir, "tmp"), "thumb-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	thumbsMade.Add(1)
	return nil
}

// --- scaling ---------------------------------------------------------------

// A box filter: each output pixel is the area-weighted average of the
// source pixels it covers. Exact for reductions, which is all this does.
type tap struct {
	i int
	w float32
}

func boxTaps(src, dst int) [][]tap {
	taps := make([][]tap, dst)
	scale := float64(src) / float64(dst)
	for o := range dst {
		lo, hi := float64(o)*scale, float64(o+1)*scale
		var ts []tap
		var sum float64
		for i := int(lo); i < src && float64(i) < hi; i++ {
			cover := math.Min(hi, float64(i+1)) - math.Max(lo, float64(i))
			if cover <= 0 {
				continue
			}
			ts = append(ts, tap{i, float32(cover)})
			sum += cover
		}
		if len(ts) == 0 { // dst wider than src (a chroma plane): nearest
			ts = []tap{{min(int(lo), src-1), 1}}
			sum = 1
		}
		for k := range ts {
			ts[k].w /= float32(sum)
		}
		taps[o] = ts
	}
	return taps
}

// scalePlane scales a w×h plane of 8-bit samples (stride bytes a row) to
// dw×dh, horizontally then vertically.
func scalePlane(pix []uint8, stride, w, h, dw, dh int) []uint8 {
	xt, yt := boxTaps(w, dw), boxTaps(h, dh)
	mid := make([]float32, dw*h)
	for y := range h {
		row := pix[y*stride : y*stride+w]
		for x, ts := range xt {
			var v float32
			for _, t := range ts {
				v += float32(row[t.i]) * t.w
			}
			mid[y*dw+x] = v
		}
	}
	out := make([]uint8, dw*dh)
	for y, ts := range yt {
		for x := range dw {
			var v float32
			for _, t := range ts {
				v += mid[t.i*dw+x] * t.w
			}
			out[y*dw+x] = clamp8(v)
		}
	}
	return out
}

func clamp8(v float32) uint8 {
	v += 0.5
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// scaleYCbCr scales each plane at its own resolution into a 4:4:4 image.
func scaleYCbCr(src *image.YCbCr, dw, dh int) *image.YCbCr {
	b := src.Rect
	w, h := b.Dx(), b.Dy()
	out := image.NewYCbCr(image.Rect(0, 0, dw, dh), image.YCbCrSubsampleRatio444)
	yOff := src.YOffset(b.Min.X, b.Min.Y)
	out.Y = scalePlane(src.Y[yOff:], src.YStride, w, h, dw, dh)
	cw, ch := chromaSize(src.SubsampleRatio, w, h)
	cOff := src.COffset(b.Min.X, b.Min.Y)
	out.Cb = scalePlane(src.Cb[cOff:], src.CStride, cw, ch, dw, dh)
	out.Cr = scalePlane(src.Cr[cOff:], src.CStride, cw, ch, dw, dh)
	return out
}

func chromaSize(r image.YCbCrSubsampleRatio, w, h int) (int, int) {
	switch r {
	case image.YCbCrSubsampleRatio422:
		return (w + 1) / 2, h
	case image.YCbCrSubsampleRatio420:
		return (w + 1) / 2, (h + 1) / 2
	case image.YCbCrSubsampleRatio440:
		return w, (h + 1) / 2
	case image.YCbCrSubsampleRatio411:
		return (w + 3) / 4, h
	case image.YCbCrSubsampleRatio410:
		return (w + 3) / 4, (h + 1) / 2
	}
	return w, h
}

// scaleNRGBA averages in premultiplied alpha, so a transparent pixel's
// colour doesn't bleed into its neighbours, then un-premultiplies.
func scaleNRGBA(src *image.NRGBA, dw, dh int) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	n := w * h
	planes := [4][]uint8{make([]uint8, n), make([]uint8, n), make([]uint8, n), make([]uint8, n)}
	opaque := true
	for i := range n {
		p := src.Pix[i*4 : i*4+4]
		a := uint32(p[3])
		if a != 255 {
			opaque = false
		}
		planes[0][i] = uint8(uint32(p[0]) * a / 255)
		planes[1][i] = uint8(uint32(p[1]) * a / 255)
		planes[2][i] = uint8(uint32(p[2]) * a / 255)
		planes[3][i] = p[3]
	}
	var s [4][]uint8
	for c := range 4 {
		s[c] = scalePlane(planes[c], w, w, h, dw, dh)
	}
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for i := range dw * dh {
		a := s[3][i]
		if opaque {
			a = 255
		}
		q := out.Pix[i*4 : i*4+4]
		q[3] = a
		if a == 0 {
			continue
		}
		for c := range 3 {
			q[c] = uint8(min(255, (uint32(s[c][i])*255+uint32(a)/2)/uint32(a)))
		}
	}
	return out
}

// --- orientation -------------------------------------------------------------

// orientMap gives, for EXIF orientation o and an output pixel (x, y) of a
// w×h result, the source pixel in the stored (unturned) sw×sh image.
func orientMap(o, x, y, sw, sh int) (int, int) {
	switch o {
	case 2: // mirrored
		return sw - 1 - x, y
	case 3: // upside down
		return sw - 1 - x, sh - 1 - y
	case 4: // upside down, mirrored
		return x, sh - 1 - y
	case 5: // transposed
		return y, x
	case 6: // turned 90° clockwise to display
		return y, sh - 1 - x
	case 7: // transversed
		return sw - 1 - y, sh - 1 - x
	case 8: // turned 90° anticlockwise to display
		return sw - 1 - y, x
	}
	return x, y
}

func orientDims(o, sw, sh int) (int, int) {
	if o >= 5 && o <= 8 {
		return sh, sw
	}
	return sw, sh
}

func orientPlane(p []uint8, sw, sh, o int) []uint8 {
	w, h := orientDims(o, sw, sh)
	out := make([]uint8, w*h)
	for y := range h {
		for x := range w {
			sx, sy := orientMap(o, x, y, sw, sh)
			out[y*w+x] = p[sy*sw+sx]
		}
	}
	return out
}

func orientYCbCr(src *image.YCbCr, o int) *image.YCbCr {
	if o < 2 || o > 8 {
		return src
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	w, h := orientDims(o, sw, sh)
	out := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio444)
	out.Y = orientPlane(src.Y, sw, sh, o)
	out.Cb = orientPlane(src.Cb, sw, sh, o)
	out.Cr = orientPlane(src.Cr, sw, sh, o)
	return out
}

func orientNRGBA(src *image.NRGBA, o int) *image.NRGBA {
	if o < 2 || o > 8 {
		return src
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	w, h := orientDims(o, sw, sh)
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			sx, sy := orientMap(o, x, y, sw, sh)
			copy(out.Pix[(y*w+x)*4:(y*w+x)*4+4], src.Pix[(sy*sw+sx)*4:(sy*sw+sx)*4+4])
		}
	}
	return out
}

// --- JPEG metadata -------------------------------------------------------------

// jpegSegments calls fn with each marker segment before the image data
// (marker byte and payload), stopping at the start of scan or a malformed
// segment.
func jpegSegments(b []byte, fn func(marker byte, payload []byte) bool) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return
		}
		m := b[i+1]
		if m == 0xFF { // fill byte
			i++
			continue
		}
		if m == 0xDA || m == 0xD9 { // start of scan, end of image
			return
		}
		n := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if n < 2 || i+2+n > len(b) {
			return
		}
		if !fn(m, b[i+4:i+2+n]) {
			return
		}
		i += 2 + n
	}
}

// jpegOrientation reads the EXIF orientation (1 to 8) from APP1, or 1.
func jpegOrientation(b []byte) int {
	o := 1
	jpegSegments(b, func(m byte, p []byte) bool {
		if m != 0xE1 || !bytes.HasPrefix(p, []byte("Exif\x00\x00")) {
			return true
		}
		if v, ok := exifOrientation(p[6:]); ok {
			o = v
		}
		return false
	})
	return o
}

func exifOrientation(t []byte) (int, bool) {
	if len(t) < 8 {
		return 0, false
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0, false
	}
	if bo.Uint16(t[2:4]) != 42 {
		return 0, false
	}
	ifd := int(bo.Uint32(t[4:8]))
	if ifd < 8 || ifd+2 > len(t) {
		return 0, false
	}
	n := int(bo.Uint16(t[ifd : ifd+2]))
	for k := range n {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 0, false
		}
		if bo.Uint16(t[e:e+2]) != 0x0112 {
			continue
		}
		if bo.Uint16(t[e+2:e+4]) != 3 { // SHORT
			return 0, false
		}
		v := int(bo.Uint16(t[e+8 : e+10]))
		if v < 1 || v > 8 {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// jpegICC returns the original's ICC profile segments (whole APP2
// segments, marker included), so a thumbnail of a Display P3 photo keeps
// its colours.
func jpegICC(b []byte) [][]byte {
	var segs [][]byte
	jpegSegments(b, func(m byte, p []byte) bool {
		if m == 0xE2 && bytes.HasPrefix(p, []byte("ICC_PROFILE\x00")) {
			seg := make([]byte, 0, len(p)+4)
			seg = append(seg, 0xFF, 0xE2, byte((len(p)+2)>>8), byte(len(p)+2))
			segs = append(segs, append(seg, p...))
		}
		return true
	})
	return segs
}

// withICC puts the profile segments straight after the encoded JPEG's SOI.
func withICC(jpg []byte, segs [][]byte) []byte {
	if len(segs) == 0 || len(jpg) < 2 {
		return jpg
	}
	n := len(jpg)
	for _, s := range segs {
		n += len(s)
	}
	out := make([]byte, 0, n)
	out = append(out, jpg[:2]...)
	for _, s := range segs {
		out = append(out, s...)
	}
	return append(out, jpg[2:]...)
}
