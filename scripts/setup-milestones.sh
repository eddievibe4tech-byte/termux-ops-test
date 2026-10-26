#!/usr/bin/env bash
# Create GitHub milestones for termux-ops (hlops)
set -euo pipefail
REPO="${1:-eddievibe4tech-byte/termux-ops-test}"
TOKEN="${GITHUB_TOKEN:?set GITHUB_TOKEN}"
API="https://api.github.com/repos/${REPO}/milestones"

create_ms() { # title description
  code=$(curl -s -o /tmp/ms_resp.json -w '%{http_code}' -X POST "$API" \
    -H "Authorization: Bearer ${TOKEN}" -H "Accept: application/vnd.github+json" \
    --data "$(python3 -c "import json,sys; print(json.dumps({'title':sys.argv[1],'description':sys.argv[2]}))" "$1" "$2")")
  if [ "$code" = "201" ]; then
    n=$(python3 -c "import json;print(json.load(open('/tmp/ms_resp.json'))['number'])")
    echo "created M#$n: $1"
  elif [ "$code" = "422" ]; then
    echo "exists: $1"
  else
    echo "FAILED($code): $1"; cat /tmp/ms_resp.json; exit 1
  fi
}

create_ms "M0: Go Foundation & Termux Build" "Go module init, CLI skeleton, Termux detection, logging."
create_ms "M1: Config & Termux Paths"        "Cross-platform paths, TOML multi-task config, init-config, doctor."
create_ms "M2: Analyze"                      "Termux storage scanning with safety limits and reports."
create_ms "M3: Multi-task Backup"            "tar.gz atomic backups, termux-env task, manifest, retention, sha256."
create_ms "M4: Native Cleanup"               "Temp cleanup and old backup pruning."
create_ms "M5: Provider Cleanup"             "pkg/apt probe framework and cache clean."
create_ms "M6: Run, Lock & Termux Wake-lock" "run orchestration, lockfile, termux-wake-lock integration."
create_ms "M7: Schedule Generator (crond)"   "Cron line generation and crond install guide."
create_ms "M8: Packaging & Docs"             "Release workflow, Termux install script, docs."
create_ms "M9: Future Enhancements"          "Backlog: incremental backup, encryption, remote targets, restore, job-scheduler."
echo "Milestones done."
