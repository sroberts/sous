# Security policy

## Supported versions

Only the [latest release](https://github.com/sroberts/sous/releases/latest)
receives security fixes. Sous is before 1.0, so fixes ship in a new release
rather than as patches to older ones.

## Reporting a vulnerability

Please **don't** open a public issue for security problems.

Report it privately through
[GitHub's private vulnerability reporting](https://github.com/sroberts/sous/security/advisories/new).
Include what you found, how to reproduce it, and the impact you expect.
Sous has one maintainer; you'll get a reply there, and updates until it's
fixed and disclosed.

## What's in scope

The areas where a bug could hurt someone running Sous:

- **Fetching pages for import** (`internal/safefetch`): Sous fetches links
  people give it, and must never reach private or local addresses, or the
  machine it runs on.
- **Saving links without a browser** (`POST /v1/recipes/import`): it fetches
  and saves arbitrary links in one call, through the same `safefetch` guard.
  Under Outpost it is reached with an Outpost access token, which Outpost
  checks before Sous sees the request; on its own it relies on the household
  trust model below. Rate limiting is deliberately not built: every call needs
  a person, the same fetch is open through preview and MCP, the importer caps
  concurrency and time, and Outpost throttles rejected tokens.
- **Identity** (`internal/identity`): verifying the Outpost gateway's signed
  token, and never letting a request act as someone else when the gateway is
  in use.
- **Storage and uploads**: images, the SQLite database, and anything that
  could write outside the storage directory.
- **The web UI**: script injection through recipe content or imported pages.

Running Sous on its own has no passwords yet
([#6](https://github.com/sroberts/sous/issues/6)), so anyone who can reach
it can act as anyone. That's documented behaviour, not a vulnerability.
