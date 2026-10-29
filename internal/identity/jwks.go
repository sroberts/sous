package identity

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Errors returned by (*JWKS).Key. The resolver maps ErrUnknownKey to 401
// invalid_identity and ErrJWKSUnavailable to 503 identity_unavailable.
var (
	ErrUnknownKey      = errors.New("identity: unknown key id")
	ErrJWKSUnavailable = errors.New("identity: JWKS unavailable")
)

const (
	fetchTimeout = 5 * time.Second
	maxJWKSBytes = 1 << 20
	// refetchEvery bounds refetches on an unknown kid, so a stream of forged
	// kids can't turn into a stream of requests to Outpost.
	refetchEvery = time.Minute
	// retryEvery bounds attempts while no fetch has ever succeeded.
	retryEvery = 5 * time.Second
)

// JWKS is an in-memory cache of the Outpost gateway's signing keys, fetched
// from OUTPOST_JWKS_URL.
type JWKS struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	keys        map[string]ed25519.PublicKey
	fetchedAt   time.Time // last success; zero if never
	attemptedAt time.Time // last attempt; zero if never
	lastErr     error     // from the last attempt
	inflight    *fetchCall
}

type fetchCall struct {
	done chan struct{}
}

// JWKSStatus reports the cache's health for GET /v1/status.
type JWKSStatus struct {
	OK        bool    `json:"ok" jsonschema:"whether the last JWKS fetch succeeded"`
	FetchedAt *string `json:"fetched_at" jsonschema:"time of the last successful fetch, UTC RFC 3339, or null if never"`
	Error     *string `json:"error" jsonschema:"why the last fetch failed, or null"`
}

// NewJWKS returns a cache for the JWKS at url. It fetches nothing until the
// first Key call. A nil client means a default client with a 5s timeout.
func NewJWKS(url string, client *http.Client) *JWKS {
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	return &JWKS{url: url, client: client, now: time.Now}
}

// Key returns the Ed25519 public key for kid.
//
// It fetches on first need. On an unknown kid it refetches at most once a
// minute (every 5s while no fetch has ever succeeded); concurrent callers
// share one in-flight fetch. A fetch has a 5s timeout and a 1 MiB limit,
// and keeps only OKP/Ed25519 keys whose x is 32 bytes. It returns
// ErrUnknownKey when the keys are known but kid is not among them, and an
// error wrapping ErrJWKSUnavailable when no usable key set can be fetched.
func (j *JWKS) Key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	waited := false
	j.mu.Lock()
	for {
		if k, ok := j.keys[kid]; ok {
			j.mu.Unlock()
			return k, nil
		}
		if j.inflight == nil {
			// One fetch per call is enough: if the key set we just waited
			// for lacks kid, fetching again won't find it.
			if waited || !j.mayFetch() {
				err := j.missErr()
				j.mu.Unlock()
				return nil, err
			}
			j.inflight = &fetchCall{done: make(chan struct{})}
			j.attemptedAt = j.now()
			// The fetch outlives this caller's context: other callers may
			// be waiting on it.
			go j.fetch(context.WithoutCancel(ctx), j.inflight)
		}
		c := j.inflight
		j.mu.Unlock()
		select {
		case <-c.done:
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %w", ErrJWKSUnavailable, ctx.Err())
		}
		waited = true
		j.mu.Lock()
	}
}

// mayFetch reports whether the rate limits allow a fetch now. j.mu is held.
func (j *JWKS) mayFetch() bool {
	if j.attemptedAt.IsZero() {
		return true
	}
	every := refetchEvery
	if j.fetchedAt.IsZero() {
		every = retryEvery
	}
	return j.now().Sub(j.attemptedAt) >= every
}

// missErr is the error for a kid not in the cache when no fetch may run.
// j.mu is held.
func (j *JWKS) missErr() error {
	if j.lastErr != nil {
		return fmt.Errorf("%w: %w", ErrJWKSUnavailable, j.lastErr)
	}
	return ErrUnknownKey
}

func (j *JWKS) fetch(ctx context.Context, c *fetchCall) {
	keys, err := j.download(ctx)
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lastErr = err
	if err == nil {
		j.keys = keys
		j.fetchedAt = j.now()
	}
	j.inflight = nil
	close(c.done)
}

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

func (j *JWKS) download(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS fetch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes+1))
	if err != nil {
		return nil, fmt.Errorf("JWKS fetch: %w", err)
	}
	if len(body) > maxJWKSBytes {
		return nil, errors.New("JWKS fetch: body over 1 MiB")
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("JWKS fetch: %w", err)
	}
	keys := map[string]ed25519.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		if (k.Alg != "" && k.Alg != "EdDSA") || (k.Use != "" && k.Use != "sig") {
			continue
		}
		x, err := b64.DecodeString(k.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			continue
		}
		keys[k.Kid] = ed25519.PublicKey(x)
	}
	if len(keys) == 0 {
		return nil, errors.New("JWKS fetch: no usable Ed25519 keys")
	}
	return keys, nil
}

// Status reports whether the last fetch succeeded and when one last did.
func (j *JWKS) Status() JWKSStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	var s JWKSStatus
	if !j.fetchedAt.IsZero() {
		at := j.fetchedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		s.FetchedAt = &at
		s.OK = j.lastErr == nil
	}
	if j.lastErr != nil {
		msg := j.lastErr.Error()
		s.Error = &msg
	}
	return s
}
