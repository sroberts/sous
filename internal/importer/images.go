package importer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/recipes"
	"github.com/sroberts/sous/internal/safefetch"
)

// maxImageTries is how many photos are downloaded before giving up.
const maxImageTries = 3

// maxGIFPixels bounds the frame decoded from a GIF, so a tiny file can't
// claim a huge canvas.
const maxGIFPixels = 40 << 20

// storeImages downloads the best photos in rank order until one stores,
// and makes it the draft's cover. fetchCtx carries the import's budget;
// ctx, without it, is used for storing, so a photo that arrived in time is
// kept. Every candidate is returned, with Hash or Problem set on those
// tried.
func (im *Importer) storeImages(fetchCtx, ctx context.Context, c identity.Caller, cands []Candidate, referer string, draft *recipes.RecipeInput) []recipes.ImageCandidate {
	out := make([]recipes.ImageCandidate, len(cands))
	for i, cand := range cands {
		out[i] = recipes.ImageCandidate{URL: cand.URL, Width: cand.Width, Height: cand.Height}
	}
	for i := 0; i < len(out) && i < maxImageTries; i++ {
		if fetchCtx.Err() != nil {
			problem := "Sous ran out of time before fetching this photo."
			out[i].Problem = &problem
			break
		}
		hash, problem := im.storeImage(fetchCtx, ctx, c, out[i].URL, referer)
		if problem != "" {
			out[i].Problem = &problem
			continue
		}
		out[i].Hash = &hash
		draft.Images = []string{hash}
		break
	}
	return out
}

func (im *Importer) storeImage(fetchCtx, ctx context.Context, c identity.Caller, u, referer string) (hash, problem string) {
	resp, err := im.fetch.Get(fetchCtx, u, safefetch.KindImage, referer)
	if err != nil {
		var blocked *safefetch.BlockedError
		switch {
		case errors.As(err, &blocked):
			return "", MessageLocalAddress
		case errors.Is(err, safefetch.ErrTooLarge):
			return "", "The photo is larger than 25 MB."
		case errors.Is(err, safefetch.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
			return "", "The photo took too long to download."
		}
		return "", "The photo couldn't be downloaded."
	}
	if resp.Status < 200 || resp.Status > 299 {
		return "", fmt.Sprintf("The site answered %d for the photo.", resp.Status)
	}
	body := resp.Body
	switch format := sniffImage(body); format {
	case "gif":
		if body, err = convertGIF(body); err != nil {
			return "", "The GIF couldn't be read."
		}
	case "":
	default:
		return "", format + " photos aren't supported."
	}
	ref, err := im.svc.UploadImage(ctx, c, bytes.NewReader(body))
	if err != nil {
		var re *recipes.Error
		if errors.As(err, &re) && re.Code == recipes.CodeUnsupportedMediaType {
			return "", "The photo isn't JPEG, PNG, WebP or HEIC."
		}
		im.log.Warn("import: storing photo failed", "error", err.Error())
		return "", "The photo couldn't be stored."
	}
	return ref.Hash, ""
}

// sniffImage names the formats that need converting ("gif") or can't be
// stored. Anything else is left to UploadImage to judge.
func sniffImage(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "gif"
	case bytes.HasPrefix(b, []byte("II*\x00")), bytes.HasPrefix(b, []byte("MM\x00*")):
		return "TIFF"
	case bytes.HasPrefix(b, []byte("BM")):
		return "BMP"
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "avif" || string(b[8:12]) == "avis"):
		return "AVIF"
	}
	if bytes.Contains(bytes.ToLower(b[:min(len(b), 512)]), []byte("<svg")) {
		return "SVG"
	}
	return ""
}

// convertGIF re-encodes a GIF's first frame: PNG when it has transparent
// pixels, else JPEG at quality 90.
func convertGIF(b []byte) ([]byte, error) {
	cfg, err := gif.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxGIFPixels {
		return nil, errors.New("gif dimensions out of range")
	}
	img, err := gif.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if transparent(img) {
		err = png.Encode(&out, img)
	} else {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 90})
	}
	return out.Bytes(), err
}

func transparent(img image.Image) bool {
	if p, ok := img.(*image.Paletted); ok {
		return !p.Opaque()
	}
	r := img.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				return true
			}
		}
	}
	return false
}
