package identity

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
)

// Errors returned by (*JWKS).Key. The resolver maps ErrUnknownKey to 401
// invalid_identity and ErrJWKSUnavailable to 503 identity_unavailable.
var (
	ErrUnknownKey      = errors.New("identity: unknown key id")
	ErrJWKSUnavailable = errors.New("identity: JWKS unavailable")
)

// JWKS is an in-memory cache of the Outpost gateway's signing keys, fetched
// from OUTPOST_JWKS_URL.
type JWKS struct {
	url    string
	client *http.Client
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
	return &JWKS{url: url, client: client}
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
	panic("TODO")
}

// Status reports whether the last fetch succeeded and when one last did.
func (j *JWKS) Status() JWKSStatus {
	return JWKSStatus{}
}
