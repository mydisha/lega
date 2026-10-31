# lega

Free up disk space on Ubuntu and its derivatives (Linux Mint, Zorin OS, Pop!_OS,
elementary OS, KDE neon, Debian) from an interactive terminal UI.

lega finds caches, logs, old packages and build folders that are safe to delete,
shows how big they are, and removes only what you pick. It also shows how full
your disks are and lets you explore which folders use the most space.

Inspired by [Mole](https://github.com/tw93/mole), which does the same on macOS.

## Install

With Go 1.26 or newer:

```sh
go install github.com/mydisha/lega/cmd/lega@latest
```

Or download a `.deb` or `.tar.gz` from the
[releases page](https://github.com/mydisha/lega/releases):

```sh
sudo apt install ./lega_*_amd64.deb
```

## Use

```sh
lega              # menu
lega status       # disks and what fills your home folder
lega clean        # pick junk to remove
lega analyze      # explore your home folder by size
lega analyze /var # ... or any other folder
lega purge        # remove node_modules, target, .venv and other build folders
```

Run lega as your normal user. When you pick system items (APT cache, journal,
old snaps), it asks for your sudo password once, right before cleaning them.

### Keys

| Key | Action |
| --- | --- |
| `↑` `↓` / `j` `k` | move |
| `space` | select or unselect |
| `a` | select all or none |
| `→` / `l` | open a folder, or show the files of a clean item |
| `←` / `h` | go up, or fold |
| `enter` | continue |
| `f` | (analyze) largest files in this folder |
| `s` | (analyze) sort by size, name or file count |
| `d` | (analyze) move to the Trash |
| `o` | (analyze) show in the file manager |
| `q` / `esc` | back |

### Scripts and pipes

```sh
lega clean --dry-run          # list what would be removed
lega clean --yes              # remove the preselected user items
lega clean --yes --system     # ... and the preselected system items (asks for sudo)
lega purge --dry-run ~/code   # list build folders under ~/code
lega purge --yes              # remove build folders not used in the last 7 days
lega status --json
lega analyze --json ~/Downloads
```

## What clean removes

Items marked "review" are listed but start unselected. Check them before you
select them.

| Group | Item | Default |
| --- | --- | --- |
| System | Thumbnail cache, old desktop session logs | selected |
| Browsers | Cache of Firefox (deb, snap, Flatpak), Chrome, Chromium, Brave, Edge, Vivaldi, Opera. Logins, history and bookmarks are kept | selected |
| Developer tools | npm, Yarn, pnpm (`pnpm store prune`), pip, Go build cache, Composer, Bun, Deno, VS Code / VSCodium / Cursor caches | selected |
| Developer tools | Go module cache, Cargo registry, Gradle, Maven, JetBrains caches, Docker build cache (`docker builder prune`) | review |
| Apps | Slack, Discord and Teams caches | selected |
| Apps | Snap and Flatpak app caches, everything else in `~/.cache` | review |
| Files | Trash (also on mounted drives), installers in Downloads older than 30 days | review |
| System, needs sudo | APT package cache (`apt-get clean`), journal older than 14 days, rotated logs in `/var/log`, crash reports and core dumps, old snap versions | selected |
| System, needs sudo | Unused packages and old kernels (`apt-get autoremove --purge`), unused Flatpak runtimes | review |

`lega purge` looks in `~/Projects`, `~/code`, `~/dev`, `~/src`, `~/www`,
`~/workspace` and similar folders for `node_modules`, `target`, `.venv`,
`venv`, `__pycache__`, `.next`, `.nuxt`, `.svelte-kit`, `.turbo`, `.gradle`,
`.dart_tool`, Python tool caches, and `build` or `dist` next to a project file.

## Safety

- Every path is checked before it is deleted: it must sit inside the folder its
  rule cleans, symlinks are never followed out of that folder, mount points
  and files owned by other users are skipped.
- Your home folder, `~/.config`, `~/.local`, your personal folders (Documents,
  Pictures, ...) and system folders are never deleted. Nothing inside `~/.ssh`,
  `~/.gnupg` or your keyrings is touched.
- Browser and editor caches are locked while the app is running.
- `analyze` moves things to the Trash. It only deletes for good when a drive has
  no Trash, and asks first.
- `purge` skips build folders that git tracks and leaves folders used in the
  last 7 days unselected.
- System items run through `sudo lega _root-clean <item>`. Only item names cross
  over to root; the root side finds the files again by itself, with the same
  checks.

### Keep list

List paths or glob patterns that lega must never remove, one per line, in
`~/.config/lega/whitelist`:

```
# keep JetBrains indexes
~/.cache/JetBrains
/var/log/*.gz
```

If this file exists but cannot be read, `clean` and `purge` stop instead of
running without it.

### Log

Every removal is written to `~/.local/state/lega/operations.log` (time, action,
item, bytes, result, path). Set `LEGA_NO_OPLOG=1` to turn it off.

## Build from source

```sh
git clone https://github.com/mydisha/lega
cd lega
go test ./...
go build -o lega ./cmd/lega
```

See [CONTRIBUTING.md](CONTRIBUTING.md) to add a cleaning rule.

## License

[MIT](LICENSE)
