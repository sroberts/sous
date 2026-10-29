// Package conformance builds the sous binary and runs the Outpost spec §13
// checks against it as a black box.
package conformance

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

var binary string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "sous-conformance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "sous")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/sous")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// --- helpers ---

// scratch is an isolated world for one check. Home, Tmp and Work must stay
// empty; only Storage may be written.
type scratch struct {
	Home, Tmp, Work, Storage string
}

func newScratch(t *testing.T) scratch {
	t.Helper()
	root := t.TempDir()
	s := scratch{
		Home:    filepath.Join(root, "home"),
		Tmp:     filepath.Join(root, "tmp"),
		Work:    filepath.Join(root, "work"),
		Storage: filepath.Join(root, "storage"),
	}
	for _, d := range []string{s.Home, s.Tmp, s.Work, s.Storage} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// lines collects process output line by line.
type lines struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	lines []string
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.Write(p)
	for {
		i := bytes.IndexByte(l.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		l.lines = append(l.lines, string(l.buf.Next(i + 1)[:i]))
	}
	return len(p), nil
}

func (l *lines) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]string(nil), l.lines...)
	if l.buf.Len() > 0 {
		out = append(out, l.buf.String())
	}
	return out
}

// proc is a running sous process.
type proc struct {
	t      *testing.T
	cmd    *exec.Cmd
	sc     scratch
	port   int
	stdout lines
	stderr lines
	done   chan struct{}
	err    error // valid once done is closed
}

// start launches sous with the given port arguments and extra environment.
// The environment is built from scratch so nothing leaks in from the host.
func start(t *testing.T, sc scratch, port int, extraEnv ...string) *proc {
	t.Helper()
	p := &proc{t: t, sc: sc, port: port, done: make(chan struct{})}
	var args []string
	if port != 0 {
		args = []string{"--port", fmt.Sprint(port)}
	}
	p.cmd = exec.Command(binary, args...)
	p.cmd.Dir = sc.Work
	p.cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + sc.Home,
		"TMPDIR=" + sc.Tmp,
		"OUTPOST_STORAGE_DIR=" + sc.Storage,
	}, extraEnv...)
	p.cmd.Stdout = &p.stdout
	p.cmd.Stderr = &p.stderr
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			p.cmd.Process.Kill()
			<-p.done
		}
	})
	return p
}

// startReady starts sous and waits until /up reports 200.
func startReady(t *testing.T, sc scratch) *proc {
	t.Helper()
	p := start(t, sc, freePort(t))
	p.waitUp(10 * time.Second)
	return p
}

