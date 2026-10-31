# Contributing to lega

Thanks for helping. Bug reports, new cleaning rules and fixes for other
Ubuntu-based distros are all welcome.

## Setup

You need Go 1.26 or newer.

```sh
make test    # go vet and go test -race
make build   # ./lega, with the version from git
make lint    # golangci-lint and shellcheck, if you have them
```

To try the interface without touching your real files, point it at a fake home:

```sh
mkdir -p /tmp/fakehome/.cache/pip
HOME=/tmp/fakehome ./lega clean
```

## Project layout

| Folder | What it does |
| --- | --- |
| `cmd/lega` | Command line: subcommands, flags, plain and JSON output |
| `internal/clean` | Cleaning rules and the engine that scans and runs them |
| `internal/purge` | Finds build folders in projects |
| `internal/safety` | The guard every deletion goes through, and the keep list |
| `internal/scan` | Measures folder sizes |
| `internal/disk` | Lists mounted disks |
| `internal/trash` | Moves files to the desktop Trash |
| `internal/oplog` | The operations log |
| `internal/platform` | Distro, user folders and running processes |
| `internal/update` | `lega update`, and the release layout `install.sh` relies on |
| `internal/ui` | The terminal interface |

## Adding a cleaning rule

Most rules are a `pathRule`: a folder plus glob patterns of what to remove
inside it. Add one to the right file in `internal/clean` (`rules_system.go`,
`rules_browser.go`, `rules_dev.go` or `rules_root.go`):

```go
&pathRule{
	info: Info{
		ID: "example", Name: "Example cache", Group: GroupDev,
		Processes:   []string{"example"}, // locked while this runs
		Description: "What it is, and what happens after it is removed.",
	},
	spots: []spot{{"$CACHE/example", []string{"*"}}},
},
```

When a tool has its own cleanup command, use a `commandRule` instead, so the
tool stays in charge of its files.

A rule must:

- Remove only data the app rebuilds by itself, or be marked `Risk: Review`.
- Never match settings, profiles, cookies, history or anything the user made.
- Clean the contents of a cache folder, not the folder itself (use `/*`).
- Have a test that builds a fake home with `testContext`, checks that the
  junk goes, and checks that the files next to it stay.

Rules run from specific to general. If your path is also inside `~/.cache`,
put your rule before `generalRules()` so it wins the overlap.

## Code style

- Run `gofmt`. CI also runs `golangci-lint`.
- Comments are short and plain. Every exported name gets a one or two line
  doc comment. Inside functions, comment only the *why* that the code cannot
  show.
- Every bug fix comes with a test that fails without the fix.

## Commits and pull requests

- Use [Conventional Commits](https://www.conventionalcommits.org):
  `feat(clean): add Yarn berry cache`, `fix(ui): ...`, `docs: ...`.
- Keep one change per commit, in an order that builds and passes tests at
  every step.
- In the pull request, say which distro and version you tested on.

## Releasing

Maintainers release from a green `main`, either with **Actions → Release →
Run workflow** on GitHub or with `make release BUMP=minor`. The version,
release notes and builds are all automatic. See
[docs/RELEASING.md](docs/RELEASING.md) for the steps and what to do when a
release fails.
