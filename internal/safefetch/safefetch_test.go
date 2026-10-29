package safefetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type hits struct{ n atomic.Int64 }

func (h *hits) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.n.Add(1)
		next(w, r)
	}
}

func serve(t *testing.T, h http.HandlerFunc) (*httptest.Server, *hits) {
	t.Helper()
	var c hits
	srv := httptest.NewServer(c.wrap(h))
	t.Cleanup(srv.Close)
	return srv, &c
}

func addrOf(srv *httptest.Server) netip.AddrPort {
	return netip.MustParseAddrPort(srv.Listener.Addr().String())
}

func portOf(srv *httptest.Server) string {
	return strconv.Itoa(int(addrOf(srv).Port()))
}

func fakeDNS(m map[string][]string) func(context.Context, string) ([]netip.Addr, error) {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		ss, ok := m[host]
		if !ok {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		var out []netip.Addr
		for _, s := range ss {
			out = append(out, netip.MustParseAddr(s))
		}
		return out, nil
	}
}

// client builds a Client that may reach only the given servers.
func client(o Options, allowed ...*httptest.Server) *Client {
	if o.Testing == nil {
		o.Testing = &TestHooks{}
	}
	for _, s := range allowed {
		o.Testing.AllowAddrPorts = append(o.Testing.AllowAddrPorts, addrOf(s))
	}
	return New(o)
}

func ok(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }
}

func wantBlocked(t *testing.T, err error) *BlockedError {
	t.Helper()
	var be *BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	return be
}

func TestAllowedServerFetches(t *testing.T) {
	srv, h := serve(t, ok("hello"))
	resp, err := client(Options{}, srv).Get(context.Background(), srv.URL+"/x", KindPage, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || string(resp.Body) != "hello" || resp.URL.Path != "/x" {
		t.Errorf("resp = %d %q %s", resp.Status, resp.Body, resp.URL)
	}
	if h.n.Load() != 1 {
		t.Errorf("hits = %d", h.n.Load())
	}
}

func TestBlockedLiterals(t *testing.T) {
	srv, h := serve(t, ok("secret"))
	var dials atomic.Int64
	c := New(Options{Testing: &TestHooks{OnDial: func(netip.AddrPort) { dials.Add(1) }}})
	for _, u := range []string{
		srv.URL, // loopback, not allowlisted
		"http://127.0.0.1/",
		"http://127.0.0.2:" + portOf(srv) + "/",
		"http://0.0.0.0:" + portOf(srv) + "/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/",
		"http://100.100.100.100/",
		"http://100.127.255.255/",
		"http://[::1]:" + portOf(srv) + "/",
		"http://[::]/",
		"http://[fe80::1%25lo0]/",
		"http://[fd7a:115c:a1e0::1]/",
		"http://[::ffff:127.0.0.1]:" + portOf(srv) + "/",
		"http://[::ffff:169.254.169.254]/",
		"http://[::ffff:a00:1]/",
		"http://[64:ff9b::a9fe:a9fe]/",
		"http://[64:ff9b::7f00:1]/",
		"http://[2002:7f00:1::1]/",
		"https://203.0.113.1/",
	} {
		_, err := c.Get(context.Background(), u, KindPage, "")
		wantBlocked(t, err)
	}
	if dials.Load() != 0 || h.n.Load() != 0 {
		t.Errorf("dials = %d, hits = %d; want 0, 0", dials.Load(), h.n.Load())
	}
}

func TestFakeDNSToLoopbackBlocked(t *testing.T) {
	srv, h := serve(t, ok("secret"))
	c := New(Options{Testing: &TestHooks{LookupNetIP: fakeDNS(map[string][]string{
		"evil.test":   {"127.0.0.1"},
		"evil6.test":  {"::1"},
		"mapped.test": {"::ffff:127.0.0.1"},
		"meta.test":   {"169.254.169.254"},
	})}})
	for _, host := range []string{"evil.test", "evil6.test", "mapped.test", "meta.test"} {
		_, err := c.Get(context.Background(), "http://"+host+":"+portOf(srv)+"/", KindPage, "")
		be := wantBlocked(t, err)
		if be.Host != host {
			t.Errorf("BlockedError.Host = %q, want %q", be.Host, host)
		}
	}
	if h.n.Load() != 0 {
		t.Errorf("hits = %d", h.n.Load())
	}
}

func TestMixedDNSAnswerBlocked(t *testing.T) {
	srv, h := serve(t, ok("x"))
	for _, answer := range [][]string{
		{"127.0.0.1", "10.0.0.5"},
		{"10.0.0.5", "127.0.0.1"},
		{"127.0.0.1", "::1"},
	} {
		c := client(Options{Testing: &TestHooks{LookupNetIP: fakeDNS(map[string][]string{"mixed.test": answer})}}, srv)
		_, err := c.Get(context.Background(), "http://mixed.test:"+portOf(srv)+"/", KindPage, "")
		wantBlocked(t, err)
	}
	if h.n.Load() != 0 {
		t.Errorf("hits = %d", h.n.Load())
	}
}

func TestPinnedDial(t *testing.T) {
	var host string
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		host = r.Host
		fmt.Fprint(w, "pinned")
	})
	var mu sync.Mutex
	var dialed []netip.AddrPort
	c := client(Options{Testing: &TestHooks{
		LookupNetIP: fakeDNS(map[string][]string{"pinned.test": {"127.0.0.1"}}),
		OnDial: func(ap netip.AddrPort) {
			mu.Lock()
			dialed = append(dialed, ap)
			mu.Unlock()
		},
	}}, srv)
	resp, err := c.Get(context.Background(), "http://pinned.test:"+portOf(srv)+"/", KindPage, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "pinned" {
		t.Errorf("body = %q", resp.Body)
	}
	if len(dialed) != 1 || dialed[0] != addrOf(srv) {
		t.Errorf("dialed = %v, want [%v]", dialed, addrOf(srv))
	}
	if want := "pinned.test:" + portOf(srv); host != want {
		t.Errorf("Host = %q, want %q", host, want)
	}
}

