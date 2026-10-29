// Package safefetch fetches user-supplied URLs without letting them reach the
// household network: every connection, including each redirect hop, is pinned
// to an address that was checked as public.
package safefetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// Kind selects the size cap and Accept header for a fetch.
type Kind int

const (
	KindPage Kind = iota
	KindImage
)

const (
	MaxPageBytes  = 5 << 20
	MaxImageBytes = 25 << 20

	// errorBodyBytes caps the body kept from a non-2xx response.
	errorBodyBytes = 64 << 10

	DefaultTimeout      = 10 * time.Second
	DefaultMaxRedirects = 5
	DefaultUserAgent    = "Sous (+https://github.com/sroberts/sous)"
)

func (k Kind) limit() int64 {
	if k == KindImage {
		return MaxImageBytes
	}
	return MaxPageBytes
}

func (k Kind) accept() string {
	if k == KindImage {
		return "image/webp,image/png,image/jpeg,*/*;q=0.5"
	}
	return "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5"
}

var (
	ErrUnsupportedURL   = errors.New("safefetch: unsupported URL")
	ErrTooLarge         = errors.New("safefetch: response too large")
	ErrTooManyRedirects = errors.New("safefetch: too many redirects")
	ErrTimeout          = errors.New("safefetch: timed out")
)

type Options struct {
	UserAgent    string
	Timeout      time.Duration // whole fetch, including redirects and body; default 10s
	MaxRedirects int           // default 5
	Testing      *TestHooks
}

// TestHooks let tests reach httptest servers. New panics if they are set
// outside a test binary.
type TestHooks struct {
	// AllowAddrPorts are exact address:port pairs exempt from the blocklist.
	AllowAddrPorts []netip.AddrPort
	LookupNetIP    func(ctx context.Context, host string) ([]netip.Addr, error)
	OnDial         func(netip.AddrPort)
}

type Response struct {
	URL    *url.URL // after redirects
	Status int
	Header http.Header
	Body   []byte
}

type Client struct {
	opts   Options
	hc     *http.Client
	self   map[netip.Addr]bool
	allow  map[netip.AddrPort]bool
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	onDial func(netip.AddrPort)
}

func New(o Options) *Client {
	if o.Testing != nil && !testing.Testing() {
		panic("safefetch: TestHooks used outside tests")
	}
	if o.UserAgent == "" {
		o.UserAgent = DefaultUserAgent
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = DefaultMaxRedirects
	}
	c := &Client{
		opts:  o,
		self:  interfaceAddrs(),
		allow: map[netip.AddrPort]bool{},
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
	}
	if h := o.Testing; h != nil {
		for _, ap := range h.AllowAddrPorts {
			c.allow[netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())] = true
		}
		if h.LookupNetIP != nil {
			c.lookup = h.LookupNetIP
		}
		c.onDial = h.OnDial
	}
	tr := &http.Transport{
		Proxy:                  nil,
		DialContext:            c.dial,
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		MaxIdleConns:           16,
		IdleConnTimeout:        30 * time.Second,
	}
	c.hc = &http.Client{
		Transport:     tr,
		Jar:           nil,
		CheckRedirect: c.checkRedirect,
	}
	return c
}

func (c *Client) permitted(ap netip.AddrPort) bool {
	ap = netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
	if c.allow[ap] {
		return true
	}
	return !c.self[ap.Addr()] && Public(ap.Addr())
}

func (c *Client) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("safefetch: bad port %q", portStr)
	}
	var addrs []netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{a}
	} else {
		addrs, err = c.lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
	}
	// Refuse if any answer is blocked, so a mixed answer can't be raced.
	targets := make([]netip.AddrPort, len(addrs))
	for i, a := range addrs {
		ap := netip.AddrPortFrom(a.Unmap(), uint16(port))
		if !c.permitted(ap) {
			return nil, &BlockedError{Host: host, Addr: ap.Addr()}
		}
		targets[i] = ap
	}
	// With several addresses, give each a short slice so a blackholed IPv6
	// address can't spend the whole fetch budget before IPv4 is tried.
	timeout := c.opts.Timeout
	if len(targets) > 1 && timeout > 3*time.Second {
		timeout = 3 * time.Second
	}
	d := &net.Dialer{Timeout: timeout, Control: c.control}
	var lastErr error
	for _, ap := range targets {
		if c.onDial != nil {
			c.onDial(ap)
		}
		// Dial the checked literal so the connection is pinned to it.
		conn, err := d.DialContext(ctx, network, ap.String())
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

// control is a backstop: it re-checks the address the socket is actually
// connecting to.
func (c *Client) control(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("safefetch: unparseable dial address %q", address)
	}
	if !c.permitted(ap) {
		return &BlockedError{Addr: ap.Addr().Unmap()}
	}
	return nil
}

func checkURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q", ErrUnsupportedURL, u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("%w: credentials in URL", ErrUnsupportedURL)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("%w: no host", ErrUnsupportedURL)
	}
	return nil
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.opts.MaxRedirects {
		return ErrTooManyRedirects
	}
	if err := checkURL(req.URL); err != nil {
		return err
	}
	req.Header.Del("Cookie")
	req.Header.Del("Authorization")
	return nil
}

// Get fetches rawURL. A non-2xx status is returned as a Response, with at most
// 64 KiB of body, not as an error.
func (c *Client) Get(ctx context.Context, rawURL string, kind Kind, referer string) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedURL, err)
	}
	if err := checkURL(u); err != nil {
		return nil, err
	}
	fctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedURL, err)
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	req.Header.Set("Accept", kind.accept())
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, c.mapErr(ctx, fctx, err)
	}
	defer resp.Body.Close()

	limit := kind.limit()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !ok {
		limit = errorBodyBytes
	} else if resp.ContentLength > limit {
		return nil, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, c.mapErr(ctx, fctx, err)
	}
	if int64(len(body)) > limit {
		if !ok {
			body = body[:limit]
		} else {
			return nil, ErrTooLarge
		}
	}
	if ok && len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		if body, err = gunzip(body, limit); err != nil {
			return nil, err
		}
	}
	return &Response{URL: resp.Request.URL, Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// gunzip decompresses a body the server gzipped without saying so, under the
// same cap as the raw body.
func gunzip(b []byte, limit int64) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b, nil
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, limit+1))
	if int64(len(out)) > limit {
		return nil, ErrTooLarge
	}
	if err != nil {
		// Not really gzip after all; keep the original bytes.
		return b, nil
	}
	return out, nil
}

func (c *Client) mapErr(ctx, fctx context.Context, err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var be *BlockedError
	if errors.As(err, &be) {
		return be
	}
	switch {
	case errors.Is(err, ErrUnsupportedURL), errors.Is(err, ErrTooManyRedirects):
		return err
	}
	var de *net.DNSError
	if errors.As(err, &de) && !de.IsTimeout {
		return de
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ne net.Error
	if fctx.Err() != nil || (errors.As(err, &ne) && ne.Timeout()) {
		return ErrTimeout
	}
	return err
}
