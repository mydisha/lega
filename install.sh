#!/usr/bin/env bash
# Install lega from its GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/mydisha/lega/main/install.sh | bash
#   ... | bash -s -- v0.2.0                  install a given version
#   ... | bash -s -- --prefix /usr/local/bin install somewhere else
#
# The download is checked against the release's SHA256SUMS before anything
# is installed.
set -euo pipefail

releases="${LEGA_RELEASES_URL:-https://github.com/mydisha/lega/releases}"
releases="${releases%/}"
prefix=""
version=""

die() {
	printf 'lega install: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: install.sh [version] [--prefix DIR]

  version       release to install, e.g. v0.2.0 (default: latest)
  --prefix DIR  folder for the lega binary
                (default: ~/.local/bin, or /usr/local/bin as root)
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--prefix)
		[ $# -ge 2 ] || die "--prefix needs a folder"
		prefix="$2"
		shift 2
		;;
	--prefix=*)
		prefix="${1#--prefix=}"
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	v[0-9]* | [0-9]*)
		version="$1"
		shift
		;;
	*) die "unknown argument: $1 (see --help)" ;;
	esac
done

[ "$(uname -s)" = Linux ] || die "lega runs on Linux only"
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) die "no release build for $(uname -m); install with: go install github.com/mydisha/lega/cmd/lega@latest" ;;
esac
for tool in curl tar sha256sum; do
	command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

if [ -z "$prefix" ]; then
	if [ "$(id -u)" -eq 0 ]; then
		prefix=/usr/local/bin
	else
		prefix="$HOME/.local/bin"
	fi
fi

if [ -z "$version" ]; then
	# /releases/latest redirects to /releases/tag/<version>.
	url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "$releases/latest") ||
		die "cannot reach $releases"
	version="${url##*/}"
fi
case "$version" in v*) ;; *) version="v$version" ;; esac
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
	die "no lega release found"

asset="lega_linux_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

printf 'Downloading lega %s for %s...\n' "$version" "$arch"
curl -fsSL -o "$tmp/$asset" "$releases/download/$version/$asset" ||
	die "cannot download $asset for $version"
curl -fsSL -o "$tmp/SHA256SUMS" "$releases/download/$version/SHA256SUMS" ||
	die "cannot download SHA256SUMS for $version"

want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
[ -n "$want" ] || die "$asset is not listed in SHA256SUMS"
got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
[ "$got" = "$want" ] || die "$asset does not match its checksum; nothing was installed"

tar -xzf "$tmp/$asset" -C "$tmp" lega 2>/dev/null || die "the archive has no lega binary"

mkdir -p "$prefix" 2>/dev/null || die "cannot create $prefix"
[ -w "$prefix" ] || die "no write access to $prefix; use --prefix or run with sudo"
# Copy next to the target, then rename, so a running lega is never half written.
cp "$tmp/lega" "$prefix/.lega.new"
chmod 0755 "$prefix/.lega.new"
mv -f "$prefix/.lega.new" "$prefix/lega"

printf 'Installed lega %s to %s/lega\n' "${version#v}" "$prefix"

found=$(command -v lega 2>/dev/null || true)
case ":$PATH:" in
*":$prefix:"*)
	if [ -n "$found" ] && [ "$found" != "$prefix/lega" ]; then
		printf 'Note: %s comes first in your PATH.\n' "$found"
	fi
	;;
*)
	printf '\n%s is not in your PATH yet. Log out and back in, or run:\n' "$prefix"
	# shellcheck disable=SC2016 # $PATH is meant to be printed as is.
	printf '  export PATH="%s:$PATH"\n' "$prefix"
	;;
esac
printf '\nRun lega to start. Update later with: lega update\n'
