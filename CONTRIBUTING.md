# Contributing to HotDog CMS

Thank you for helping. Issues, fixes, docs, sections and packs are all
welcome.

## Sending a change

New accounts on git.coffeylabs.org can file issues but can't push or fork.
Until you have repository access, open an
[issue](https://git.coffeylabs.org/coffey-labs/hotdog-cms/issues) for the change, or say hello
on [community.coffeylabs.org](https://community.coffeylabs.org), and attach the
change to the issue as a patch:

```sh
git format-patch origin/main --stdout > my-change.patch
```

Repository access goes to volunteers we have worked with; a few good issues or
patches is the usual way in. After that you push a branch here and open a pull
request. The GitHub copy is a read-only mirror, so pull requests opened
there are not merged.

## Before your first pull request

Accept the [contributor agreement](CLA.md): add yourself to
[CONTRIBUTORS.md](CONTRIBUTORS.md) in a signed-off commit, as CLA.md
describes. You keep the copyright in your work. Coffey Labs gets a license
to use it, and promises to keep it available as open source. Maintainers
check this with `tools/cla-check.sh` before merging.

## Building and testing

    go build -o bin/hotdog-cms ./cmd/hotdog-cms
    go vet ./... && go test ./...

    cd editor-ui
    npm ci
    npx tsc --noEmit -p . && npm test
    npm run build       # into internal/editor/ui/dist, embedded by go build

Run all of it before you push, and say in the pull request what you tried
by hand.

## What we look for

- **Security and ease of use together.** HotDog CMS exists to be as easy as
  WordPress without its security problems. If a change seems to need one at
  the cost of the other, raise it in an issue first: there's usually a
  design that gives both.
- **The editor never lives on the site's host,** and changes reach a site
  only as git commits. The editor holds no publishing credentials.
- **Packs carry no code.** See [docs/packs.md](docs/packs.md).
- **Dependencies:** as few as possible, and only releases at least two weeks
  old. npm packages are pinned exactly (`npm install --save-exact`).
- **English only.** The editor and the starter aren't translated.
- **Plain words** in the interface and the docs: say what happens, in the
  terms the person using it would use.
- Code reads like the code around it. Tests come with the change.

## Licenses

The engine and the editor are AGPL-3.0-or-later. The starter site in
`starter/` and HotDog themes and packs are MIT. A contribution is under the
license of the part it changes.

## Security problems

Don't open a public issue. Email security@coffeylabs.org.
