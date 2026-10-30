## What changed and why

<!-- What this PR does, and the reasoning a reviewer can't get from the diff. Link the issue it closes. -->

## How it was tested

<!-- What you ran and what it showed: tests, a browser at desktop and 390px, a real Outpost, a live import. -->

## Not verified

<!-- What nobody has actually run. "Nothing" is a valid answer only if it's true. -->

## Checklist

- [ ] Branched from an up-to-date `main`; nothing committed to `main` directly
- [ ] CI passes: svelte-check, gofmt, go vet, and `go test -race ./...`
- [ ] No existing test or golden file changed, or the maintainer approved the change
- [ ] New behaviour has tests
- [ ] Works both under Outpost and on its own, where it applies
- [ ] UI changes checked at phone width (390px) and on desktop
- [ ] `README.md` and `spec.md` updated if behaviour or configuration changed
- [ ] No personal data (`*.melarecipes`, `web/mock-local/`), secrets or whole third-party pages
