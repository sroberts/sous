// Package mcp serves the MCP tools (spec.md §9) over the streamable HTTP
// transport, calling the same service layer as REST.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/identity"
	"github.com/sroberts/sous/internal/importer"
	"github.com/sroberts/sous/internal/recipes"
)

// HeaderCaller carries the resolved username from api.WithIdentity to the
// tools, which see the HTTP request only as req.Extra.Header. WithIdentity is
// the trust boundary: it must delete any client-sent copy before setting it.
const HeaderCaller = identity.CallerHeader

const instructions = `Sous is a household's shared recipe collection. You can search, read, create, edit, delete and restore recipes, and set your own favourite and want-to-cook flags.
Every write is attributed to the person this connection names (the X-Sous-Person header, or the Outpost login) and is recorded in the change log as made via MCP.
To edit or delete a recipe, first call get_recipe and pass its version back; if someone changed it since, you get a stale error with the current recipe to retry against.
Deletes are soft: a deleted recipe goes to the trash and restore_recipe brings it back.`

const importInstructions = `
To add a recipe from a web page, call import_recipe with its link; check the draft it returns, then save it with create_recipe, or pass save=true to save it as read.`

// Option configures New.
type Option func(*options)

type options struct {
	importer *importer.Importer
}

// WithImporter registers import_recipe, which reads recipes from links.
func WithImporter(im *importer.Importer) Option {
	return func(o *options) { o.importer = im }
}

// New returns the /mcp handler with the tools from spec.md §9 registered.
// It expects to be wrapped by api.WithIdentity, which resolves the caller.
func New(svc *recipes.Service, log *slog.Logger, version string, opts ...Option) http.Handler {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	instr := instructions
	if o.importer != nil {
		instr += importInstructions
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "sous", Title: "Sous", Version: version}, &sdk.ServerOptions{
		Instructions: instr,
		// Stateless mode connects a session per request, and the SDK logs
		// each at Info.
		Logger: slog.New(warnAndAbove{log.Handler()}),
	})
	t := &tools{svc: svc, log: log, importer: o.importer}
	register(server, t)
	if t.importer != nil {
		registerImport(server, t)
	}

	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{
		// No sessions to lose on restart.
		Stateless:    true,
		JSONResponse: true,
		// Outpost's gateway and tsnet reach the listener with the app's
		// public Host (sous.<domain>), which the localhost check rejects.
		DisableLocalhostProtection: true,
		Logger:                     log,
	})
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"cross_origin","message":"cross-origin requests to /mcp are not allowed"}}`))
	}))
	return cop.Handler(h)
}

type tools struct {
	svc      *recipes.Service
	log      *slog.Logger
	importer *importer.Importer
}

// caller prefers the Caller api.WithIdentity put on the request context,
// which the SDK carries into handlers in stateless mode, and falls back to
// HeaderCaller. Either way the write is recorded as via mcp.
func (t *tools) caller(ctx context.Context, req *sdk.CallToolRequest) (identity.Caller, error) {
	if c, ok := identity.CallerFrom(ctx); ok {
		c.Via = identity.ViaMCP
		return c, nil
	}
	c := identity.Caller{Route: identity.RouteNone, Via: identity.ViaMCP}
	if req.Extra == nil || req.Extra.Header == nil {
		return c, nil
	}
	username := req.Extra.Header.Get(HeaderCaller)
	if username == "" {
		return c, nil
	}
	p, err := t.svc.Lookup(ctx, username)
	if err != nil {
		return c, err
	}
	if p == nil {
		return c, recipes.Errorf(identity.CodeUnknownPerson, "no one is called %q", username)
	}
	c.Person, c.Route = p, identity.RouteDirect
	return c, nil
}

// toolError turns a failure into the text of an IsError tool result:
// "<code>: <message>", which the SDK sets when a handler returns an error.
func (t *tools) toolError(tool string, err error) error {
	var ie *recipes.ImportError
	if errors.As(err, &ie) {
		msg := ie.Code + ": " + ie.Message
		if ie.Page != nil && ie.Page.Title != nil {
			msg += fmt.Sprintf(" (the page is titled %q)", *ie.Page.Title)
		}
		return errors.New(msg)
	}
	var e *recipes.Error
	if !errors.As(err, &e) {
		t.log.Error("mcp tool failed", "tool", tool, "error", err.Error())
		return errors.New("internal: something went wrong on the server; try again")
	}
	switch e.Code {
	case recipes.CodeWhoAreYou:
		return errors.New(recipes.CodeWhoAreYou + `: this connection doesn't name a person, so it can only read. ` +
			`Add a header naming your Sous username to this MCP server in your client's config, ` +
			`for example "headers": {"X-Sous-Person": "alice"}, or connect through the Outpost login gateway.`)
	case recipes.CodeStale:
		if e.Current != nil {
			cur, err := json.Marshal(e.Current)
			if err != nil {
				return t.toolError(tool, err)
			}
			return fmt.Errorf("%s: %s\nThe recipe as it stands now follows; reapply your change to it and retry with version %d.\n%s",
				e.Code, e.Message, e.Current.Version, cur)
		}
	}
	return errors.New(e.Code + ": " + e.Message)
}

// add registers a tool whose input and output schemas come from
// recipes.SchemaFor: AddTool's own inference uses empty ForOptions and would
// describe Optional fields as objects.
func add[In, Out any](s *sdk.Server, t *tools, tool *sdk.Tool, h func(context.Context, identity.Caller, In) (Out, error)) {
	tool.InputSchema = recipes.SchemaFor[In]()
	tool.OutputSchema = recipes.SchemaFor[Out]()
	sdk.AddTool(s, tool, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
		var zero Out
		c, err := t.caller(ctx, req)
		if err != nil {
			return nil, zero, t.toolError(tool.Name, err)
		}
		out, err := h(ctx, c, in)
		if err != nil {
			return nil, zero, t.toolError(tool.Name, err)
		}
		return nil, out, nil
	})
}

// warnAndAbove drops records below Warn.
type warnAndAbove struct{ slog.Handler }

func (h warnAndAbove) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelWarn && h.Handler.Enabled(ctx, l)
}

func (h warnAndAbove) WithAttrs(as []slog.Attr) slog.Handler {
	return warnAndAbove{h.Handler.WithAttrs(as)}
}

func (h warnAndAbove) WithGroup(name string) slog.Handler {
	return warnAndAbove{h.Handler.WithGroup(name)}
}
