# Releasing lega

A release is a `vX.Y.Z` tag on `main`. Everything after the tag is automatic:
GitHub Actions builds the binaries and the `.deb`, writes the release notes,
attests the files and publishes the release. Users get it with `lega update`
or the install script.

## Pick the version

lega follows [Semantic Versioning](https://semver.org). Look at what changed
since the last release (`make release-notes`) and pick the bump:

| Bump | When | Example |
| --- | --- | --- |
| `patch` | only bug fixes | v0.4.1 → v0.4.2 |
| `minor` | new features, nothing breaks | v0.4.1 → v0.5.0 |
| `major` | something users rely on changes or goes away | v0.4.1 → v1.0.0 |

For a test build, give an exact pre-release version instead, such as
`v1.0.0-rc.1`.

## Release

Before you start, make sure everything you want in the release is merged into
`main` and CI on `main` is green.

### From GitHub (easiest)

1. Open **Actions → Release → Run workflow**.
2. Keep the branch on `main`, pick the bump (or type an exact version) and
   press **Run workflow**.

From a terminal, the same thing is:

```sh
make release-gh BUMP=minor
```

### From your machine

```sh
git switch main
git pull
make release BUMP=minor        # or: make release TAG=v1.0.0-rc.1
```

The script checks that `main` is clean, matches GitHub and has passed CI,
runs the tests, shows the release notes and asks before it pushes the tag.
Add `--dry-run` to `scripts/release.sh` to run the checks only.

## What happens next

The **Release** workflow:

1. Tags `main` with the next version (when started from GitHub).
2. Runs the tests on the tagged commit.
3. Writes the notes with `scripts/release-notes.sh`.
4. Builds with GoReleaser: `lega_linux_amd64.tar.gz`,
   `lega_linux_arm64.tar.gz`, a `.deb` for each, and `SHA256SUMS`.
5. Attests every file and publishes the GitHub release.

It takes about two minutes. Follow it under **Actions**. When it is done:

```sh
lega update --check      # on a machine with an older lega
```

## Release notes come from commit messages

`scripts/release-notes.sh` lists the commits since the previous release by
[Conventional Commit](https://www.conventionalcommits.org) type:

| Commit | Section |
| --- | --- |
| `feat(scope)!: ...` (with `!`) | Breaking changes |
| `feat: ...` | New features |
| `fix: ...` | Bug fixes |
| `perf: ...` | Performance |
| `docs`, `ci`, `build`, `chore`, `style`, `test`, `refactor` | left out |

Merge commits are skipped, so the notes show the commits inside each pull
request. If you squash-merge, the pull request title becomes the commit, so
give it a Conventional Commit title too. Write commit subjects for users: `fix(clean): keep Firefox logins`
reads better in the notes than `fix: handle edge case`.

## When something goes wrong

- **CI has not passed**: wait for the CI run on `main` to finish, or fix it,
  then start again. Nothing was tagged.
- **The workflow failed after tagging**: fix the cause on `main`, then delete
  the tag and the draft release if one was made, and release again:

  ```sh
  gh release delete vX.Y.Z -R mydisha/lega --yes   # if it exists
  git push origin :refs/tags/vX.Y.Z
  git tag -d vX.Y.Z
  ```

  Only do this before anyone could have installed the release. After that,
  release a new patch version instead.
- **A broken release is out**: release a fix as the next patch version.
  `lega update` moves everyone forward; it never goes back.
