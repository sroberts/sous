# Changelog

What changed for people running Sous. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Sous uses
[semantic versioning](https://semver.org/): before 1.0, a minor release may
include breaking changes, and they're called out here.

## [Unreleased]

### Added
- `POST /v1/recipes/import` reads a recipe link and saves it in one call,
  returning a short `summary` to speak. A link already in Sous returns the
  existing recipe, restoring it from the trash if needed.
- A "Save to Sous" Siri Shortcut, with setup steps on the `/import` page, to
  save a recipe from your phone without opening Sous.
- `SECURITY.md`, `SUPPORT.md`, and issue forms for feedback, import
  failures, bugs and feature requests.

## [0.2.0] - 2026-10-30

### Added
- Under Outpost's login gateway, Sous uses your Outpost sign-in: no
  "who's cooking?" prompt, and your name links back to Outpost for your
  account, signing out and other apps.
- A notice when your Outpost sign-in ends while a page is open, with a way
  to sign in again.
- `--data DIR` to run Sous on its own without Outpost's environment
  variables, plus `--help` and `--version`.
- The Outpost portal lists Sous as "Sous · Household recipes".

### Fixed
- A long website name on a recipe card made the library scroll sideways on
  phones.

## [0.1.0] - 2026-10-29

First release.

### Added
- The recipe library, recipe pages, cook mode with step timers, and the
  editor, in the Ticket Rail design.
- Import a recipe from a link, with a bookmarklet and an iOS Shortcut, and a
  paste fallback for sites that block it.
- REST API with an OpenAPI description, and MCP tools for AI agents.
- Running under Outpost, with its login gateway.
- A script to import a Mela export.

[Unreleased]: https://github.com/sroberts/sous/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/sroberts/sous/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/sroberts/sous/releases/tag/v0.1.0