func (p *proc) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *proc) exitCode() int {
	if p.err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(p.err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func (p *proc) url(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", p.port, path)
}

// waitUp polls /up until it returns 200.
func (p *proc) waitUp(timeout time.Duration) {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.exited() {
			p.t.Fatalf("exited before ready: %v\nstdout:\n%s\nstderr:\n%s", p.err,
				strings.Join(p.stdout.all(), "\n"), strings.Join(p.stderr.all(), "\n"))
		}
		if status, err := p.probe("/up"); err == nil && status == 200 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	p.t.Fatalf("not ready within %s", timeout)
}

func (p *proc) probe(path string) (int, error) {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(p.url(path))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// waitLog waits for a stdout line whose "msg" is msg.
func (p *proc) waitLog(msg string, timeout time.Duration) map[string]any {
	p.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		for _, l := range p.stdout.all() {
			var obj map[string]any
			if json.Unmarshal([]byte(l), &obj) == nil && obj["msg"] == msg {
				return obj
			}
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("no %q log line within %s; got:\n%s", msg, timeout, strings.Join(p.stdout.all(), "\n"))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// stop sends SIGTERM and waits for exit.
func (p *proc) stop(timeout time.Duration) time.Duration {
	p.t.Helper()
	begin := time.Now()
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		p.t.Fatal(err)
	}
	select {
	case <-p.done:
		return time.Since(begin)
	case <-time.After(timeout):
		p.t.Fatalf("still running %s after SIGTERM", timeout)
		return 0
	}
}

// do sends a request; person, if set, becomes X-Sous-Person.
func (p *proc) do(method, path, person string, headers map[string]string, body []byte) (*http.Response, []byte) {
	p.t.Helper()
	req, err := http.NewRequest(method, p.url(path), bytes.NewReader(body))
	if err != nil {
		p.t.Fatal(err)
	}
	if person != "" {
		req.Header.Set("X-Sous-Person", person)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c := http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func (p *proc) doJSON(method, path, person string, headers map[string]string, body any) (*http.Response, []byte) {
	p.t.Helper()
	var b []byte
	if body != nil {
		var err error
		if b, err = json.Marshal(body); err != nil {
			p.t.Fatal(err)
		}
	}
	h := map[string]string{"Content-Type": "application/json"}
	for k, v := range headers {
		h[k] = v
	}
	return p.do(method, path, person, h, b)
}

func expectStatus(t *testing.T, what string, resp *http.Response, body []byte, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s: status %d, want %d: %s", what, resp.StatusCode, want, body)
	}
}

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// lockDatabase creates sous.db in WAL mode and holds BEGIN EXCLUSIVE on it.
// In WAL mode readers still work but the write that applies migrations
// blocks, which holds sous in its startup phase.
func lockDatabase(t *testing.T, dir string) (release func()) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "sous.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release = func() {
		once.Do(func() {
			conn.ExecContext(ctx, "ROLLBACK")
			conn.Close()
			db.Close()
		})
	}
	t.Cleanup(release)
	return release
}

// --- the checks ---

func skipShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("conformance builds and runs the binary; skipped under -short")
	}
}

func TestLoopbackOnly(t *testing.T) {
	skipShort(t)
	p := startReady(t, newScratch(t))

	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p.port), time.Second)
	if err != nil {
		t.Fatalf("loopback dial: %v", err)
	}
	c.Close()

	addrs, _ := net.InterfaceAddrs()
	var other net.IP
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() && ipn.IP.To4() != nil {
			other = ipn.IP
			break
		}
	}
	if other == nil {
		t.Log("no non-loopback IPv4 address; skipping the external half")
		return
	}
	c, err = net.DialTimeout("tcp", net.JoinHostPort(other.String(), fmt.Sprint(p.port)), time.Second)
	if err == nil {
		c.Close()
		t.Fatalf("reachable on %s; must bind 127.0.0.1 only", other)
	}
}

func TestStartup503Then200(t *testing.T) {
	skipShort(t)
	sc := newScratch(t)
	release := lockDatabase(t, sc.Storage)
	p := start(t, sc, freePort(t))

	// The listener comes up before storage opens; wait for any answer.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := p.probe("/up"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never listened")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if status, _ := p.probe("/up"); status != 503 {
		t.Fatalf("/up while starting = %d, want 503", status)
	}
	resp, body := p.do("GET", "/v1/status", "", nil, nil)
	expectStatus(t, "/v1/status while starting", resp, body, 503)
	var e struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.Error.Code != "starting" {
		t.Fatalf("/v1/status body = %s, want error code starting", body)
	}
	if p.exited() {
		t.Fatalf("exited while the database was locked (busy_timeout shorter than the lock?): %v", p.err)
	}

	release()
	p.waitUp(10 * time.Second)
}

