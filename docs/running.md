# Installing and running Sous

Sous is one program. Run it under [Outpost](https://github.com/sroberts/outpost),
which signs everyone in, restarts it and backs it up, or run it on its own.

## Install

Each [release](https://github.com/sroberts/sous/releases) has a binary for
Apple silicon Macs (`sous-darwin-arm64`) and Linux (`sous-linux-amd64`,
`sous-linux-arm64`), a `SHA256SUMS` file, and the `outpost.toml` Outpost
reads. Under Outpost, `outpost add` downloads and verifies it for you (below).
On your own:

```sh
gh release download -R sroberts/sous -p sous-darwin-arm64 -p SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
chmod +x sous-darwin-arm64 && mv sous-darwin-arm64 /usr/local/bin/sous
sous --version
```

A binary downloaded with a browser on macOS is quarantined, and macOS
refuses to open it; `xattr -d com.apple.quarantine sous-darwin-arm64` clears
that. `gh` and `curl` downloads aren't quarantined.

## Run it with Outpost

```sh
outpost add sroberts/sous    # latest release, checksum verified, port picked
outpost apply                # a running daemon starts it
```

Outpost gives Sous its port and storage directory, restarts it if it fails,
and lists it in the portal as "Sous". Day to day:

```sh
outpost status sous
outpost logs sous -f
outpost backup sous          # the whole library: database and photos
outpost update sous          # newer release; rolls back if it won't start
```

**With the login gateway on**, Sous takes who you are from your Outpost
sign-in. There's no "who's cooking?" prompt and no switching people; your
name in the top bar links back to Outpost for your account, signing out, and
your other apps. If your sign-in ends while a page is open, Sous says so and
offers to sign you in again. A person's username in Outpost is their identity
in Sous, so use the same one if you move a library over.

**Without the gateway**, Sous behaves as it does on its own (next section):
each device picks who's cooking.

## Run it on its own

```sh
sous --port 8080 --data ~/sous-library
```

Then open http://127.0.0.1:8080. `--data` is where Sous keeps everything
(a SQLite database and a folder of photos) and is created if it's missing.
`sous --help` lists every setting.

- **First run.** A new Sous comes with twelve recipes, credited to "Sous".
  Seven are historic, from Hannah Glasse's 1751 curry to Escoffier's 1907
  Peach Melba, each with a modern adaptation, the original text and a period
  illustration, and tagged Historic. Five are classics written for Sous and
  tagged Classic: boeuf bourguignon, poulet au vinaigre, steak frites and a
  French omelette, inspired by Julia Child, Paul Bocuse, Anthony Bourdain and
  Jacques Pépin, and a flourless chocolate torte. Delete them and they stay
  deleted. Sous only adds them to a library that has never been used, so a
  restore that brings recipes or history never gets them (restoring a
  never-used library can), and `SOUS_SAMPLES=false` skips them on that start.
  This applies under Outpost too.
- **Who's cooking.** There are no accounts. The first time a device makes a
  change, Sous asks who's cooking and remembers the answer on that device.
  Anyone who can reach Sous can pick any name; it records who did what, it
  doesn't keep anyone out. Passwords are planned in
  [#6](https://github.com/sroberts/sous/issues/6).
- **Other devices.** Sous only listens on `127.0.0.1`, so only this machine
  can reach it. To use it from a phone or tablet, put something in front that
  you trust to decide who gets in, such as
  `tailscale serve --bg 8080` for your tailnet, or a reverse proxy with its
  own login.
- **Keeping it running.** Sous runs in the foreground and stops cleanly on
  Ctrl-C or SIGTERM, so any supervisor works: a launchd agent, a systemd
  unit, or `tmux`.
- **Backups.** Stop Sous and copy the `--data` directory. While it runs,
  `sqlite3 sous.db ".backup sous-backup.db"` copies the database safely; copy
  the `images` folder alongside it.
- **Updating.** Replace the binary and restart. Sous upgrades its database
  on start.