func TestDNSErrorPassesThrough(t *testing.T) {
	c := New(Options{Testing: &TestHooks{LookupNetIP: fakeDNS(nil)}})
	_, err := c.Get(context.Background(), "http://nowhere.test/", KindPage, "")
	var de *net.DNSError
	if !errors.As(err, &de) || !de.IsNotFound {
		t.Fatalf("err = %v, want not-found *net.DNSError", err)
	}
}

func TestRedirectToPrivateBlocked(t *testing.T) {
	inner, innerHits := serve(t, ok("secret"))
	outer, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		target := inner.URL + "/"
		if r.URL.Path == "/byname" {
			target = "http://internal.test:" + portOf(inner) + "/"
		}
		http.Redirect(w, r, target, http.StatusFound)
	})
	c := client(Options{Testing: &TestHooks{LookupNetIP: fakeDNS(map[string][]string{"internal.test": {"127.0.0.1"}})}}, outer)
	for _, path := range []string{"/", "/byname"} {
		_, err := c.Get(context.Background(), outer.URL+path, KindPage, "")
		wantBlocked(t, err)
	}
	if innerHits.n.Load() != 0 {
		t.Errorf("private server hits = %d, want 0", innerHits.n.Load())
	}
}

func redirector(t *testing.T) (*httptest.Server, *hits) {
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/r/"))
		if n > 0 {
			http.Redirect(w, r, "/r/"+strconv.Itoa(n-1), http.StatusFound)
			return
		}
		fmt.Fprint(w, "done")
	})
}

func TestRedirectLimit(t *testing.T) {
	srv, h := redirector(t)
	c := client(Options{}, srv)
	resp, err := c.Get(context.Background(), srv.URL+"/r/5", KindPage, "")
	if err != nil {
		t.Fatalf("5 redirects: %v", err)
	}
	if string(resp.Body) != "done" || resp.URL.Path != "/r/0" {
		t.Errorf("resp = %q at %s", resp.Body, resp.URL)
	}
	h.n.Store(0)
	_, err = c.Get(context.Background(), srv.URL+"/r/6", KindPage, "")
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("6 redirects: err = %v, want ErrTooManyRedirects", err)
	}
	if h.n.Load() != 6 {
		t.Errorf("hits = %d, want 6 (the 6th redirect is not followed)", h.n.Load())
	}
}

func TestUnsupportedURLs(t *testing.T) {
	c := New(Options{Testing: &TestHooks{OnDial: func(netip.AddrPort) { t.Error("dialed") }}})
	for _, u := range []string{
		"",
		"example.com",
		"/relative",
		"//example.com/",
		"ftp://example.com/",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"data:text/html,hi",
		"gopher://example.com/",
		"http:///nohost",
		"http://user:pass@example.com/",
		"http://user@example.com/",
		"https://:pass@example.com/",
		"http://exa mple.com/",
	} {
		if _, err := c.Get(context.Background(), u, KindPage, ""); !errors.Is(err, ErrUnsupportedURL) {
			t.Errorf("Get(%q) = %v, want ErrUnsupportedURL", u, err)
		}
	}
}

