#!/usr/bin/env bash
# Print the release notes in Markdown: the commits since the previous
# release, grouped by Conventional Commit type, then how to install or update.
#
#   scripts/release-notes.sh          notes for what HEAD would release
#   scripts/release-notes.sh v0.2.0   notes for an existing tag
#
# The same text is shown before tagging and published on the release page.
# Commits typed docs, ci, build, chore, style, test or refactor are left out.
set -euo pipefail

repo=mydisha/lega
tag=${1:-}
end=${tag:-HEAD}
git rev-parse -q --verify "$end^{commit}" >/dev/null || {
	printf 'release-notes: unknown tag %s\n' "$end" >&2
	exit 1
}

prev=$(git describe --tags --abbrev=0 --match 'v[0-9]*' "$end^" 2>/dev/null || true)
range=$end
[ -z "$prev" ] || range="$prev..$end"

# section TITLE TYPE prints the commits of one type, as "- scope: subject".
section() {
	local lines
	lines=$(git log --no-merges --reverse --format='%s' "$range" |
		grep -E "^$2(\([^)]*\))?!?: " |
		sed -E 's/^[a-z]+\(([^)]*)\)!?: (.*)$/- \1: \2/; s/^[a-z]+!?: (.*)$/- \1/' || true)
	[ -z "$lines" ] || printf '## %s\n\n%s\n\n' "$1" "$lines"
}

[ -n "$prev" ] || printf 'The first release of lega.\n\n'

breaking=$(git log --no-merges --reverse --format='%s' "$range" |
	grep -E '^[a-z]+(\([^)]*\))?!: ' |
	sed -E 's/^[a-z]+(\([^)]*\))?!: (.*)$/- \2/' || true)
[ -z "$breaking" ] || printf '## Breaking changes\n\n%s\n\n' "$breaking"

body=$(
	section "New features" feat
	section "Bug fixes" fix
	section "Performance" perf
)
if [ -n "$body" ]; then
	printf '%s\n\n' "$body"
else
	printf 'Maintenance release: build, docs and tooling only.\n\n'
fi

cat <<EOF
## Install or update

Already have lega? Run \`lega update\`.

New install:

\`\`\`sh
curl -fsSL https://raw.githubusercontent.com/$repo/main/install.sh | bash
\`\`\`

Or download the \`.deb\` below and run \`sudo apt install ./lega_*_amd64.deb\`.
Check any file with \`gh attestation verify <file> -R $repo\`.
EOF

if [ -n "$prev" ] && [ -n "$tag" ]; then
	printf '\n**Full changelog:** https://github.com/%s/compare/%s...%s\n' "$repo" "$prev" "$tag"
fi
