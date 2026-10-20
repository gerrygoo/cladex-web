#!/usr/bin/env bash
# Read-only health check of the production NAS (see docs/NAS_OPERATIONS.md and
# .claude/skills/nas-check/SKILL.md). Changes nothing anywhere. Exit status: 0 all good,
# 1 something needs attention, 2 couldn't check something important.
set -uo pipefail

cd "$(dirname "$0")/.."
NAS_HOST=192.168.3.169
LAN_URL="http://$NAS_HOST:8090/healthz"
PUBLIC_URL="https://cotizador.cladex.com.mx/healthz"
status=0
warn() { echo "  ⚠ $*"; [ "$status" -lt 1 ] && status=1; }
fail() { echo "  ✗ $*"; status=2; }
ok() { echo "  ✓ $*"; }

echo "== Deploy"
main_sha=$(git ls-remote origin refs/heads/main 2>/dev/null | cut -c1-7)
[ -n "$main_sha" ] || fail "can't read origin/main"
run=$(gh run list --branch main --limit 1 --json headSha,status,conclusion,updatedAt \
	-q '.[0] | "\(.headSha[0:7]) \(.status) \(.conclusion) \(.updatedAt)"' 2>/dev/null)
read -r run_sha run_status run_conclusion run_at <<<"$run"
echo "  origin/main: $main_sha · last Actions run: $run_sha $run_status/${run_conclusion:-–} at $run_at"
[ "$run_conclusion" = "failure" ] && warn "the last deploy workflow failed: gh run list --branch main"

lan=$(curl -s -m 5 "$LAN_URL")
public=$(curl -s -m 10 "$PUBLIC_URL")
lan_sha=${lan#ok }
public_sha=${public#ok }
[[ "$lan" == ok* ]] && ok "LAN    $LAN_URL → $lan" || fail "LAN health check failed ($LAN_URL): '${lan:-no answer}'"
[[ "$public" == ok* ]] && ok "public $PUBLIC_URL → $public" || fail "public health check failed ($PUBLIC_URL): '${public:-no answer}' — VPS/FRP or the app"

if [[ "$lan" == ok* && -n "$main_sha" ]]; then
	if [ "$lan_sha" = "$main_sha" ]; then
		ok "production runs origin/main ($main_sha)"
	else
		# The NAS pulls every 5 min after Actions pushes the image; give it ~15 min.
		age=$(( $(date +%s) - $(date -d "$run_at" +%s 2>/dev/null || date +%s) ))
		if [ "$run_status" != "completed" ] || [ "$age" -lt 900 ]; then
			echo "  … production is on $lan_sha, main is $main_sha; deploy still in flight (run ${run_status}, ${age}s ago)"
		else
			warn "production is on $lan_sha but main is $main_sha, ${age}s after the deploy run — the NAS pull cron may be stuck"
		fi
	fi
fi

echo "== NAS (over SSH)"
if ! timeout 4 bash -c "echo > /dev/tcp/$NAS_HOST/22" 2>/dev/null; then
	fail "SSH port 22 on $NAS_HOST is closed — SSH is probably turned off in UGOS (Control Panel → Terminal). Skipping container, backup and disk checks."
	exit "$status"
fi
remote=$(timeout 40 ssh -o BatchMode=yes -o ConnectTimeout=8 cladex-nas '
	cd /volume1/docker/cladex || exit 3
	echo "--ps";        docker compose ps --format "{{.Name}}|{{.State}}|{{.Status}}|{{.Image}}"
	echo "--image";     docker image inspect ghcr.io/gerrygoo/cladex-web:latest --format "{{.Created}}" 2>/dev/null
	echo "--litestream"; docker logs cladex-litestream --since 30m 2>&1 | tail -5
	echo "--backup";    ls -t backups/*.tar* 2>/dev/null | head -1 | xargs -r stat -c "%Y %n"
	echo "--disk";      df -h /volume1 | tail -1
	echo "--stats";     docker stats --no-stream --format "{{.Name}}|{{.MemUsage}}|{{.CPUPerc}}" $(docker compose ps -q)
' 2>&1) || { fail "ssh cladex-nas failed: $(echo "$remote" | tail -1)"; exit "$status"; }

section() { awk -v s="--$1" '$0==s{f=1;next} /^--/{f=0} f' <<<"$remote"; }
while IFS='|' read -r name state st image; do
	[ -z "$name" ] && continue
	[ "$state" = "running" ] && ok "$name: $st" || warn "$name is $state ($st)"
done <<<"$(section ps)"
echo "  image built: $(section image)"
lit=$(section litestream)
if grep -qiE "error|fail" <<<"$lit"; then warn "litestream logged errors in the last 30 min:"; echo "$lit" | sed 's/^/      /'; else ok "litestream: no errors in the last 30 min"; fi
read -r bk_epoch bk_name <<<"$(section backup)"
if [ -n "${bk_epoch:-}" ]; then
	hours=$(( ($(date +%s) - bk_epoch) / 3600 ))
	[ "$hours" -le 26 ] && ok "newest backup $bk_name (${hours}h old)" || warn "newest backup $bk_name is ${hours}h old (nightly expected)"
else
	warn "no backup tarball found under backups/"
fi
disk=$(section disk)
use=$(awk '{print $5}' <<<"$disk" | tr -d %)
[ "${use:-0}" -lt 85 ] && ok "disk /volume1: $disk" || warn "disk /volume1 is ${use}% full: $disk"
section stats | sed 's/^/  · /'
exit "$status"
