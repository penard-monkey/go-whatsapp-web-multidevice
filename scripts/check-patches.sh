#!/bin/bash
# Verify every patch listed in PATCHES.md is still in the tree.
#
# A fork does not remember its patches: a sync that rebases cleanly around our
# code, or a reset to upstream, drops them without a word. This turns that
# silence into a failure. saywhat's services/gowa/build.sh runs it before it
# will build, and sync-upstream.sh runs it after every rebase.
#
#   scripts/check-patches.sh            check the sentinels
#   scripts/check-patches.sh --test     also run the Go tests that cover them
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
manifest="$repo/PATCHES.md"
[ -f "$manifest" ] || { echo "no PATCHES.md at $repo" >&2; exit 1; }

# Table rows look like: | `id` | what | `sentinel` | `files` | `tests` |
rows=$(awk -F'|' '/^\| `/ {gsub(/[` ]/,"",$2); gsub(/`/,"",$4); gsub(/^ +| +$/,"",$4); print $2 "\t" $4}' "$manifest")
[ -n "$rows" ] || { echo "PATCHES.md lists no patches — if that is right, delete this check" >&2; exit 1; }

missing=0
while IFS=$'\t' read -r id sentinel; do
  [ -n "$id" ] || continue
  # -w, not a substring: a renamed `FormatTemplateSummaryXX` must not satisfy
  # a sentinel of `FormatTemplateSummary`. Tests are excluded on purpose — a
  # sentinel surviving only in a _test.go file means the implementation is
  # gone, which is exactly the case this is here to catch.
  if grep -rqwF "$sentinel" "$repo/src" --include='*.go' --exclude='*_test.go'; then
    printf '  ok      %-20s %s\n' "$id" "$sentinel"
  else
    printf '  MISSING %-20s %s\n' "$id" "$sentinel" >&2
    missing=$((missing + 1))
  fi
done <<< "$rows"

if [ "$missing" -gt 0 ]; then
  cat >&2 <<EOF

$missing patch(es) from PATCHES.md are not in this tree. An upstream sync
almost certainly dropped them. Recover with:
  git log --all -S'<sentinel>' -- src/
and re-apply, or delete the row from PATCHES.md if the patch is genuinely
retired. Do not build from here in the meantime: the binary would quietly
lose behaviour saywhat depends on.
EOF
  exit 1
fi

if [ "${1:-}" = "--test" ]; then
  echo "running the tests that cover them"
  (cd "$repo/src" && go test ./pkg/utils/)
fi
echo "all patches present"
