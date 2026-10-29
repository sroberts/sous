package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	base := t.TempDir()
	storage := filepath.Join(base, "data")

	gateway := map[string]string{
		"OUTPOST_AUTH":     "gateway",
		"OUTPOST_ISSUER":   "https://outpost.example",
		"OUTPOST_JWKS_URL": "http://127.0.0.1:7000/.well-known/jwks.json",
	}
	with := func(m map[string]string, kv ...string) map[string]string {
		out := map[string]string{"OUTPOST_PORT": "8095", "OUTPOST_STORAGE_DIR": storage}
		for k, v := range m {
			out[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i]] = kv[i+1]
		}
		return out
	}

	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{name: "env port", env: with(nil),
			want: Config{Port: 8095, StorageDir: storage, LogLevel: slog.LevelInfo}},
		{name: "flag wins", args: []string{"--port", "9000"}, env: with(nil),
			want: Config{Port: 9000, StorageDir: storage, LogLevel: slog.LevelInfo}},
		{name: "flag equals form", args: []string{"--port=9001"}, env: with(nil, "OUTPOST_PORT", ""),
			want: Config{Port: 9001, StorageDir: storage, LogLevel: slog.LevelInfo}},
		{name: "no port", env: with(nil, "OUTPOST_PORT", ""), wantErr: "port is required"},
		{name: "bad env port", env: with(nil, "OUTPOST_PORT", "http"), wantErr: "OUTPOST_PORT"},
		{name: "zero port", env: with(nil, "OUTPOST_PORT", "0"), wantErr: "OUTPOST_PORT"},
		{name: "port too big", env: with(nil, "OUTPOST_PORT", "65536"), wantErr: "OUTPOST_PORT"},
		{name: "bad flag port", args: []string{"--port", "x"}, env: with(nil), wantErr: "--port"},
		{name: "positional", args: []string{"serve"}, env: with(nil), wantErr: "unexpected argument"},
		{name: "unknown flag", args: []string{"--verbose"}, env: with(nil), wantErr: "flags"},
		{name: "no storage", env: with(nil, "OUTPOST_STORAGE_DIR", ""), wantErr: "OUTPOST_STORAGE_DIR is required"},
		{name: "storage is a file", env: with(nil, "OUTPOST_STORAGE_DIR", filepath.Join(base, "file")),
			wantErr: "OUTPOST_STORAGE_DIR"},
		{name: "gateway", env: with(gateway),
			want: Config{Port: 8095, StorageDir: storage, Auth: "gateway", Issuer: gateway["OUTPOST_ISSUER"],
				JWKSURL: gateway["OUTPOST_JWKS_URL"], LogLevel: slog.LevelInfo}},
		{name: "gateway required", env: with(gateway, "SOUS_REQUIRE_GATEWAY", "true"),
			want: Config{Port: 8095, StorageDir: storage, Auth: "gateway", Issuer: gateway["OUTPOST_ISSUER"],
				JWKSURL: gateway["OUTPOST_JWKS_URL"], RequireGateway: true, LogLevel: slog.LevelInfo}},
		{name: "unknown auth", env: with(nil, "OUTPOST_AUTH", "basic"), wantErr: "OUTPOST_AUTH"},
		{name: "gateway no issuer", env: with(gateway, "OUTPOST_ISSUER", ""), wantErr: "OUTPOST_ISSUER"},
		{name: "gateway no jwks", env: with(gateway, "OUTPOST_JWKS_URL", ""), wantErr: "OUTPOST_JWKS_URL"},
		{name: "gateway jwks not http", env: with(gateway, "OUTPOST_JWKS_URL", "file:///etc/jwks.json"),
			wantErr: "OUTPOST_JWKS_URL"},
		{name: "gateway jwks no host", env: with(gateway, "OUTPOST_JWKS_URL", "https://"), wantErr: "OUTPOST_JWKS_URL"},
		{name: "issuer ignored without gateway", env: with(nil, "OUTPOST_ISSUER", "x", "OUTPOST_JWKS_URL", "bogus"),
			want: Config{Port: 8095, StorageDir: storage, LogLevel: slog.LevelInfo}},
		{name: "require gateway without gateway", env: with(nil, "SOUS_REQUIRE_GATEWAY", "true"),
			wantErr: "SOUS_REQUIRE_GATEWAY"},
		{name: "require gateway false", env: with(nil, "SOUS_REQUIRE_GATEWAY", "false"),
			want: Config{Port: 8095, StorageDir: storage, LogLevel: slog.LevelInfo}},
		{name: "require gateway not strict", env: with(gateway, "SOUS_REQUIRE_GATEWAY", "1"),
			wantErr: "SOUS_REQUIRE_GATEWAY"},
		{name: "require gateway yes", env: with(gateway, "SOUS_REQUIRE_GATEWAY", "yes"),
			wantErr: "SOUS_REQUIRE_GATEWAY"},
		{name: "log debug", env: with(nil, "SOUS_LOG_LEVEL", "debug"),
			want: Config{Port: 8095, StorageDir: storage, LogLevel: slog.LevelDebug}},
		{name: "log warn", env: with(nil, "SOUS_LOG_LEVEL", "warn"),
			want: Config{Port: 8095, StorageDir: storage, LogLevel: slog.LevelWarn}},
		{name: "log error", env: with(nil, "SOUS_LOG_LEVEL", "error"),
			want: Config{Port: 8095, StorageDir: storage, LogLevel: slog.LevelError}},
		{name: "log bad", env: with(nil, "SOUS_LOG_LEVEL", "trace"), wantErr: "SOUS_LOG_LEVEL"},
	}

	if err := os.WriteFile(filepath.Join(base, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(tt.args, func(k string) string { return tt.env[k] })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestLoadStorageDir(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg, err := Load(nil, func(k string) string {
		return map[string]string{"OUTPOST_PORT": "8095", "OUTPOST_STORAGE_DIR": "rel/data"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.StorageDir) || !strings.HasSuffix(cfg.StorageDir, filepath.Join("rel", "data")) {
		t.Fatalf("StorageDir = %q, want an absolute path ending in rel/data", cfg.StorageDir)
	}
	fi, err := os.Stat(cfg.StorageDir)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("storage dir mode = %v, want a 0700 directory", fi.Mode())
	}
}

func TestLoadErrorCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	_, err := Load(nil, func(k string) string {
		return map[string]string{"OUTPOST_PORT": "8095", "OUTPOST_STORAGE_DIR": dir, "OUTPOST_AUTH": "nope"}[k]
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("storage dir exists after a config error: %v", err)
	}
}
