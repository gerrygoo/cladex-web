#!/usr/bin/env bash
# Public uptime check of production, from anywhere with internet: the scheduled
# .github/workflows/uptime.yml runs it, and scripts/nas-check.sh uses it for its public
# half. Read-only. Exit status: 0 all good, 1 something is wrong.
#
#   - /healthz answers "ok <sha>" on every one of SAMPLES requests, always the same sha.
#     One request isn't enough: on 2026-11-13 the proxy spread requests over both
#     blue/green ports and a monitoring hub on the idle one answered half of them.
#   - c.cladex.com.mx redirects to cotizador.cladex.com.mx.
#   - Both TLS certificates have at least CERT_DAYS left (certbot renews on the VPS).
#   - With gh available: production runs the sha of the last successful deploy run once
#     that run is DEPLOY_GRACE minutes old (the NAS cron pulls every 5 min), so a stuck
#     deploy shows up here too.
set -uo pipefail

HOST=cotizador.cladex.com.mx
SHORT=c.cladex.com.mx
SAMPLES=${SAMPLES:-10}
CERT_DAYS=14
DEPLOY_GRACE=30
status=0
ok() { echo "  ✓ $*"; }
bad() { echo "  ✗ $*"; status=1; }

# Distinct /healthz answers over $2 requests to $1, one per line with its count, the
# body cut to 40 characters (an HTML page shows up as its first line).
sample() {
	for _ in $(seq 1 "$2"); do
		body=$(curl -s -m 10 "$1" | head -1 | cut -c1-40)
		echo "${body:-<no answer>}"
	done | sort | uniq -c
}

answers=$(sample "https://$HOST/healthz" "$SAMPLES")
if [ "$(wc -l <<<"$answers")" -eq 1 ] && grep -qE '^ *[0-9]+ ok [0-9a-f]+$' <<<"$answers"; then
	sha=$(awk '{print $3}' <<<"$answers")
	ok "https://$HOST/healthz → ok $sha on all $SAMPLES requests"
else
	sha=
	bad "https://$HOST/healthz answered something other than \"ok <sha>\" (count, answer):"
	sed 's/^/      /' <<<"$answers"
fi

loc=$(curl -s -m 10 -o /dev/null -w '%{http_code} %{redirect_url}' "https://$SHORT/")
[ "$loc" = "301 https://$HOST/" ] && ok "https://$SHORT/ → 301 to $HOST" ||
	bad "https://$SHORT/ should 301 to https://$HOST/, got: $loc"

for h in "$HOST" "$SHORT"; do
	end=$(echo | timeout 10 openssl s_client -connect "$h:443" -servername "$h" 2>/dev/null |
		openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
	if [ -z "$end" ]; then
		bad "couldn't read $h's TLS certificate"
		continue
	fi
	days=$(( ($(date -d "$end" +%s) - $(date +%s)) / 86400 ))
	[ "$days" -ge "$CERT_DAYS" ] && ok "$h certificate: ${days} days left" ||
		bad "$h certificate expires in ${days} days ($end) — is certbot renewing on the VPS?"
done

if [ -n "$sha" ] && command -v gh >/dev/null; then
	run=$(gh run list --repo gerrygoo/cladex-web --workflow deploy.yml --branch main --status success \
		--limit 1 --json headSha,updatedAt -q '.[0] | "\(.headSha[0:7]) \(.updatedAt)"' 2>/dev/null)
	read -r run_sha run_at <<<"$run"
	if [ -z "${run_sha:-}" ]; then
		echo "  · couldn't read the last deploy run; skipping the deployed-sha check"
	elif [ "$run_sha" = "$sha" ]; then
		ok "production runs the last deployed build ($sha)"
	else
		mins=$(( ($(date +%s) - $(date -d "$run_at" +%s)) / 60 ))
		if [ "$mins" -lt "$DEPLOY_GRACE" ]; then
			echo "  · production is on $sha, the last deploy run built $run_sha ${mins} min ago; still rolling out"
		else
			bad "production is on $sha but the deploy run for $run_sha finished ${mins} min ago — the NAS deploy is stuck (deploy.log on the NAS says why)"
		fi
	fi
fi

exit "$status"
