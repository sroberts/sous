package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSamples(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "data")
	for _, tt := range []struct {
		value   string
		want    bool // NoSamples
		wantErr bool
	}{
		{value: "", want: false},
		{value: "true", want: false},
		{value: "false", want: true},
		{value: "no", wantErr: true},
		{value: "1", wantErr: true},
		{value: "TRUE", wantErr: true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			env := map[string]string{"OUTPOST_PORT": "8095", "OUTPOST_STORAGE_DIR": storage, "SOUS_SAMPLES": tt.value}
			got, err := Load(nil, func(k string) string { return env[k] })
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "SOUS_SAMPLES") {
					t.Fatalf("err = %v, want one naming SOUS_SAMPLES", err)
				}
				return
			}
			if err != nil || got.NoSamples != tt.want {
				t.Fatalf("NoSamples = %v, %v, want %v", got.NoSamples, err, tt.want)
			}
		})
	}
}
