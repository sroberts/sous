// Package samples holds the recipes a new library starts with: historic
// recipes from public-domain cookbooks (SOURCES.md says where each came
// from), added by Sous itself so a first look at the app isn't empty.
//
// Each sample is data/NN-slug.json, a strict RecipeInput with no images,
// and optionally data/NN-slug.jpg or .png, its one image. NN sets the order.
package samples

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"strings"

	"github.com/sroberts/sous/internal/recipes"
)

//go:embed data
var data embed.FS

// All returns the embedded samples in file name order.
func All() ([]recipes.Sample, error) {
	return load(data, "data")
}

func load(fsys fs.FS, dir string) ([]recipes.Sample, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	var out []recipes.Sample
	images := 0
	for _, e := range entries { // ReadDir sorts by name
		base, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			switch path.Ext(e.Name()) {
			case ".jpg", ".png":
				images++
				continue
			}
			return nil, fmt.Errorf("samples: unexpected file %s", e.Name())
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var sm recipes.Sample
		if err := decodeStrict(b, &sm.Recipe); err != nil {
			return nil, fmt.Errorf("samples: %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(sm.Recipe.Title) == "" || len(sm.Recipe.Images) > 0 {
			return nil, fmt.Errorf("samples: %s: needs a title and no images", e.Name())
		}
		for _, ext := range []string{".jpg", ".png"} {
			if !names[base+ext] {
				continue
			}
			if sm.Image != nil {
				return nil, fmt.Errorf("samples: %s has both a .jpg and a .png", base)
			}
			if sm.Image, err = fs.ReadFile(fsys, path.Join(dir, base+ext)); err != nil {
				return nil, err
			}
		}
		if sm.Image != nil {
			images--
		}
		out = append(out, sm)
	}
	if images != 0 {
		return nil, fmt.Errorf("samples: %d image(s) without a recipe of the same name", images)
	}
	return out, nil
}

// decodeStrict is the API's rule for recipe bodies: unknown fields and
// trailing data are errors, so a typo in a sample can't be lost silently.
func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after the recipe")
	}
	return nil
}

// Seed adds the samples to svc's library if it has never been used, logging
// "samples added" with the count when it does.
func Seed(ctx context.Context, svc *recipes.Service, log *slog.Logger) error {
	all, err := All()
	if err != nil {
		return err
	}
	n, err := svc.SeedSamples(ctx, all)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Info("samples added", "count", n)
	}
	return nil
}
