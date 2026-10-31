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

## Quick start

On a Mac with Apple silicon (Linux binaries are on the
[releases page](https://github.com/sroberts/sous/releases/latest)):

```sh
gh release download -R sroberts/sous -p sous-darwin-arm64
chmod +x sous-darwin-arm64
./sous-darwin-arm64 --port 8080 --data ~/sous-library   # then open http://127.0.0.1:8080
```

A new library starts with twelve recipes, from Hannah Glasse's 1751 curry to
a flourless chocolate torte, so there's something to cook straight away.
Delete them and they stay deleted.

With [Outpost](https://github.com/sroberts/outpost), it's two commands, and
Outpost signs everyone in, keeps Sous running and backs it up:

```sh
outpost add sroberts/sous
outpost apply
```

Sous is young: the basics work today, and the [roadmap](#roadmap) lists
what's coming.

![Cook mode: the current step in large type, ingredients ticked off on the left, and an eight-minute timer running](docs/images/cook.png)

<p align="center"><img src="docs/images/phone.png" alt="The library on a phone" width="300"></p>

## Guides

- [Installing and running Sous](docs/running.md): checking the download,
  Outpost, running it on its own, other devices, backups and updates.
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