func TestUpSpeedAndSilence(t *testing.T) {
	skipShort(t)
	p := startReady(t, newScratch(t))

	var worst time.Duration
	for i := 0; i < 50; i++ {
		begin := time.Now()
		status, err := p.probe("/up")
		if d := time.Since(begin); d > worst {
			worst = d
		}
		if err != nil || status != 200 {
			t.Fatalf("probe %d: status %d, err %v", i, status, err)
		}
	}
	if worst >= 100*time.Millisecond {
		t.Fatalf("slowest /up = %s, want under 100ms", worst)
	}

	// A request that is logged proves the log is live, so the absence of
	// /up is meaningful.
	p.do("GET", "/v1/status", "", nil, nil)
	deadline := time.Now().Add(2 * time.Second)
	for !logged(p, "/v1/status") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !logged(p, "/v1/status") {
		t.Fatal("request logging did not record /v1/status")
	}
	if logged(p, `"/up"`) {
		t.Fatal("/up appears in the logs")
	}
}

func logged(p *proc, sub string) bool {
	for _, l := range p.stdout.all() {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestSIGTERM(t *testing.T) {
	skipShort(t)
	sc := newScratch(t)
	p := startReady(t, sc)

	took := p.stop(7 * time.Second)
	if code := p.exitCode(); code != 0 {
		t.Fatalf("exit code %d after SIGTERM, want 0; stderr:\n%s", code, strings.Join(p.stderr.all(), "\n"))
	}
	t.Logf("exited in %s", took)
	p.waitLog("stopped", time.Second)

	names := readDir(t, sc.Storage)
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
		if strings.HasSuffix(n, "-wal") || strings.HasSuffix(n, "-shm") {
			t.Errorf("leftover %s after clean shutdown", n)
		}
	}
	for _, want := range []string{"sous.db", "images", "tmp"} {
		if !have[want] {
			t.Errorf("storage is missing %s; has %v", want, names)
		}
	}
}

func TestExitOne(t *testing.T) {
	skipShort(t)
	t.Run("port taken", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		p := start(t, newScratch(t), l.Addr().(*net.TCPAddr).Port)
		expectExit1(t, p)
	})
	t.Run("no port", func(t *testing.T) {
		expectExit1(t, start(t, newScratch(t), 0))
	})
	t.Run("bogus auth", func(t *testing.T) {
		expectExit1(t, start(t, newScratch(t), freePort(t), "OUTPOST_AUTH=bogus"))
	})
	t.Run("database from the future", func(t *testing.T) {
		sc := newScratch(t)
		db, err := sql.Open("sqlite", filepath.Join(sc.Storage, "sous.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("PRAGMA user_version=999"); err != nil {
			t.Fatal(err)
		}
		db.Close()
		expectExit1(t, start(t, sc, freePort(t)))
	})
}

func expectExit1(t *testing.T, p *proc) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		t.Fatal("still running; expected it to exit 1")
	}
	if code := p.exitCode(); code != 1 {
		t.Fatalf("exit code %d, want 1; stderr:\n%s", code, strings.Join(p.stderr.all(), "\n"))
	}
}

