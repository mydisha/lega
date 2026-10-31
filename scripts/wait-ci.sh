#!/usr/bin/env bash
# Wait for the CI workflow on a commit to finish, and fail unless it passed.
#
#   scripts/wait-ci.sh <commit> [owner/repo]
#
# Right after a merge, CI takes a few seconds to start, so a missing run is
# polled for a while before giving up. Needs gh.
set -euo pipefail

sha=${1:?usage: scripts/wait-ci.sh <commit> [owner/repo]}
repo=${2:-mydisha/lega}
tries=${WAIT_CI_TRIES:-24} # with WAIT_CI_SLEEP=5, two minutes
pause=${WAIT_CI_SLEEP:-5}

die() {
	printf 'wait-ci: %s\n' "$*" >&2
	exit 1
}

id=""
for ((i = 0; i < tries; i++)); do
	id=$(gh run list -R "$repo" --workflow CI --commit "$sha" --limit 1 \
		--json databaseId --jq '.[].databaseId')
	[ -z "$id" ] || break
	[ "$i" -gt 0 ] || printf 'Waiting for CI to start on %s...\n' "${sha:0:7}"
	sleep "$pause"
done
[ -n "$id" ] || die "no CI run found for ${sha:0:7}; push it to GitHub first"

url="https://github.com/$repo/actions/runs/$id"
if [ "$(gh run view "$id" -R "$repo" --json status --jq .status)" != completed ]; then
	printf 'Waiting for CI on %s to finish: %s\n' "${sha:0:7}" "$url"
	# The result is read below, so a failed run is reported the same way.
	gh run watch "$id" -R "$repo" --interval 10 >/dev/null || true
fi

conclusion=$(gh run view "$id" -R "$repo" --json conclusion --jq .conclusion)
[ "$conclusion" = success ] || die "CI on ${sha:0:7} ended with: ${conclusion:-unknown}. Fix it first: $url"
printf 'CI passed on %s.\n' "${sha:0:7}"
