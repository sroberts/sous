package api

import (
	"bytes"
	"image"
	"image/jpeg"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestImageWidths(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 2000, 1000)), nil); err != nil {
		t.Fatal(err)
	}
	orig := buf.Bytes()
	r := h.do("POST", "/v1/images", orig, append(scott, "Content-Type", "image/jpeg")...)
	h.want(r, 201, "")
	hash := r.json(t)["hash"].(string)

	for _, w := range []int{320, 640, 1280} {
		r := h.do("GET", "/v1/images/"+hash+"?w="+strconv.Itoa(w), nil)
		h.want(r, 200, "")
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(r.body))
		if err != nil {
			t.Fatalf("w=%d: %v", w, err)
		}
		if cfg.Width != w || cfg.Height != w/2 {
			t.Errorf("w=%d: %dx%d", w, cfg.Width, cfg.Height)
		}
		if ct := r.header.Get("Content-Type"); ct != "image/jpeg" {
			t.Errorf("w=%d: Content-Type %q", w, ct)
		}
		if cc := r.header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Errorf("w=%d: Cache-Control %q", w, cc)
		}
		if et := r.header.Get("ETag"); et != `"`+hash+"-"+strconv.Itoa(w)+`"` {
			t.Errorf("w=%d: ETag %q", w, et)
		}
		// A revalidation with the same tag is answered 304.
		r2 := h.do("GET", "/v1/images/"+hash+"?w="+strconv.Itoa(w), nil, "If-None-Match", r.header.Get("ETag"))
		if r2.status != http.StatusNotModified {
			t.Errorf("w=%d: If-None-Match gave %d", w, r2.status)
		}
	}

	// Without w, the original, tagged as before.
	r = h.do("GET", "/v1/images/"+hash, nil)
	h.want(r, 200, "")
	if !bytes.Equal(r.body, orig) || r.header.Get("ETag") != `"`+hash+`"` {
		t.Errorf("the original changed: %d bytes, ETag %q", len(r.body), r.header.Get("ETag"))
	}
	r = h.do("GET", "/v1/images/"+hash+"?w=", nil)
	h.want(r, 200, "")
	if !bytes.Equal(r.body, orig) {
		t.Errorf("an empty w isn't the original")
	}

	// w=0 is the same as no w: the original.
	r = h.do("GET", "/v1/images/"+hash+"?w=0", nil)
	h.want(r, 200, "")
	if !bytes.Equal(r.body, orig) {
		t.Errorf("w=0 isn't the original")
	}
	for _, bad := range []string{"100", "-640", "abc", "640&w=320", "99999", "640.5"} {
		h.want(h.do("GET", "/v1/images/"+hash+"?w="+bad, nil), 400, "invalid")
	}
	h.want(h.do("GET", "/v1/images/"+strings.Repeat("0", 64)+"?w=640", nil), 404, "not_found")
	h.want(h.do("GET", "/v1/images/..%2f..%2fsous.db?w=640", nil), 404, "not_found")
}

// A photo smaller than the width asked for comes as it is.
func TestImageWidthSmallPhoto(t *testing.T) {
	h := newHarness(t)
	scott := h.person("Scott")
	img := pngBytes(t)
	r := h.do("POST", "/v1/images", img, append(scott, "Content-Type", "image/png")...)
	h.want(r, 201, "")
	hash := r.json(t)["hash"].(string)
	r = h.do("GET", "/v1/images/"+hash+"?w=320", nil)
	h.want(r, 200, "")
	if !bytes.Equal(r.body, img) || r.header.Get("Content-Type") != "image/png" {
		t.Errorf("not the original: %d bytes, %s", len(r.body), r.header.Get("Content-Type"))
	}
}
