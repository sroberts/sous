# AI agents and the API

## MCP

MCP clients connect to `/mcp` on the same port, with the same tools as the
REST API, including `import_recipe`. Every change is attributed to a person:

- **On its own:** `http://127.0.0.1:8080/mcp`, with an `X-Sous-Person:
  <username>` header naming who the agent acts for.
- **Under Outpost:** connect on loopback (Outpost assigns the port; see
  `outpost status sous`) with the same header. The gateway itself only lets
  in people signed in with a browser, and `SOUS_REQUIRE_GATEWAY=true` closes
  the loopback route too, so leave it unset if agents need MCP.
- **Access tokens:** the gateway also accepts an Outpost access token
  (`Authorization: Bearer opat_…`) and passes Sous the same signed identity as
  a login. Scripts and the Siri Shortcut use this to reach Sous from outside.
  MCP still connects on loopback, as above.

The tools are listed in [`spec.md`](../spec.md) §9.

## REST API

The REST API is described at `/v1/openapi.json` on a running Sous, and in
[`spec.md`](../spec.md) §8. Writes name a person the same way as MCP.
