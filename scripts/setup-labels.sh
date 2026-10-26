#!/usr/bin/env bash
# Create GitHub labels for termux-ops (hlops)
set -euo pipefail
REPO="${1:-eddievibe4tech-byte/termux-ops-test}"
TOKEN="${GITHUB_TOKEN:?set GITHUB_TOKEN}"
API="https://api.github.com/repos/${REPO}/labels"

add_label() { # name color description
  code=$(curl -s -o /tmp/label_resp.json -w '%{http_code}' -X POST "$API" \
    -H "Authorization: Bearer ${TOKEN}" -H "Accept: application/vnd.github+json" \
    -d "{\"name\":\"$1\",\"color\":\"$2\",\"description\":\"$3\"}")
  if [ "$code" = "422" ]; then
    curl -s -o /dev/null -X PATCH "${API}/$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$1")" \
      -H "Authorization: Bearer ${TOKEN}" -H "Accept: application/vnd.github+json" \
      -d "{\"color\":\"$2\",\"description\":\"$3\"}" >/dev/null
    echo "updated: $1"
  elif [ "$code" = "201" ]; then
    echo "created: $1"
  else
    echo "FAILED($code): $1"; cat /tmp/label_resp.json; exit 1
  fi
}

# type:*
add_label "type:feature"   "a2eeef" "New feature or request"
add_label "type:bug"       "d73a4a" "Something isn't working"
add_label "type:docs"      "0075ca" "Documentation improvements"
add_label "type:ci"        "e9969a" "Continuous integration & release workflow"
add_label "type:test"      "bfd4f2" "Tests & coverage"
add_label "type:security"  "ededed" "Security related work"
add_label "type:refactor"  "c5def5" "Refactoring without behavior change"
# priority:*
add_label "priority:p0"    "b60205" "Must do / blocking"
add_label "priority:p1"    "d93f0b" "Should do in this milestone"
add_label "priority:p2"    "fbca04" "Nice to have"
# platform:*
add_label "platform:termux" "8ccbf1" "Termux (Android) specific"
add_label "platform:linux"  "c2e0c6" "General Linux"
add_label "platform:cross"  "fef2c0" "Cross-platform concern"
# area:*
add_label "area:config"          "1d76db" "Config loading & defaults"
add_label "area:analyze"         "5319e7" "Storage analysis"
add_label "area:backup"          "0e8a16" "Backup engine"
add_label "area:backup-task"     "0e8a16" "Backup task model & types"
add_label "area:cleanup"         "e4e669" "Cleanup framework"
add_label "area:cleanup-native"  "e4e669" "Native cleanup actions"
add_label "area:cleanup-providers" "e4e669" "Provider cleanup (pkg/apt)"
add_label "area:run"             "006b75" "run command orchestration"
add_label "area:schedule"        "642efa" "Schedule generator (crond)"
add_label "area:logging"         "d4c5f9" "Logging system"
add_label "area:packaging"       "bfd4f2" "Packaging & releases"
add_label "area:termux-storage"  "8ccbf1" "Termux storage paths/permissions"
add_label "area:crond"           "642efa" "cron integration"
add_label "area:wake-lock"       "f9d0c4" "termux-wake-lock integration"
# status:*
add_label "status:blocked"      "d4c5f9" "Blocked by external dependency"
add_label "status:needs-design" "ffffff" "Needs design before implementation"
add_label "status:ready"        "0e8a16" "Ready to start"
echo "All labels done."
