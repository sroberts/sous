package config

import "testing"

func TestLoadUpgrade(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	u, err := LoadUpgrade(env())
	if err != nil || u.Outpost() || u.OllamaURL != DefaultOllamaURL || u.GraphModel != "" || u.DecisionModel != "laya" {
		t.Errorf("standalone: %+v %v", u, err)
	}
	u, err = LoadUpgrade(env("OUTPOST_AI_URL", "http://127.0.0.1:47901", "OUTPOST_AI_TOKEN", "oai_x"))
	if err != nil || !u.Outpost() || !u.On() || u.OllamaURL != "" || u.DecisionModel != "system1" {
		t.Errorf("under Outpost: %+v %v", u, err)
	}
	// Pointed at an Ollama itself, Sous uses it, Outpost or not.
	u, err = LoadUpgrade(env("OUTPOST_AI_URL", "http://127.0.0.1:47901", "OUTPOST_AI_TOKEN", "oai_x", "SOUS_OLLAMA_URL", "http://10.0.0.2:11434"))
	if err != nil || u.Outpost() || u.OllamaURL != "http://10.0.0.2:11434" {
		t.Errorf("Ollama wins: %+v %v", u, err)
	}
	// Outpost without a token (an older Outpost): not Outpost's models.
	if u, _ := LoadUpgrade(env("OUTPOST_AI_URL", "http://127.0.0.1:47901")); u.Outpost() {
		t.Errorf("no token: %+v", u)
	}
	if _, err := LoadUpgrade(env("OUTPOST_AI_URL", "http://x", "OUTPOST_AI_TOKEN", "t", "SOUS_AUTO_VERIFY", "maybe")); err == nil {
		t.Error("bad auto-verify under Outpost accepted")
	}
	if u, _ := LoadUpgrade(env("SOUS_OLLAMA_URL", "off")); u.On() {
		t.Errorf("off: %+v", u)
	}
	if _, err := LoadUpgrade(env("SOUS_OLLAMA_URL", "ftp://x")); err == nil {
		t.Error("bad URL accepted")
	}
}
