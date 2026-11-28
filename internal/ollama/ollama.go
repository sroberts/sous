// Package ollama reaches the models step graphs use: a generative model
// for structured JSON, and a decision model (Laya) for typed questions
// answered with a probability per option. It talks to Outpost's AI
// endpoint when Sous runs under Outpost (the operator's "llm" and
// "system1", Outpost holding any provider's key), and to a local Ollama
// otherwise. It implements recipes.Generator and recipes.Decider.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sroberts/sous/internal/recipes"
)

// Client is where the two models are and what they're called.
type Client struct {
	BaseURL       string // Ollama's, e.g. http://127.0.0.1:11434, or Outpost's AI endpoint
	GraphModel    string // writes step graphs ("llm" under Outpost)
	DecisionModel string // answers typed questions ("system1" under Outpost)
	// Token is Outpost's AI token; set, the client speaks Outpost's API
	// (OpenAI chat completions and System 1, under their prefixes).
	Token string
	HTTP  *http.Client
}

// NewOutpost returns a client for Outpost's AI endpoint. Outpost picks
// the models behind "llm" and "system1"; a first request may take minutes
// while Ollama loads one.
func NewOutpost(url, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(url, "/"), GraphModel: "llm", DecisionModel: "system1", Token: token,
		HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

// Granted asks Outpost which models this app may use, as its model list
// says. Unreachable, it assumes both, so requests say what's wrong then.
func (c *Client) Granted(ctx context.Context) (llm, system1 bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/openai/v1/models", nil)
	if err != nil {
		return true, true
	}
	c.auth(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return true, true
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil {
		return true, true
	}
	for _, m := range out.Data {
		llm = llm || m.ID == "llm"
		system1 = system1 || m.ID == "system1"
	}
	return llm, system1
}

// auth adds Outpost's token. It's sent only to BaseURL, never logged.
func (c *Client) auth(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

// New returns a client with timeouts suited to local models: a graph can
// take a few minutes on a laptop, a decision under a second.
func New(baseURL, graphModel, decisionModel string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), GraphModel: graphModel, DecisionModel: decisionModel,
		HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

// ErrUnreachable wraps failures to reach Ollama, or a model it doesn't
// have, so callers can say so plainly and try again later rather than
// count it against the recipe.
var ErrUnreachable = recipes.ErrModelsUnavailable

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return c.failed(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(data))
		}
		// A missing model, one that doesn't fit in memory just now, or
		// (under Outpost) a model not given to Sous or not available, is a
		// reason to wait, not the recipe's fault.
		if (resp.StatusCode == http.StatusNotFound && strings.Contains(e.Error, "not found")) ||
			strings.Contains(e.Error, "requires") && strings.Contains(e.Error, "available") ||
			c.Token != "" && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadGateway ||
				resp.StatusCode == http.StatusServiceUnavailable) {
			return fmt.Errorf("%w: %s", ErrUnreachable, e.Error)
		}
		return fmt.Errorf("ollama %s: %s: %s", path, resp.Status, e.Error)
	}
	return json.Unmarshal(data, out)
}

// Generate asks the graph model for JSON matching schema.
func (c *Client) Generate(ctx context.Context, prompt string, schema any) ([]byte, error) {
	if c.Token != "" {
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		err := c.post(ctx, "/openai/v1/chat/completions", map[string]any{
			"model": c.GraphModel, "temperature": 0, "max_tokens": 16000,
			"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "step_graph", "schema": schema}},
			"messages":        []map[string]string{{"role": "user", "content": prompt + "\n\nAnswer with the JSON only."}},
		}, &out)
		if err != nil {
			return nil, err
		}
		if len(out.Choices) == 0 {
			return nil, fmt.Errorf("the graph model gave no answer")
		}
		return []byte(out.Choices[0].Message.Content), nil
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	err := c.post(ctx, "/api/chat", map[string]any{
		"model": c.GraphModel, "stream": false, "format": schema, "think": false,
		"options":  map[string]any{"temperature": 0, "num_ctx": 8192},
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	}, &out)
	if err != nil {
		return nil, err
	}
	return []byte(out.Message.Content), nil
}

// Decide asks the decision model every question about one state in one
// call. Each answer is the probability of each choice, by choice label.
func (c *Client) Decide(ctx context.Context, state string, qs []recipes.Question) (map[string]recipes.Answer, error) {
	type question struct {
		Type         string            `json:"type"`
		Instructions string            `json:"instructions"`
		Criteria     map[string]string `json:"criteria"`
	}
	body := struct {
		Model     string              `json:"model"`
		State     string              `json:"state"`
		Questions map[string]question `json:"questions"`
	}{Model: c.DecisionModel, State: state, Questions: map[string]question{}}
	for _, q := range qs {
		crit := map[string]string{}
		for _, ch := range q.Choices {
			crit[ch.Label] = ch.Means
		}
		body.Questions[q.ID] = question{Type: "choice", Instructions: q.Ask, Criteria: crit}
	}
	var out struct {
		Answers map[string]struct {
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"answers"`
	}
	path := "/v1/systemone"
	if c.Token != "" {
		path = "/systemone/v1/systemone"
	}
	if err := c.post(ctx, path, body, &out); err != nil {
		return nil, err
	}
	res := map[string]recipes.Answer{}
	for id, a := range out.Answers {
		res[id] = recipes.Answer{Probabilities: a.Probabilities}
	}
	for _, q := range qs {
		if _, ok := res[q.ID]; !ok {
			return nil, fmt.Errorf("ollama: no answer to %s", q.ID)
		}
	}
	return res, nil
}

// failed sorts a request that got no answer: no connection at all is
// ErrUnreachable (try again later), but a model that's too slow is the
// recipe's failure, so a background job moves on rather than retrying it
// forever.
func (c *Client) failed(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return fmt.Errorf("%w at %s: %v", ErrUnreachable, c.BaseURL, err)
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return fmt.Errorf("the model didn't answer within %s", c.HTTP.Timeout)
	}
	return fmt.Errorf("ollama: %w", err)
}
