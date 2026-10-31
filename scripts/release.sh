#!/usr/bin/env bash
# Release lega from your machine: check main, show the release notes, then
# tag and push. The Release workflow builds and publishes the rest.
#
#   scripts/release.sh minor            next version from the latest tag
#   scripts/release.sh v1.0.0-rc.1      an exact version
#   scripts/release.sh patch --dry-run  only run the checks
#   scripts/release.sh minor --yes      do not ask before tagging
#
# Set RELEASE_SKIP_CI=1 to skip the CI status check (needs gh otherwise).
set -euo pipefail

repo=mydisha/lega
want=""
dry=0
yes=0

die() {
	printf 'release: %s\n' "$*" >&2
	exit 1
}

for arg in "$@"; do
	case "$arg" in
	--dry-run) dry=1 ;;
	--yes) yes=1 ;;
	-h | --help)
		sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	-*) die "unknown flag: $arg" ;;
	*)
		[ -z "$want" ] || die "give one version or bump"
		want="$arg"
		;;
	esac
done
[ -n "$want" ] || die "usage: scripts/release.sh patch|minor|major|vX.Y.Z [--dry-run] [--yes]"

cd "$(git rev-parse --show-toplevel)"
[ "$(git branch --show-current)" = main ] || die "switch to main first"
[ -z "$(git status --porcelain)" ] || die "the working tree has changes; commit them first"
git fetch --quiet --tags origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] ||
	die "main differs from origin/main; push or pull first"
version=$(scripts/next-version.sh "$want")

sha=$(git rev-parse HEAD)
if [ "${RELEASE_SKIP_CI:-0}" != 1 ]; then
	command -v gh >/dev/null 2>&1 || die "gh is needed to check CI (or set RELEASE_SKIP_CI=1)"
	result=$(gh run list -R "$repo" --workflow CI --commit "$sha" --limit 1 \
		--json status,conclusion --jq '.[] | "\(.status) \(.conclusion)"' 2>/dev/null || true)
	[ "$result" = "completed success" ] ||
		die "CI has not passed on ${sha:0:7} (got: ${result:-no run}); see https://github.com/$repo/actions"
fi

go vet ./...
go test ./...

printf '\n----- release notes for %s -----\n\n' "$version"
scripts/release-notes.sh
printf '\n-----\n\n'

if [ "$dry" -eq 1 ]; then
	printf 'All checks passed. %s was not tagged (dry run).\n' "$version"
	exit 0
fi
if [ "$yes" -ne 1 ]; then
	[ -t 0 ] || die "no terminal to confirm on; add --yes"
	printf 'Tag %s on %s and publish it? [y/N] ' "$version" "${sha:0:7}"
	read -r answer
	case "$answer" in
	y | Y | yes | YES) ;;
	*) die "nothing was tagged" ;;
	esac
fi
git tag -a "$version" -m "lega $version"
git push origin "$version"
printf 'Pushed %s. Follow the build at https://github.com/%s/actions\n' "$version" "$repo"
