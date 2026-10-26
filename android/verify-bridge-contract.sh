#!/usr/bin/env bash
# android/verify-bridge-contract.sh — CI guard for issue #39.
#
# The Companion APK parses the bridge JSON contract with hand-written Kotlin
# (BridgeStatus.kt). If the Go side renames or drops a field, the APK silently
# reads empty strings. This script extracts every JSON tag from
# internal/bridge/status.go and fails when one is missing from the Kotlin
# parser — keeping docs/companion-app-bridge.md as the single source of truth.
set -euo pipefail

cd "$(dirname "$0")/.."

GO_SRC="internal/bridge/status.go"
KT_SRC="android/app/src/main/java/dev/hlops/companion/BridgeStatus.kt"

missing=0
# Collect `json:"name,omitempty"` tags only from the exported BridgeStatus /
# TaskState structs (the APK-facing schema). The private rawBackup/rawCleanup
# mirrors describe on-disk *reports*, which the CLI normalises away before the
# APK ever sees them, so they are intentionally excluded here.
fields=$(awk '
    /^type (BridgeStatus|TaskState) struct/ { inside = 1; next }
    inside && /^}/ { inside = 0 }
    inside { while (match($0, /json:"[a-z_0-9]+/)) { print substr($0, RSTART+6, RLENGTH-6); $0 = substr($0, RSTART+RLENGTH) } }
' "$GO_SRC" | sort -u)

while IFS= read -r field; do
    [ -z "$field" ] && continue
    if ! grep -q "\"$field\"" "$KT_SRC"; then
        echo "ERROR: bridge JSON field '$field' (from $GO_SRC) not handled in $KT_SRC" >&2
        missing=1
    fi
done <<< "$fields"

if [ "$missing" -ne 0 ]; then
    echo "Bridge contract drift detected — update BridgeStatus.kt and docs/companion-app-bridge.md together." >&2
    exit 1
fi
echo "OK: all $(grep -c . <<< "$fields") bridge JSON fields are covered by the Kotlin parser."
