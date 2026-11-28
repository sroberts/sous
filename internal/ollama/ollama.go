// Package ollama talks to a local Ollama server: a generative model for
// structured JSON, and a decision model (Laya) for typed questions answered
// with a probability per option. It implements recipes.Generator and
// recipes.Decider.
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

// Client is one Ollama server and the two models Sous uses on it.
type Client struct {
	BaseURL       string // e.g. http://127.0.0.1:11434
	GraphModel    string // writes step graphs
	DecisionModel string // answers typed questions
	HTTP          *http.Client
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
		if resp.StatusCode == http.StatusNotFound && strings.Contains(e.Error, "not found") {
			return fmt.Errorf("%w: %s", ErrUnreachable, e.Error)
		}
		return fmt.Errorf("ollama %s: %s: %s", path, resp.Status, e.Error)
	}
	return json.Unmarshal(data, out)
}

// Generate asks the graph model for JSON matching schema.
func (c *Client) Generate(ctx context.Context, prompt string, schema any) ([]byte, error) {
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
	if err := c.post(ctx, "/v1/systemone", body, &out); err != nil {
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

// Models reports what's installed, to say clearly when a model is missing.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, c.failed(err)
	}
	defer resp.Body.Close()
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, len(out.Models))
	for i, m := range out.Models {
		names[i] = m.Name
	}
	return names, nil
}
