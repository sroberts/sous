package config

import (
	"strings"
	"testing"
)

// OUTPOST_AI_URL must be a plain http(s) URL, and a bad one isn't repeated
// in the error.
func TestOutpostAIURLChecked(t *testing.T) {
	for _, v := range []string{"ftp://x", "127.0.0.1:47901", "http://user:pw@x", "http://x/?a=b", "http://x/#f", "http://"} {
		env := map[string]string{"OUTPOST_AI_URL": v, "OUTPOST_AI_TOKEN": "t"}
		_, err := LoadUpgrade(func(k string) string { return env[k] })
		if err == nil || strings.Contains(err.Error(), "pw") {
			t.Errorf("%s: %v", v, err)
		}
	}
	env := map[string]string{"OUTPOST_AI_URL": "https://outpost.local/ai/", "OUTPOST_AI_TOKEN": "t"}
	if u, err := LoadUpgrade(func(k string) string { return env[k] }); err != nil || u.OutpostAIURL != "https://outpost.local/ai" {
		t.Errorf("good URL: %+v %v", u, err)
	}
}
