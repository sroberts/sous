// Package mcp serves the MCP tools (spec.md §9) over the streamable HTTP
// transport, calling the same service layer as REST.
package mcp

import (
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sroberts/sous/internal/recipes"
)

// transportOptions are fixed by the plan: no sessions to lose on restart,
// plain JSON responses, and no localhost DNS-rebinding check, because
// Outpost's gateway reaches 127.0.0.1 with the app's public Host.
var transportOptions = &sdk.StreamableHTTPOptions{
	Stateless:                  true,
	JSONResponse:               true,
	DisableLocalhostProtection: true,
}

// New returns the /mcp handler with the ten tools from spec.md §9
// registered. It expects to be wrapped by api.WithIdentity: the caller
// reaches tools via an internal header set from the request context (any
// client-sent copy removed first) and read in tools from req.Extra.Header.
// The transport is sdk.NewStreamableHTTPHandler with transportOptions.
// Tools must set Tool.InputSchema from recipes.SchemaFor: AddTool's own
// inference uses empty ForOptions and would misdescribe Optional fields.
func New(svc *recipes.Service, log *slog.Logger) http.Handler {
	panic("TODO")
}