func TestNoWritesOutsideStorage(t *testing.T) {
	skipShort(t)
	sc := newScratch(t)
	p := startReady(t, sc)

	resp, body := p.doJSON("POST", "/v1/people", "", nil, map[string]string{"name": "Test"})
	expectStatus(t, "create person", resp, body, 201)
	var person struct{ Username string }
	json.Unmarshal(body, &person)
	if person.Username == "" {
		t.Fatalf("no username in %s", body)
	}

	recipe := map[string]any{
		"title":       "Soup",
		"tags":        []string{"dinner"},
		"ingredients": []string{"# Broth", "2 cups water", "1 tsp salt"},
		"steps":       []string{"Boil."},
	}
	resp, body = p.doJSON("POST", "/v1/recipes", person.Username, nil, recipe)
	expectStatus(t, "create recipe", resp, body, 201)
	var created struct {
		ID      string
		Version int
	}
	json.Unmarshal(body, &created)
	etag := resp.Header.Get("ETag")
	if created.ID == "" || etag == "" {
		t.Fatalf("create returned id %q etag %q", created.ID, etag)
	}
	path := "/v1/recipes/" + created.ID

	resp, body = p.doJSON("PATCH", path, person.Username, nil, map[string]string{"title": "Better soup"})
	expectStatus(t, "patch without If-Match", resp, body, 428)

	resp, body = p.doJSON("PATCH", path, person.Username, map[string]string{"If-Match": etag}, map[string]string{"title": "Better soup"})
	expectStatus(t, "patch", resp, body, 200)
	newTag := resp.Header.Get("ETag")
	if newTag == etag {
		t.Fatalf("ETag did not change after patch: %s", etag)
	}

	resp, body = p.doJSON("PATCH", path, person.Username, map[string]string{"If-Match": etag}, map[string]string{"title": "Stale"})
	expectStatus(t, "patch with stale If-Match", resp, body, 412)

	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89" +
		"\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9a\xa0\xa0\x00\x00\x00\x00IEND\xaeB`\x82")
	resp, body = p.do("POST", "/v1/images", person.Username, map[string]string{"Content-Type": "image/png"}, png)
	expectStatus(t, "upload image", resp, body, 201)
	var img struct{ Hash string }
	json.Unmarshal(body, &img)
	resp, body = p.do("GET", "/v1/images/"+img.Hash, "", nil, nil)
	expectStatus(t, "fetch image", resp, body, 200)

	resp, body = p.doJSON("DELETE", path, person.Username, map[string]string{"If-Match": newTag}, nil)
	expectStatus(t, "delete", resp, body, 204)
	resp, body = p.doJSON("POST", path+"/restore", person.Username, nil, nil)
	expectStatus(t, "restore", resp, body, 200)

	p.stop(7 * time.Second)
	for name, dir := range map[string]string{"HOME": sc.Home, "TMPDIR": sc.Tmp, "working dir": sc.Work} {
		if got := readDir(t, dir); len(got) != 0 {
			t.Errorf("%s was written to: %v", name, got)
		}
	}
}

func TestCleanStartAfterEmptyingStorage(t *testing.T) {
	skipShort(t)
	sc := newScratch(t)
	first := startReady(t, sc)
	first.doJSON("POST", "/v1/people", "", nil, map[string]string{"name": "Test"})
	first.stop(7 * time.Second)

	for _, name := range readDir(t, sc.Storage) {
		if err := os.RemoveAll(filepath.Join(sc.Storage, name)); err != nil {
			t.Fatal(err)
		}
	}
	p := startReady(t, sc)
	p.waitLog("ready", 2*time.Second)
	p.stop(7 * time.Second)
	if code := p.exitCode(); code != 0 {
		t.Fatalf("exit code %d, want 0", code)
	}
}

func TestStablePID(t *testing.T) {
	skipShort(t)
	p := startReady(t, newScratch(t))
	pid := p.cmd.Process.Pid
	time.Sleep(2 * time.Second)
	if p.exited() {
		t.Fatalf("pid %d exited within 2s: %v", pid, p.err)
	}
	if status, err := p.probe("/up"); err != nil || status != 200 {
		t.Fatalf("not serving after 2s: status %d, err %v", status, err)
	}
	if err := p.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("pid %d is gone: %v", pid, err)
	}
}

func TestLogFormat(t *testing.T) {
	skipShort(t)
	p := startReady(t, newScratch(t))
	p.do("GET", "/v1/status", "", nil, nil)
	p.do("GET", "/v1/nope", "", nil, nil)
	p.stop(7 * time.Second)

	out := p.stdout.all()
	if len(out) == 0 {
		t.Fatal("no stdout output")
	}
	for _, l := range out {
		var obj map[string]any
		if err := json.Unmarshal([]byte(l), &obj); err != nil {
			t.Errorf("not a JSON object: %q", l)
		}
		if strings.Contains(l, "\x1b") {
			t.Errorf("ANSI escape in %q", l)
		}
	}
	for _, l := range p.stderr.all() {
		if strings.Contains(l, "\x1b") {
			t.Errorf("ANSI escape on stderr: %q", l)
		}
	}
}
