#!/bin/sh
# Zero-downtime deploy of production. Runs on the NAS from /volume1/docker/cladex, next to
# compose.yaml, via cron (docs/NAS_OPERATIONS.md#deploy-pipeline):
#   */5 * * * * /volume1/docker/cladex/deploy.sh >> /volume1/docker/cladex/deploy.log 2>&1
#
#   deploy.sh          pull; if production's image changed, roll production onto it
#   deploy.sh restart  roll production even if it hasn't (a compose or secrets change)
#   deploy.sh active   print the live slot, cladex-blue or cladex-green
#
# Rolling: start the new image in the idle slot, wait until its /healthz answers, then
# stop the old slot. nginx (nginx.conf) retries refused connections on the other slot, so
# requests don't notice. A new slot that never gets healthy is removed and the old one
# keeps serving; the next tick tries again.
#
# Exclusive rolling, when the new image brings a pending migration marked
# `-- cladex:exclusive` (one the old code can't run against): stop the old slot first,
# so nothing serves while the schema changes, and nginx shows its maintenance page until
# the new slot answers. If the new slot fails, the old one comes back only if none of
# those migrations got applied; otherwise production stays on the maintenance page,
# because neither build can serve that schema, until a fix is pushed.
set -eu
cd "$(dirname "$0")"

IMAGE=ghcr.io/gerrygoo/cladex-web:latest
HEALTH_WAIT=60       # seconds: startup runs pending migrations before it listens
EXCLUSIVE_WAIT=600   # the same, for a deploy that pauses production on a long migration
ERR=/tmp/cladex-deploy.err

log() { echo "$(date '+%F %T') $*"; }
port() { case $1 in cladex-blue) echo 8092 ;; cladex-green) echo 8093 ;; esac; }
running() { [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = true ]; }

# The new image's pending exclusive migrations, one per line (cmd/server runMigrateCLI).
# Read-only: it migrates nothing. Fails if it can't tell.
pending_exclusive() {
	set +e
	out=$(docker compose run --rm --no-deps -T cladex migrate pending-exclusive 2>"$ERR")
	rc=$?
	set -e
	case $rc in
	0 | 3) echo "$out" ;;
	*) cat "$ERR" >&2; return 1 ;;
	esac
}

# The live slot, empty if neither runs. Both run only mid-roll, or after someone ran
# `docker compose up -d` by hand; then the one started last is the live one.
active() {
	if running cladex-blue && running cladex-green; then
		b=$(docker inspect -f '{{.State.StartedAt}}' cladex-blue)
		g=$(docker inspect -f '{{.State.StartedAt}}' cladex-green)
		if [ "$(printf '%s\n%s\n' "$b" "$g" | sort | tail -1)" = "$g" ]; then
			echo cladex-green
		else
			echo cladex-blue
		fi
	elif running cladex-green; then
		echo cladex-green
	elif running cladex-blue; then
		echo cladex-blue
	fi
}

if [ "${1:-}" = active ]; then
	active
	exit 0
fi

# One deploy at a time: a slow roll must not meet the next cron tick.
exec 9>/tmp/cladex-deploy.lock
flock -n 9 || { log "another deploy is running"; exit 0; }

docker compose pull -q
# Everything but production is simply recreated when its image or config changes.
docker compose up -d --no-deps cladex-proxy litestream cladex-staging

live=$(active)
case $live in
cladex-blue) next=cladex-green ;;
*) next=cladex-blue ;;
esac

if [ -n "$live" ] && [ "${1:-}" != restart ] &&
	[ "$(docker inspect -f '{{.Image}}' "$live")" = "$(docker image inspect -f '{{.Id}}' "$IMAGE")" ]; then
	if running "$next"; then
		log "stopping leftover $next ($live is live)"
		docker compose stop "$next"
	fi
	exit 0
fi

exclusive=
limit=$HEALTH_WAIT
if [ -n "$live" ]; then
	exclusive=$(pending_exclusive) || { log "can't tell whether the new image's migrations are exclusive; not rolling"; exit 1; }
fi
if [ -n "$exclusive" ]; then
	log "exclusive migration(s) $(echo $exclusive): pausing production — $live stops first, nginx shows the maintenance page"
	docker compose stop "$live"
	limit=$EXCLUSIVE_WAIT
fi

log "rolling ${live:-nothing live} → $next"
docker compose up -d --no-deps --force-recreate "$next"
url="http://127.0.0.1:$(port "$next")/healthz"
waited=0
until health=$(curl -fsS -m 2 "$url" 2>/dev/null); do
	waited=$((waited + 1))
	# A restart means it exited (a failed migration, say): no point waiting out the limit.
	restarts=$(docker inspect -f '{{.RestartCount}}' "$next" 2>/dev/null || echo 0)
	if [ "$waited" -ge "$limit" ] || [ "$restarts" -gt 0 ]; then
		log "$next not healthy after ${waited}s ($restarts restarts), removing it. Its last lines:"
		docker compose logs --no-log-prefix --tail 30 "$next" || true
		docker compose rm -sf "$next"
		if [ -z "$exclusive" ]; then
			log "${live:-nothing} stays live"
		elif [ "$(pending_exclusive 2>/dev/null)" = "$exclusive" ]; then
			log "none of the exclusive migrations got applied; $live serves again"
			docker compose start "$live"
		else
			log "PRODUCTION IS DOWN: the schema changed but $next never got healthy, and $live can't serve the new schema. The maintenance page is up; push a fix."
		fi
		exit 1
	fi
	sleep 1
done
log "$next healthy: $health"

if [ -n "$live" ] && [ -z "$exclusive" ]; then
	# SIGTERM: the old slot stops accepting and drains its in-flight requests (cmd/server).
	docker compose stop "$live"
fi
log "live: $next"