func TestRedirectToUnsupportedURL(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ftp":
			http.Redirect(w, r, "ftp://example.com/", http.StatusFound)
		case "/file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/creds":
			http.Redirect(w, r, "http://user:pw@127.0.0.1:"+portOf2(r)+"/ok", http.StatusFound)
		default:
			fmt.Fprint(w, "ok")
		}
	})
	c := client(Options{}, srv)
	for _, p := range []string{"/ftp", "/file", "/creds"} {
		if _, err := c.Get(context.Background(), srv.URL+p, KindPage, ""); !errors.Is(err, ErrUnsupportedURL) {
			t.Errorf("redirect %s: err = %v, want ErrUnsupportedURL", p, err)
		}
	}
	if h.n.Load() != 3 {
		t.Errorf("hits = %d, want 3", h.n.Load())
	}
}

func portOf2(r *http.Request) string {
	_, p, _ := net.SplitHostPort(r.Host)
	return p
}

func streamZeros(size int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := size
		w.WriteHeader(200)
		w.(http.Flusher).Flush() // no Content-Length: the cap must hold while reading
		buf := make([]byte, 32<<10)
		for n > 0 {
			k := min(n, len(buf))
			if _, err := w.Write(buf[:k]); err != nil {
				return
			}
			n -= k
		}
	}
}

func TestBodyCap(t *testing.T) {
	atCap, _ := serve(t, streamZeros(MaxPageBytes))
	over, _ := serve(t, streamZeros(MaxPageBytes+1))
	c := client(Options{}, atCap, over)
	resp, err := c.Get(context.Background(), atCap.URL, KindPage, "")
	if err != nil || len(resp.Body) != MaxPageBytes {
		t.Fatalf("at cap: err = %v", err)
	}
	if _, err := c.Get(context.Background(), over.URL, KindPage, ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("over page cap: err = %v, want ErrTooLarge", err)
	}
	resp, err = c.Get(context.Background(), over.URL, KindImage, "")
	if err != nil || len(resp.Body) != MaxPageBytes+1 {
		t.Errorf("image under its cap: err = %v", err)
	}
	bigImage, _ := serve(t, streamZeros(MaxImageBytes+1))
	c = client(Options{}, bigImage)
	if _, err := c.Get(context.Background(), bigImage.URL, KindImage, ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("over image cap: err = %v, want ErrTooLarge", err)
	}
}

func TestContentLengthOverCap(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(MaxImageBytes+1))
		w.WriteHeader(200)
		w.Write([]byte("x"))
	})
	c := client(Options{}, srv)
	start := time.Now()
	if _, err := c.Get(context.Background(), srv.URL, KindImage, ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v; Content-Length should be refused up front", d)
	}
}

func gz(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(b)
	zw.Close()
	return buf.Bytes()
}

func TestGzipBomb(t *testing.T) {
	bomb := gz(t, make([]byte, 50<<20))
	if len(bomb) > MaxPageBytes/10 {
		t.Fatalf("bomb is %d bytes compressed", len(bomb))
	}
	small := gz(t, []byte("<html>tiny</html>"))
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/declared":
			w.Header().Set("Content-Encoding", "gzip")
			w.Write(bomb)
		case "/undeclared":
			w.Write(bomb)
		case "/small":
			w.Write(small)
		}
	})
	c := client(Options{}, srv)
	for _, p := range []string{"/declared", "/undeclared"} {
		for _, k := range []Kind{KindPage, KindImage} {
			if _, err := c.Get(context.Background(), srv.URL+p, k, ""); !errors.Is(err, ErrTooLarge) {
				t.Errorf("%s kind %d: err = %v, want ErrTooLarge", p, k, err)
			}
		}
	}
	resp, err := c.Get(context.Background(), srv.URL+"/small", KindPage, "")
	if err != nil || string(resp.Body) != "<html>tiny</html>" {
		t.Errorf("unexpected gzip not decoded: %q, %v", resp.Body, err)
	}
}

func TestNon2xxReturnsTruncatedResponse(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		w.Write(bytes.Repeat([]byte("a"), 200<<10))
	})
	resp, err := client(Options{}, srv).Get(context.Background(), srv.URL, KindPage, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 403 || len(resp.Body) != 64<<10 || resp.Header.Get("Cf-Mitigated") != "challenge" {
		t.Errorf("resp = %d, %d bytes, %v", resp.Status, len(resp.Body), resp.Header)
	}
}

