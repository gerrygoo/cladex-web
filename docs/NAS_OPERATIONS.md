# Operating the UGREEN NAS — notes

A consolidated runbook for the box Cladex actually runs on. The authoritative
architecture decisions live in [PLAN.md](PLAN.md) (Environment section and the M0/M1
footnotes); this file pulls the operational parts of that together in one place and
adds what's been learned running commands against the box across sessions.

## Hardware

UGREEN NASync **DXP4800** — Intel N100, 4 cores, 8 GB RAM. Shared with other services
(Jellyfin, etc.), so the app's footprint matters: the container idles around 25 MB and
the deploy pipeline was built to avoid anything heavier (this is why PDF rendering uses
a static Typst binary instead of a headless Chromium, which idles ~400 MB and spikes
past 1 GB per render).

## Network access

- **SSH alias**: `ssh cladex-nas` → `192.168.3.169`, user `ggo`, key
  `~/.ssh/cladex_nas`, defined in `~/.ssh/config`. `ggo` has docker-group membership, so
  `docker` commands work without `sudo`.
- **It's LAN-only.** The NAS has no public IP; there's no route to it from the open
  internet. The VPS (`cladex-vps`, see below) exists specifically to front it.
- **Sandboxed sessions may not have a route to it at all.** Confirmed 2026-08-13 from
  this worktree: `ssh cladex-nas` returns `Connection refused` (not an auth failure —
  the connection itself doesn't reach the LAN), and `ssh cladex-vps` separately fails
  because the 1Password SSH agent isn't reachable from this sandbox either. **Verify
  connectivity at the start of any session that needs to touch the NAS or VPS** — don't
  assume access carries over from a previous session, and don't assume a failure means
  the box is down. If unreachable, hand the command to the user rather than guessing
  around it.
- **VPS**: `ssh cladex-vps` → `5.78.203.98` (Hetzner), user `root`, key `cladex-vps` in
  1Password's SSH agent. Runs nginx + certbot + FRP server, multiplexing the public
  domains (`cotizador.cladex.com.mx`, `c.cladex.com.mx`) onto the NAS over FRP, since
  the NAS itself is behind NAT.

## scp is broken — use `scp -O`

Plain `scp` to the NAS fails with `dest open "..." No such file or directory` even when
the destination directory exists and is writable. The NAS's sshd doesn't play well with
the SFTP-based protocol modern OpenSSH clients default to for `scp`. **Always pass `-O`**
(forces the legacy protocol):

```bash
scp -O somefile cladex-nas:/volume1/docker/cladex/somefile
```

Discovered deploying the 1.1 backup script; cost real time before the `-O` flag was
found, so don't rediscover this the hard way again.

## Docker / filesystem conventions

- Compose, `network_mode: host` (not bridged — simplifies port handling on a box
  already juggling several services' ports).
- App configs and data live under `/volume1/docker/[service]/`, e.g.
  `/volume1/docker/cladex/data/cladex.db`. Quote PDFs are regenerated on request, not
  stored, so the database is the only state.
- `PUID=1000` / `PGID=10` — match these in any compose file touching NAS-mounted
  volumes, or file ownership breaks.
- **Ports already taken on this box**: 8080 (FRP), 8081, 8096, 8191, 8282, 5055, 7878,
  8989. Cladex itself runs on **8090**.
- The deployed image is **distroless — no shell.** `docker compose exec cladex sh` (or
  any `exec ... sh`) fails outright. Exec the app binary directly:
  ```bash
  docker compose exec cladex /cladex user add rodolfo --name "Rodolfo Flores" --role vendedor
  ```
  This also constrains what the image can ship: only `/cladex` is in the final layer
  (see `Dockerfile`), so `cmd/server/main.go` dispatches `user add/passwd/disable`
  itself when invoked as `cladex user ...` — there is no separate `cladexctl` binary in
  the deployed image, even though one exists in the repo for local dev convenience
  (`go run ./cmd/cladexctl ...`, without the server's env-var assumptions).

## Deploy pipeline

GitHub Actions builds and pushes `ghcr.io/gerrygoo/cladex-web:latest` (public registry,
no auth needed to pull) on every push to `main`. The NAS can't be pushed to directly —
it's behind NAT — so a cron entry on the NAS pulls every 5 minutes and restarts if the
image changed. End to end: push to `main` → Actions build (~a couple minutes) → next
NAS cron tick (≤5 min) → live. No manual deploy step in the normal case.

**Installing or changing the NAS's own crontab needs `sudo`**, which prompts for a
password a non-interactive SSH session can't answer — this is a hard wall for an agent
session; hand it to the user rather than trying to script around it. Same wall applies
to the nightly backup cron (see below).

## Litestream (continuous DB replication)

`compose.yaml` carries a `litestream` sidecar that replicates `data/cladex.db` to
`backups/litestream/cladex` continuously, closing the up-to-24h window the nightly
tarball leaves open. Config is `litestream.yml` in the repo root.

**This does not arrive via the deploy pipeline.** The NAS cron pulls the *image*; it
does not sync `compose.yaml`, which the NAS keeps its own copy of from slice 0.6. So a
compose change is a manual push, one time:

```bash
ssh cladex-nas 'mkdir -p /volume1/docker/cladex/backups'
scp -O compose.yaml litestream.yml cladex-nas:/volume1/docker/cladex/   # -O, see above
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose up -d'
```

Verify it caught:

```bash
ssh cladex-nas 'docker logs cladex-litestream --tail 20'   # expect "replica sync" lines
ssh cladex-nas 'ls -R /volume1/docker/cladex/backups/litestream/cladex | head'
```

Restore (the app must be stopped, and the target must not already exist):

```bash
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose stop cladex'
# Latest:
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm litestream \
    restore -config /etc/litestream.yml -o /data/cladex-recovered.db /data/cladex.db'
# A specific moment:
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm litestream \
    restore -config /etc/litestream.yml -timestamp 2026-08-19T14:30:00Z \
    -o /data/cladex-recovered.db /data/cladex.db'
```

Restoring to `cladex-recovered.db` rather than over `cladex.db` is deliberate — inspect
it before swapping anything into place.

Two caveats worth knowing before relying on this. Litestream v0.5 **dropped age
encryption**, so the replica is plaintext; that's acceptable while it stays on the NAS
under `backups/`, but not if it's ever pointed at S3. `backup.sh` still runs alongside
it for a self-contained nightly tarball that restores without Litestream.

## Backups

`backup.sh` runs nightly on the NAS: `sqlite3 .backup` + integrity check, into a
14-day-retention tarball under
`/volume1/docker/cladex/backups/`. Restore-tested by extracting into a scratch dir and
opening the DB — do this after any schema-shape change, before trusting new data lands
on top of it. Offsite copy is deferred to UGREEN's own NAS-to-cloud sync pointed at that
directory, once set up — no bespoke offsite push exists yet.

## Running one-off commands against production

- Cross-compile for the NAS's arch (`amd64`) before staging a binary there — the dev
  Mac is arm64, and this box is not.
- For anything that touches the live DB (like the 1.2 catalog import), dry-run first
  against the real production path, confirm it matches a local test run exactly, *then*
  run for real — and this always needs the user's explicit go-ahead, standing commit/push
  authorization doesn't cover writes to production data.
- Clean up scratch artifacts (staged binaries, workbooks, etc.) immediately after a
  one-off run — nothing sensitive should linger on the NAS longer than the run itself.

## Quick reference

| Task | Command |
|---|---|
| Check the app is up | `ssh cladex-nas curl -s localhost:8090/healthz` |
| See resource usage | `ssh cladex-nas docker stats --no-stream` |
| Tail app logs | `ssh cladex-nas docker compose -f /volume1/docker/cladex/compose.yaml logs -f` |
| Copy a file to the NAS | `scp -O <file> cladex-nas:/volume1/docker/cladex/<dest>` |
| Provision a user | `ssh cladex-nas docker compose exec cladex /cladex user add <username> --name "..." --role vendedor` |
| Force a redeploy now (don't wait for cron) | `ssh cladex-nas 'cd /volume1/docker/cladex && docker compose pull && docker compose up -d'` |
| Check replication is live | `ssh cladex-nas 'docker logs cladex-litestream --tail 20'` |
| Push a compose change (not automatic) | `scp -O compose.yaml litestream.yml cladex-nas:/volume1/docker/cladex/ && ssh cladex-nas 'cd /volume1/docker/cladex && docker compose up -d'` |
