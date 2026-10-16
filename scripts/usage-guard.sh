#!/usr/bin/env bash
# Exit 0 if it is OK to spend Claude quota now, non-zero (with a reason on stdout) if not.
#
# Source of truth is the statusline JSON that an interactive Claude Code session writes to
# $CLADEX_USAGE_FILE (see docs/ISSUE_PIPELINE.md for the statusLine snippet). There is no
# documented way to read subscription usage outside a session, so this FAILS CLOSED:
# missing, stale, or unparseable data means "don't run".
#
# Tunables (env):
#   CLADEX_USAGE_MAX_5H   skip if 5-hour window used % is above this   (default 70)
#   CLADEX_USAGE_MAX_7D   skip if weekly window used % is above this   (default 70)
#   CLADEX_USAGE_MAX_AGE  max age of the usage file, seconds           (default 1800)
#   CLADEX_USAGE_FILE     path to the statusline JSON dump             (default ~/.cache/claude-statusline.json)
set -uo pipefail

file="${CLADEX_USAGE_FILE:-$HOME/.cache/claude-statusline.json}"
max5="${CLADEX_USAGE_MAX_5H:-70}"
max7="${CLADEX_USAGE_MAX_7D:-70}"
max_age="${CLADEX_USAGE_MAX_AGE:-1800}"

[ -r "$file" ] || { echo "usage unknown: $file missing"; exit 10; }

age=$(( $(date +%s) - $(stat -c %Y "$file") ))
[ "$age" -le "$max_age" ] || { echo "usage unknown: $file is ${age}s old (max ${max_age}s)"; exit 11; }

now=$(date +%s)
# A window whose resets_at has passed is no longer in effect, so it can't block. A window that is
# absent is "unknown", which blocks unless the other window is known (both absent => block).
read -r p5 p7 < <(jq -r --argjson now "$now" '
  def pct($w): (.rate_limits[$w] // null)
    | if . == null or (.used_percentage | type) != "number" then "unknown"
      elif (.resets_at // 0) <= $now then "expired"
      else (.used_percentage | tostring) end;
  "\(pct("five_hour")) \(pct("seven_day"))"' "$file" 2>/dev/null) || { echo "usage unknown: cannot parse $file"; exit 12; }

[ -n "${p5:-}" ] || { echo "usage unknown: cannot parse $file"; exit 12; }

if [ "$p5" = unknown ] && [ "$p7" = unknown ]; then
  echo "usage unknown: no rate_limits in $file"; exit 13
fi

over() { awk -v p="$1" -v m="$2" 'BEGIN{exit !(p+0 > m+0)}'; }

if [[ "$p5" =~ ^[0-9.]+$ ]] && over "$p5" "$max5"; then
  echo "5h usage ${p5}% > ${max5}%"; exit 20
fi
if [[ "$p7" =~ ^[0-9.]+$ ]] && over "$p7" "$max7"; then
  echo "7d usage ${p7}% > ${max7}%"; exit 21
fi

echo "usage ok (5h=${p5} 7d=${p7})"
exit 0
