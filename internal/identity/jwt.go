package identity

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// skew is the clock-skew allowance on exp and iat.
const skew = 30 * time.Second

// b64 rejects padding and non-zero trailing bits, so each token has exactly
// one encoding.
var b64 = base64.RawURLEncoding.Strict()

// errInvalid wraps every verification failure that is the token's fault.
var errInvalid = errors.New("invalid identity token")

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errInvalid, fmt.Sprintf(format, args...))
}

type tokenHeader struct {
	Alg  string          `json:"alg"`
	Kid  string          `json:"kid"`
	Crit json.RawMessage `json:"crit"`
}

// claims is the X-Outpost-Identity payload as Outpost's gateway signs it.
type claims struct {
	Iss    string   `json:"iss"`
	Aud    audience `json:"aud"`
	Sub    string   `json:"sub"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Groups []string `json:"groups"`
	Iat    *int64   `json:"iat"`
	Exp    *int64   `json:"exp"`
}

// audience is aud as either a single string (what Outpost sends) or an
// array of strings (RFC 7519 §4.1.3).
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*a = audience{s}
		return nil
	}
	var ss []string
	if err := json.Unmarshal(b, &ss); err != nil {
		return errors.New("aud must be a string or an array of strings")
	}
	*a = ss
	return nil
}

// keyFunc returns the verification key for kid. Its errors are returned
// from verify unwrapped, so ErrJWKSUnavailable stays distinguishable.
type keyFunc func(kid string) (ed25519.PublicKey, error)

// verify checks a compact EdDSA JWS from Outpost and returns its claims. The
// signature is checked before the payload is even decoded.
func verify(token string, key keyFunc, iss, aud string, now time.Time) (*claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, invalidf("want 3 segments, got %d", len(parts))
	}
	rawHeader, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, invalidf("header encoding")
	}
	var h tokenHeader
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return nil, invalidf("header json")
	}
	if h.Alg != "EdDSA" {
		return nil, invalidf("alg %q not accepted", h.Alg)
	}
	if h.Kid == "" {
		return nil, invalidf("no kid")
	}
	// Outpost never sets crit, and RFC 7515 §4.1.11 requires rejecting
	// extensions we don't understand.
	if h.Crit != nil {
		return nil, invalidf("crit not supported")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, invalidf("signature encoding")
	}
	pub, err := key(h.Kid)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, invalidf("bad signature")
	}

	rawPayload, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, invalidf("payload encoding")
	}
	var c claims
	if err := json.Unmarshal(rawPayload, &c); err != nil {
		return nil, invalidf("payload json: %v", err)
	}
	if c.Iss != iss {
		return nil, invalidf("iss %q", c.Iss)
	}
	if !slices.Contains(c.Aud, aud) {
		return nil, invalidf("aud %q does not match host %q", []string(c.Aud), aud)
	}
	if c.Sub == "" {
		return nil, invalidf("no sub")
	}
	if c.Exp == nil || c.Iat == nil {
		return nil, invalidf("exp and iat are required")
	}
	if !now.Before(time.Unix(*c.Exp, 0).Add(skew)) {
		return nil, invalidf("expired")
	}
	if time.Unix(*c.Iat, 0).After(now.Add(skew)) {
		return nil, invalidf("issued in the future")
	}
	return &c, nil
}