func TestSlowFirstByte(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	})
	c := client(Options{Timeout: 300 * time.Millisecond}, srv)
	start := time.Now()
	if _, err := c.Get(context.Background(), srv.URL, KindPage, ""); !errors.Is(err, ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestDripFedBody(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		for {
			if _, err := w.Write([]byte("a")); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})
	c := client(Options{Timeout: 300 * time.Millisecond}, srv)
	start := time.Now()
	if _, err := c.Get(context.Background(), srv.URL, KindPage, ""); !errors.Is(err, ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestParentCancelIsNotTimeout(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := client(Options{}, srv).Get(ctx, srv.URL, KindPage, "")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNoCookiesOrCredentials(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Cookie")+"|"+r.Header.Get("Authorization"))
		mu.Unlock()
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
			http.Redirect(w, r, "/check", http.StatusFound)
			return
		}
		fmt.Fprint(w, "ok")
	})
	c := client(Options{}, srv)
	for _, p := range []string{"/set", "/check"} {
		if _, err := c.Get(context.Background(), srv.URL+p, KindPage, ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("requests = %d, want 3", len(seen))
	}
	for i, s := range seen {
		if s != "|" {
			t.Errorf("request %d sent cookie|auth %q", i, s)
		}
	}
}

func TestRequestHeaders(t *testing.T) {
	var got http.Header
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone() })
	c := client(Options{UserAgent: "Sous/1.2.3 (+https://github.com/sroberts/sous)"}, srv)

	if _, err := c.Get(context.Background(), srv.URL, KindPage, ""); err != nil {
		t.Fatal(err)
	}
	if ua := got.Get("User-Agent"); ua != "Sous/1.2.3 (+https://github.com/sroberts/sous)" {
		t.Errorf("User-Agent = %q", ua)
	}
	if a := got.Get("Accept"); !strings.HasPrefix(a, "text/html") {
		t.Errorf("page Accept = %q", a)
	}
	if got.Get("Referer") != "" {
		t.Errorf("Referer = %q, want none", got.Get("Referer"))
	}

	if _, err := c.Get(context.Background(), srv.URL, KindImage, "https://example.com/recipe"); err != nil {
		t.Fatal(err)
	}
	a := got.Get("Accept")
	if !strings.HasPrefix(a, "image/webp") || strings.Contains(a, "avif") {
		t.Errorf("image Accept = %q", a)
	}
	if r := got.Get("Referer"); r != "https://example.com/recipe" {
		t.Errorf("Referer = %q", r)
	}

	if _, err := client(Options{}, srv).Get(context.Background(), srv.URL, KindPage, ""); err != nil {
		t.Fatal(err)
	}
	if ua := got.Get("User-Agent"); ua != DefaultUserAgent {
		t.Errorf("default User-Agent = %q", ua)
	}
}

func TestEnvironmentProxyIgnored(t *testing.T) {
	proxy, proxyHits := serve(t, ok("proxied"))
	target, targetHits := serve(t, ok("direct"))
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY"} {
		t.Setenv(k, proxy.URL)
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	c := client(Options{Testing: &TestHooks{LookupNetIP: fakeDNS(map[string][]string{"site.test": {"127.0.0.1"}})}}, target)
	resp, err := c.Get(context.Background(), "http://site.test:"+portOf(target)+"/", KindPage, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "direct" || proxyHits.n.Load() != 0 || targetHits.n.Load() != 1 {
		t.Errorf("body %q, proxy hits %d, target hits %d", resp.Body, proxyHits.n.Load(), targetHits.n.Load())
	}
	if tr := c.hc.Transport.(*http.Transport); tr.Proxy != nil {
		t.Error("Transport.Proxy is set")
	}
}

func TestDefaults(t *testing.T) {
	c := New(Options{})
	if c.opts.Timeout != 10*time.Second || c.opts.MaxRedirects != 5 || c.opts.UserAgent != DefaultUserAgent {
		t.Errorf("opts = %+v", c.opts)
	}
	if KindPage.limit() != 5<<20 || KindImage.limit() != 25<<20 {
		t.Errorf("limits = %d, %d", KindPage.limit(), KindImage.limit())
	}
	if c.hc.Jar != nil {
		t.Error("cookie jar set")
	}
	tr := c.hc.Transport.(*http.Transport)
	if tr.Proxy != nil || tr.TLSHandshakeTimeout != 10*time.Second ||
		tr.ResponseHeaderTimeout != 10*time.Second || tr.MaxResponseHeaderBytes != 64<<10 {
		t.Errorf("transport = %+v", tr)
	}
	if c.onDial != nil || len(c.allow) != 0 {
		t.Error("test hooks active without Options.Testing")
	}
}
