#!/usr/bin/env bash
# Tag a release and push the tag. The Release workflow then builds the
# binaries, the .deb and SHA256SUMS and publishes them on GitHub.
#
#   scripts/release.sh v0.2.0            check, tag and push
#   scripts/release.sh v0.2.0 --dry-run  only run the checks
#
# Set RELEASE_SKIP_CI=1 to skip the CI status check (needs gh otherwise).
set -euo pipefail

repo=mydisha/lega
version=""
dry=0

die() {
	printf 'release: %s\n' "$*" >&2
	exit 1
}

for arg in "$@"; do
	case "$arg" in
	--dry-run) dry=1 ;;
	-h | --help)
		sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	-*) die "unknown flag: $arg" ;;
	*)
		[ -z "$version" ] || die "give one version"
		version="$arg"
		;;
	esac
done
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
	die "usage: scripts/release.sh vX.Y.Z [--dry-run]"

cd "$(git rev-parse --show-toplevel)"
[ "$(git branch --show-current)" = main ] || die "switch to main first"
[ -z "$(git status --porcelain)" ] || die "the working tree has changes; commit them first"
git fetch --quiet --tags origin main
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] ||
	die "main differs from origin/main; push or pull first"
if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
	die "$version already exists"
fi

# Versions only go up. Pre-releases are left out of this check because
# sort -V puts v1.0.0-rc.1 after v1.0.0.
last=$(git tag --list 'v*' | grep -v -- - | sort -V | tail -n 1 || true)
if [ -n "$last" ] && [[ "$version" != *-* ]]; then
	top=$(printf '%s\n%s\n' "$last" "$version" | sort -V | tail -n 1)
	[ "$top" = "$version" ] || die "$version is older than $last"
fi

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

if [ "$dry" -eq 1 ]; then
	printf 'All checks passed. %s was not tagged (dry run).\n' "$version"
	exit 0
fi
git tag -a "$version" -m "lega $version"
git push origin "$version"
printf 'Pushed %s. Follow the build at https://github.com/%s/actions\n' "$version" "$repo"
