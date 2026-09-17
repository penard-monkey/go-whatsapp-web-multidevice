#!/bin/bash
# Rebase this fork's patches onto upstream, then prove they survived.
#
# Rebase, not merge: our patches stay a readable series on top of upstream
# instead of a tangle, and a conflict lands inside the SAYWHAT-PATCH block
# that actually collided. What git will NOT do is notice when upstream
# refactors around our code and the rebase succeeds with the patch no longer
# doing anything — that is what check-patches.sh and the Go tests are for.
#
#   scripts/sync-upstream.sh              rebase onto upstream/main
#   scripts/sync-upstream.sh v9.4.0       rebase onto a tag
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
target=${1:-upstream/main}
upstream_url=https://github.com/aldinokemal/go-whatsapp-web-multidevice.git

git remote get-url upstream >/dev/null 2>&1 || {
  echo "adding upstream remote ($upstream_url)"
  git remote add upstream "$upstream_url"
}
[ -z "$(git status --porcelain)" ] || { echo "working tree is dirty; commit or stash first" >&2; exit 1; }

branch=$(git rev-parse --abbrev-ref HEAD)
before=$(git rev-parse HEAD)
echo "fetching upstream"
git fetch upstream --tags

echo "rebasing $branch onto $target"
if ! git rebase "$target"; then
  cat >&2 <<EOF

Rebase stopped on a conflict. That is the system working: upstream changed
code one of our patches touches. Resolve it, 'git rebase --continue', then:
  scripts/check-patches.sh --test
To abandon: git rebase --abort   (you were at $before)
EOF
  exit 1
fi

echo
"$repo/scripts/check-patches.sh"
echo
echo "running the full suite"
(cd "$repo/src" && go build ./... && go test ./...)
cat <<EOF

Synced $branch: $before -> $(git rev-parse --short HEAD)

Nothing is live yet. To ship it:
  cd ~/workspace/saywhat && services/gowa/build.sh
then kickstart the job deliberately — that drops the WhatsApp session.
EOF
