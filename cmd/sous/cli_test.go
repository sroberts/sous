package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestHelpAndVersion: both answer on stdout and exit 0 without needing a
// port or storage, so someone trying Sous for the first time gets help, not
// a configuration error.
func TestHelpAndVersion(t *testing.T) {
	none := func(string) string { return "" }
	for _, arg := range []string{"--help", "-h"} {
		var out bytes.Buffer
		if code := run([]string{arg}, none, &out); code != 0 {
			t.Errorf("%s: exit %d", arg, code)
		}
		for _, want := range []string{"--port", "--data", "OUTPOST_PORT", "OUTPOST_STORAGE_DIR"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: output lacks %q:\n%s", arg, want, out.String())
			}
		}
	}
	var out bytes.Buffer
	if code := run([]string{"--version"}, none, &out); code != 0 || out.String() != "sous "+version+"\n" {
		t.Errorf("--version: exit %d, %q", code, out.String())
	}
}
