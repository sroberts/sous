# Sous

[![ci](https://github.com/sroberts/sous/actions/workflows/ci.yml/badge.svg)](https://github.com/sroberts/sous/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/sroberts/sous)](https://github.com/sroberts/sous/releases/latest)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
![ACMM level](.plumbline-badge.svg)

A recipe library your whole household shares, on a computer you own. Save
recipes from any site in a couple of taps, cook from a screen that walks you
through each step with timers, and let an AI assistant search and add recipes
for you. It's one small program with nothing else to install, and everything
it keeps lives in one folder you can back up.

![The Sous library: a want-to-cook rail of tickets above a grid of recipes, with a recipe preview on the right](docs/images/library.png)

- **Import from a link.** Paste a link, or use the bookmarklet or iOS share
  sheet. Sous reads the recipe the page publishes and opens it for you to
  check before anything is saved. Sites that block it fall back to pasting
  the text.
- **Find what to cook.** Browse by course, search by title, tag or
  ingredient, or find recipes that use what you have.
- **Cook mode.** One step at a time in large type, with ingredients to tick
  off, timers that start from the step's own wording, and the screen kept on.
- **Shared, but personal.** One library for the household; favorites and
  want-to-cook are each person's own, every change records who made it, and
  deleted recipes can be restored.
- **Agents built in.** AI assistants such as Claude can use the whole
  library over MCP, with the same rights and the same history as the web UI.
- **Run it your way.** As a service under
  [Outpost](https://github.com/sroberts/outpost), which signs everyone in and
  backs it up, or on its own.

Sous is young: the basics work today, and the [roadmap](#roadmap) lists
what's coming.

## Getting started

### With Outpost

```sh
outpost add sroberts/sous    # latest release, checksum verified, port picked
outpost apply                # a running daemon starts it
```

Outpost gives Sous its port and storage, restarts it if it fails, backs it
up, and lists it in the portal as "Sous". With the login gateway on, Sous
knows who you are from your Outpost sign-in, so there's nothing else to set
up. `outpost status sous` shows where it's running.

### On its own

Download the binary for your machine from the
[latest release](https://github.com/sroberts/sous/releases/latest)
(`sous-darwin-arm64` for Apple silicon Macs, or `sous-linux-amd64` /
`sous-linux-arm64`), then run it:

```sh
gh release download -R sroberts/sous -p sous-darwin-arm64 -p SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
chmod +x sous-darwin-arm64 && mv sous-darwin-arm64 /usr/local/bin/sous
sous --port 8080 --data ~/sous-library
```

Then open http://127.0.0.1:8080. `--data` is where Sous keeps everything
(a SQLite database and a folder of photos) and is created if it's missing.
`sous --help` lists every setting. A binary downloaded with a browser on
macOS is quarantined; `xattr -d com.apple.quarantine sous-darwin-arm64`
clears that.

- **Who's cooking.** There are no accounts. The first time a device makes a
  change, Sous asks who's cooking and remembers the answer on that device.
  It records who did what; it doesn't keep anyone out.
- **Phones and tablets.** Sous only listens on `127.0.0.1`. To reach it from
  another device, put something in front that decides who gets in, such as
  `tailscale serve --bg 8080` for your tailnet.

### Your first library

A new library starts with twelve recipes, from Hannah Glasse's 1751 curry to
a flourless chocolate torte, so there's something to cook straight away.
Delete them and they stay deleted. To bring in your own, choose **Import**
and paste a link, or see [Getting recipes in](docs/importing.md) for the
bookmarklet, the iOS Shortcut and moving from Mela.

![Cook mode: the current step in large type, ingredients ticked off on the left, and an eight-minute timer running](docs/images/cook.png)

<p align="center"><img src="docs/images/phone.png" alt="The library on a phone" width="300"></p>

## Guides

- [Running Sous](docs/running.md): Outpost day to day, the login gateway,
  keeping Sous running on its own, backups and updates.
- [Getting recipes in](docs/importing.md): links, the bookmarklet, the iOS
  Shortcut, and moving from Mela.
- [AI agents and the API](docs/agents.md): connecting MCP clients, and the
  REST API.
- [Developing Sous](docs/development.md): building from source, the web UI,
  and checking the importer against real sites.

## Roadmap

Planned in [`spec.md`](spec.md) §13, not built yet:

- Scaling a recipe by servings, and US ↔ metric conversion
- A weekly meal plan and a combined shopping list grouped by aisle
- Collections, ratings, personal notes and cook history
- An installable app that works offline for recipes you've opened
- Passwords for running Sous on its own ([#6](https://github.com/sroberts/sous/issues/6))

## Help and contributing

[`SUPPORT.md`](SUPPORT.md) says where to ask questions or report a recipe
site that didn't import. [`CONTRIBUTING.md`](CONTRIBUTING.md) has the
workflow and what CI checks; [`spec.md`](spec.md) is the design.

## License

[MIT](LICENSE)
