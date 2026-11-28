package config

import (
	"cmp"
	"fmt"
	"net/url"
	"strings"
)

// Upgrade is how Sous reaches the local models that turn recipes into step
// graphs: an Ollama server with a generative model, which writes a graph's
// structure, and a decision model, which classifies each action and says
// how sure it is.
type Upgrade struct {
	OllamaURL     string // SOUS_OLLAMA_URL, default http://127.0.0.1:11434; empty when "off"
	GraphModel    string // SOUS_GRAPH_MODEL; empty: no local graph model, assistants draft structure
	DecisionModel string // SOUS_DECISION_MODEL
	AutoVerify    bool   // SOUS_AUTO_VERIFY=true: an upgrade sure of every label saves it as verified
}

// Defaults for step graph upgrades.
const (
	DefaultOllamaURL     = "http://127.0.0.1:11434"
	DefaultDecisionModel = "laya"
)

// On reports whether upgrades are configured at all.
func (u Upgrade) On() bool { return u.OllamaURL != "" }

// LoadUpgrade reads the upgrade settings. Ollama being unreachable isn't a
// configuration error: Sous starts without it and says so when asked to
// upgrade.
func LoadUpgrade(getenv func(string) string) (Upgrade, error) {
	var u Upgrade
	switch v := strings.TrimSpace(getenv("SOUS_OLLAMA_URL")); {
	case v == "":
		u.OllamaURL = DefaultOllamaURL
	case strings.EqualFold(v, "off"):
	default:
		p, err := url.Parse(v)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return u, fmt.Errorf("SOUS_OLLAMA_URL: %q is not an http(s) URL or \"off\"", v)
		}
		u.OllamaURL = strings.TrimRight(v, "/")
	}
	// No graph model by default: the small local models tried didn't write
	// sound structure, so an assistant drafts it and Sous classifies.
	u.GraphModel = strings.TrimSpace(getenv("SOUS_GRAPH_MODEL"))
	u.DecisionModel = cmp.Or(strings.TrimSpace(getenv("SOUS_DECISION_MODEL")), DefaultDecisionModel)
	switch v := getenv("SOUS_AUTO_VERIFY"); v {
	case "", "false":
	case "true":
		u.AutoVerify = true
	default:
		return u, fmt.Errorf("SOUS_AUTO_VERIFY: %q is not \"true\" or \"false\"", v)
	}
	return u, nil
}
