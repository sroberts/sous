package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDataFlag covers --data, for running Sous without Outpost. It is used
// when OUTPOST_STORAGE_DIR is unset; both naming different directories is an
// error, so Outpost's directory, which its backups cover, is never overridden.
func TestDataFlag(t *testing.T) {
	base := t.TempDir()
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	flagDir := filepath.Join(base, "flag")
	c, err := Load([]string{"--port", "8095", "--data", flagDir}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.StorageDir != flagDir {
		t.Errorf("StorageDir = %q; want the --data dir %q", c.StorageDir, flagDir)
	}
	if _, err := os.Stat(flagDir); err != nil {
		t.Errorf("--data dir not created: %v", err)
	}

	// The same directory both ways is fine.
	if _, err := Load([]string{"--port", "8095", "--data", flagDir}, env(map[string]string{"OUTPOST_STORAGE_DIR": flagDir})); err != nil {
		t.Errorf("same dir both ways: %v", err)
	}

	// Different directories: refused, and neither is created.
	envDir := filepath.Join(base, "env")
	other := filepath.Join(base, "other")
	_, err = Load([]string{"--port", "8095", "--data", other}, env(map[string]string{"OUTPOST_STORAGE_DIR": envDir}))
	if err == nil || !strings.Contains(err.Error(), "differ") {
		t.Errorf("err = %v; want the two directories refused", err)
	}
	for _, d := range []string{envDir, other} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("%s was created: %v", d, err)
		}
	}

	// A relative --data is made absolute.
	t.Chdir(base)
	c, err = Load([]string{"--port", "8095", "--data", "rel"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.Abs("rel"); c.StorageDir != want {
		t.Errorf("StorageDir = %q; want %q", c.StorageDir, want)
	}

	// Neither: the error names both ways to set it.
	_, err = Load([]string{"--port", "8095"}, env(nil))
	if err == nil || !strings.Contains(err.Error(), "--data") || !strings.Contains(err.Error(), "OUTPOST_STORAGE_DIR") {
		t.Errorf("err = %v; want one naming --data and OUTPOST_STORAGE_DIR", err)
	}
}
