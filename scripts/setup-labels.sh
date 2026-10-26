#!/usr/bin/env bash
# Create repository labels via GitHub API (see PR #34 setup notes).
set -euo pipefail
REPO="${1:-eddievibe4tech-byte/termux-ops-test}"
for spec in "type:feature:0e8a16" "type:bug:d73a4a" "type:docs:c5def5" "type:ci:bfd4f2" "type:test:fbc4ea" "type:security:e4e669" "type:refactor:d4c5f9" "priority:p0:b60200" "priority:p1:d93f0b" "priority:p2:fbca04" "platform:termux:1d76db" "platform:linux:aa0fff" "platform:cross:c2e0c6" "status:blocked:ededed" "status:needs-design:006b75" "status:ready:8cc4d1"; do
  name=${spec%%:*}; rest=${spec#*:}; color=${rest##*:}
  curl -s -X POST -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/$REPO/labels" -d "{\"name\":\"$name\",\"color\":\"$color\"}" >/dev/null || true
done
echo "labels ensured for $REPO"
