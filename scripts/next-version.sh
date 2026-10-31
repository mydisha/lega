#!/usr/bin/env bash
# Print the tag of the next release.
#
#   scripts/next-version.sh patch    v0.4.1 -> v0.4.2  (bug fixes only)
#   scripts/next-version.sh minor    v0.4.1 -> v0.5.0  (new features)
#   scripts/next-version.sh major    v0.4.1 -> v1.0.0  (breaking changes)
#   scripts/next-version.sh v1.0.0-rc.1                (an exact version)
#
# The latest release is the highest vX.Y.Z tag, or v0.0.0 when there is none.
# Fails when the result already exists or is not newer than the latest.
set -euo pipefail

die() {
	printf 'next-version: %s\n' "$*" >&2
	exit 1
}

re='^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z.-]+)?$'
[ $# -eq 1 ] || die "usage: scripts/next-version.sh patch|minor|major|vX.Y.Z"

latest=$(git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1 || true)
latest=${latest:-v0.0.0}
[[ "$latest" =~ $re ]]
major=${BASH_REMATCH[1]}
minor=${BASH_REMATCH[2]}
patch=${BASH_REMATCH[3]}

case "$1" in
patch) next="v$major.$minor.$((patch + 1))" ;;
minor) next="v$major.$((minor + 1)).0" ;;
major) next="v$((major + 1)).0.0" ;;
v[0-9]*) next="$1" ;;
[0-9]*) next="v$1" ;;
*) die "expected patch, minor, major or a version, got: $1" ;;
esac

[[ "$next" =~ $re ]] || die "not a valid version: $next"
if git rev-parse -q --verify "refs/tags/$next" >/dev/null; then
	die "$next already exists"
fi
# v1.0.0-rc.1 counts as v1.0.0 here: it must come after the latest release.
base=${next%%-*}
top=$(printf '%s\n%s\n' "$latest" "$base" | sort -V | tail -n 1)
if [ "$base" = "$latest" ] || [ "$top" != "$base" ]; then
	die "$next is not newer than $latest"
fi
printf '%s\n' "$next"
