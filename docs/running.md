# Running Sous

The [README](../README.md#getting-started) covers installing Sous and
starting it, with Outpost or on its own. This page is what comes after.

## Under Outpost

Day to day:

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

**Without the gateway**, Sous behaves as it does on its own: each device
picks who's cooking.

## On its own

- **Who's cooking.** Anyone who can reach Sous can pick any name; it records
  who did what, it doesn't keep anyone out. Passwords are planned in
  [#6](https://github.com/sroberts/sous/issues/6).
- **Other devices.** Sous only listens on `127.0.0.1`, so only this machine
  can reach it. Put something in front that you trust to decide who gets in,
  such as `tailscale serve --bg 8080` for your tailnet, or a reverse proxy
  with its own login.
- **Keeping it running.** Sous runs in the foreground and stops cleanly on
  Ctrl-C or SIGTERM, so any supervisor works: a launchd agent, a systemd
  unit, or `tmux`.
- **Backups.** Stop Sous and copy the `--data` directory. While it runs,
  `sqlite3 sous.db ".backup sous-backup.db"` copies the database safely; copy
  the `images` folder alongside it.
- **Updating.** Replace the binary and restart. Sous upgrades its database
  on start.

## On your phone or tablet

Sous is an installable app. Once installed, it opens from your home screen
like any other app, and the library, recipes and Cook mode keep working
without a connection.

- **Android (Chrome).** Open Sous, then menu, then Install app.
- **iPhone and iPad (Safari).** Open Sous, tap Share, then Add to Home Screen.

Installing needs HTTPS. Under Outpost, `tailscale serve` gives you that. A
plain `http://` tailnet address opens Sous fine but gets no offline support,
because browsers only allow it on HTTPS and on `127.0.0.1`.

**What works offline.** The first time Sous opens with a connection, it saves
the whole library's text to the device, and tops it up in the background
whenever you're online. Photos are saved as you look at them, up to about
200 MB. Offline you can search, read recipes, and cook with timers and
check-offs. You can't save, favorite, import or add a person until you're
back online; Sous says so where it matters.

**Under the Outpost gateway.** If your sign-in lapses while you're away, Sous
still opens the saved library, read-only, with a "Sign in again" banner.
Anything you change needs you to sign in first.

**Clearing it.** Clear the site's data in the browser (or remove the app) and
the saved library goes with it. Sous saves it again next time it opens with a
connection.

## The starter recipes

A new Sous comes with twelve recipes, credited to "Sous", with Outpost or on
its own. Seven are historic, from Hannah Glasse's 1751 curry to Escoffier's
1907 Peach Melba, each with a modern adaptation, the original text and a
period illustration, and tagged Historic. Five are classics written for Sous
and tagged Classic: boeuf bourguignon, poulet au vinaigre, steak frites and a
French omelette, inspired by Julia Child, Paul Bocuse, Anthony Bourdain and
Jacques Pépin, and a flourless chocolate torte.

Delete them and they stay deleted. Sous only adds them to a library that has
never been used, so a restore that brings recipes or history never gets them
(restoring a never-used library can), and `SOUS_SAMPLES=false` skips them on
that start.
