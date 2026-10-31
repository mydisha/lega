<div align="center">

# lega

**Free up disk space on Ubuntu, Linux Mint, Zorin OS and Pop!_OS from a friendly terminal UI.**

Clean caches and junk safely, see what fills your disk, and clear out old build folders.

[![CI](https://github.com/mydisha/lega/actions/workflows/ci.yml/badge.svg)](https://github.com/mydisha/lega/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/mydisha/lega?sort=semver&label=release)](https://github.com/mydisha/lega/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/mydisha/lega?logo=go&logoColor=white&label=Go)](go.mod)
[![Built with Bubble Tea](https://img.shields.io/badge/TUI-Bubble%20Tea-ec6a2c)](https://github.com/charmbracelet/bubbletea)
[![Platform](https://img.shields.io/badge/platform-Ubuntu%20%7C%20Mint%20%7C%20Zorin%20%7C%20Pop!__OS-E95420?logo=ubuntu&logoColor=white)](#supported-systems)
[![Go Report Card](https://goreportcard.com/badge/github.com/mydisha/lega)](https://goreportcard.com/report/github.com/mydisha/lega)
[![License: MIT](https://img.shields.io/github/license/mydisha/lega)](LICENSE)

</div>

```text
Clean

System
▸ [x] Thumbnail cache                                                   432.0 MB
Browsers
  [x] Firefox cache                                                       1.4 GB
  [-] Google Chrome cache · locked (2)                                    1.1 GB
Developer tools
  [x] npm cache                                                           2.6 GB
  [x] Yarn cache                                                        406.9 MB
  [x] pip cache                                                         766.5 MB
  [x] Go build cache                                                      3.3 GB
  [x] VS Code caches                                                    311.4 MB
  [ ] Docker build cache · review                                        18.8 GB
Apps
  [x] Chat app caches (2)                                               750.8 MB
Files
  [ ] Trash · review (2)                                                  2.0 GB

Image previews made by the file manager. They are made again when needed.
Selected: 12.0 GB in 11 items

space select • a all/none • → details • enter continue • q back
```

*Lega* is Indonesian for the feeling of relief, and for a room with space to
breathe. That is what your disk gets. The project is inspired by
[Mole](https://github.com/tw93/mole), which does the same job on macOS.

## What it does

- **Clean** removes caches, logs, old packages and other junk that is safe to
  delete: browsers, developer tools, chat apps, the Trash, APT, the journal and
  old snaps. You see every item and its size before anything goes.
- **Analyze** lets you walk your folders sorted by size, like `ncdu`. Jump to
  the largest files, open a folder in Files, or send something to the Trash.
- **Status** shows how full each disk is and which folders fill your home.
- **Purge** finds `node_modules`, `target`, `.venv` and other build folders in
  your projects that you can rebuild any time.
- **Stays safe.** Personal files, settings, logins and history are never
  touched. Riskier items start unselected. Every removal is logged.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mydisha/lega/main/install.sh | bash
```

The script picks the build for your CPU (amd64 or arm64), checks it against
the release checksums and puts `lega` in `~/.local/bin`. No sudo needed.

<details>
<summary>Other ways to install</summary>

Install a given version, or into another folder:

```sh
curl -fsSL https://raw.githubusercontent.com/mydisha/lega/main/install.sh | bash -s -- v0.1.0
curl -fsSL https://raw.githubusercontent.com/mydisha/lega/main/install.sh | sudo bash -s -- --prefix /usr/local/bin
```

As a `.deb` package, from the [releases page](https://github.com/mydisha/lega/releases/latest):

```sh
sudo apt install ./lega_*_amd64.deb
```

With Go 1.26 or newer:

```sh
go install github.com/mydisha/lega/cmd/lega@latest
```

Every release file is listed in `SHA256SUMS` and carries a build provenance
attestation. To check where a download came from:

```sh
gh attestation verify lega_linux_amd64.tar.gz -R mydisha/lega
```

</details>

### Update and uninstall

```sh
lega update           # install the latest release
lega update --check   # only tell me if there is one
lega remove           # uninstall lega, its keep list and its log
```

If you installed the `.deb`, update and remove it with `apt` instead.

## Use

```sh
lega              # menu
lega clean        # pick junk to remove
lega analyze      # explore your home folder by size
lega analyze /var # ... or any other folder
lega status       # disks and what fills your home folder
lega purge        # remove build folders from your projects
```

Run lega as your normal user. When you pick system items such as the APT
cache or the journal, it asks for your sudo password once, right before it
cleans them.

<details>
<summary>The analyzer</summary>

```text
Analyze  ~
32.9 GB · 23 files · sorted by size

▸    7.4 GB   23%  ████░░░░░░░░░░░░  Videos/
     7.2 GB   22%  ███░░░░░░░░░░░░░  .cache/
     6.2 GB   19%  ███░░░░░░░░░░░░░  Downloads/
     3.5 GB   11%  ██░░░░░░░░░░░░░░  Pictures/
     2.7 GB    8%  █░░░░░░░░░░░░░░░  Projects/
     2.6 GB    8%  █░░░░░░░░░░░░░░░  .npm/
     2.0 GB    6%  █░░░░░░░░░░░░░░░  .local/
     1.3 GB    4%  █░░░░░░░░░░░░░░░  .config/
    42.0 MB    0%  █░░░░░░░░░░░░░░░  Documents/

→ open • ← up • s sort • f largest files • d trash • o show in Files • q back
```

</details>

<details>
<summary>Purge</summary>

```text
Purge project build folders
In ~/Projects

▸ [x]    1.7 GB  ~/Projects/api/target
  [x]  985.7 MB  ~/Projects/web-app/node_modules

Folders used in the last 7 days start unselected.
Selected: 2.7 GB in 2 folders

space select • a all/none • enter continue • q back
```

</details>

The menu opens with a short summary of your machine: host name, OS, kernel,
CPU and memory.

### Icons

lega can show icons next to folders, file types and apps (Firefox, Chrome,
VS Code, npm, Docker and others). They need a [Nerd Font](https://www.nerdfonts.com)
set as your terminal font, so they are off by default:

```sh
lega --icons                          # once
export LEGA_ICONS=nerd                # always, e.g. in ~/.bashrc
```

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

Every command also works without the interface:

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

Items marked "review" are listed but start unselected. Look at them before you
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

- Every path is checked right before it is deleted: it must sit inside the
  folder its rule cleans, symlinks are never followed out of that folder, and
  mount points and files owned by other users are skipped.
- Your home folder, `~/.config`, `~/.local`, your personal folders (Documents,
  Pictures, ...) and system folders are never deleted. Nothing inside `~/.ssh`,
  `~/.gnupg` or your keyrings is touched.
- Browser and editor caches are locked while the app is running.
- `analyze` moves things to the Trash. It only deletes for good when a drive
  has no Trash, and asks first.
- `purge` skips build folders that git tracks and leaves folders used in the
  last 7 days unselected.
- System items run through `sudo lega _root-clean <item>`. Only item names
  cross over to root; the root side finds the files again by itself, with the
  same checks.

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

## Supported systems

Ubuntu and the distros built on it: Linux Mint, Zorin OS, Pop!_OS, elementary
OS and KDE neon, plus Debian. Release builds are for amd64 and arm64.

On other distros lega still runs. It shows a warning and hides the system
items whose tools are missing.

## Good to know

- **Does lega go online?** Only when you run `lega update` or the install
  script. Nothing else connects anywhere.
- **Why do sizes differ from `du -h`?** lega counts like Files and Disks do:
  1 GB is 1,000,000,000 bytes. `du -h` counts in GiB.
- **Can I undo a clean?** No. `clean` and `purge` delete for good, which is why
  they only pick data that apps rebuild by themselves. `analyze` uses the
  Trash, so you can restore from there.

## Build from source

```sh
git clone https://github.com/mydisha/lega
cd lega
make test
make build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) to add a cleaning rule or cut a release.

## License

[MIT](LICENSE)
