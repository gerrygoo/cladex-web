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
  the NAS itself is behind NAT. The `cotizador` site sets `client_max_body_size 30m`
  (added 2026-10-21, issue #32): nginx's 1 MB default refused Documentos oficiales
  uploads with a bare 413 before they reached the app, which caps them at 25 MB itself.

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
  8989. Cladex itself runs on **8090**; see [Ports](#ports) for all of Cladex's.

## Ports

Cladex owns these on the NAS, **whether or not anything is listening on them right now**:

| Port | What |
|------|------|
| 8090 | `cladex-proxy` (nginx): FRP and LAN users connect here |
| 8091 | `cladex-staging` |
| 8092 | `cladex-blue` — usually stopped, so it looks free. It isn't. |
| 8093 | `cladex-green` — likewise |

The homelab repo's `docs/Infra Reference.md` port table is the registry for the whole
box, and its `scripts/check-ports.sh` checks every stack's compose file against it. Run
it before starting any new service on the NAS.

What happens when something takes a slot's port anyway (2026-11-13: a Beszel monitoring
hub published on :8092 while blue was idle):

- `cladex-proxy` sends nothing to it while the live slot is up: the idle slot is only
  nginx's `backup`. (Before that day both slots were equal upstreams, so the squatter got
  half of all requests, publicly, for hours.)
- `deploy.sh` refuses to roll onto that slot and logs `port 8092 belongs to cladex-blue,
  but something else answers on it`, so new builds stop reaching production until the
  port is freed. Find the culprit with `docker ps --filter publish=8092`.
- `scripts/nas-check.sh` warns about it, and if it ever reaches users, the scheduled
  `uptime` workflow (`scripts/uptime-check.sh`, every 10 min) fails and GitHub emails.
- The deployed image is **distroless — no shell.** `docker compose exec cladex-blue sh`
  (or any `exec ... sh`) fails outright. Run the app binary directly, through the `cladex`
  service, which exists only for this (`profiles: ["cli"]`, so `up` never starts it) and
  works whichever slot is live:
  ```bash
  docker compose run --rm cladex user add rodolfo --name "Rodolfo Flores" --role vendedor
  ```
  This also constrains what the image can ship: only `/cladex` is in the final layer
  (see `Dockerfile`), so `cmd/server/main.go` dispatches `user add/passwd/disable`
  itself when invoked as `cladex user ...` — there is no separate `cladexctl` binary in
  the deployed image, even though one exists in the repo for local dev convenience
  (`go run ./cmd/cladexctl ...`, without the server's env-var assumptions).

## Deploy pipeline

GitHub Actions builds and pushes `ghcr.io/gerrygoo/cladex-web:latest` (public registry,
no auth needed to pull) on every push to `main`. The NAS can't be pushed to directly —
it's behind NAT — so a cron entry on the NAS runs `deploy.sh` every 5 minutes:

```
*/5 * * * * /volume1/docker/cladex/deploy.sh >> /volume1/docker/cladex/deploy.log 2>&1
```

End to end: push to `main` → Actions build (~a couple minutes) → next NAS cron tick
(≤5 min) → live. No manual deploy step in the normal case.

**Deploys don't interrupt service (blue/green, since 2026-10-23).** Production is two
slots of the same service, `cladex-blue` on :8092 and `cladex-green` on :8093, behind
`cladex-proxy`, a small nginx (`nginx.conf`) that owns :8090, where FRP and LAN users
connect. Normally only one slot runs. nginx sends every request to the live slot and
treats the other as a `backup`, tried only when the live one refuses; `deploy.sh` keeps
which is which in `proxy/upstream.conf` (generated on the NAS, not in the repo) and
reloads nginx when it changes. When the image changed, `deploy.sh` checks that nothing
else holds the idle slot's port ([Ports](#ports)), starts the new image there, waits
until that slot's own `/healthz` answers `ok <sha>` (startup runs pending migrations
before it listens), points nginx at it, then stops the old slot, which drains its
in-flight requests. nginx retries any refused connection on the other slot, so no
request fails. If the new slot isn't healthy within 60 s, `deploy.sh` removes it and logs its
last lines to `deploy.log`; the old build keeps serving, and the next tick tries again.
Staging and Litestream are simply recreated in place.

- `deploy.sh active` prints the live slot; `deploy.sh restart` rolls production without a
  new image (after a compose or secrets change).
- **Never `docker compose up -d` the slots by hand**: it recreates both at once, which is
  exactly the outage this avoids. (If it happens, the next `deploy.sh` stops the extra.)
- **Migrations and the old slot.** While the new slot migrates, the old one keeps
  serving. Each migration file is one transaction, so the old slot never sees a
  half-applied file, but once it commits the old code runs against the new schema. So a
  migration is one of two kinds:
  - **Compatible** (the default): adds tables, nullable or defaulted columns, indexes,
    rows. Rolls with no pause. Its writers' lock makes the old slot's writes wait, at most
    `busy_timeout` (5 s), so split a long backfill rather than run it as one statement.
  - **Exclusive**: drops or renames a table or column, rebuilds a table, or changes
    values the old code reads (a status renamed). It starts with `-- cladex:exclusive`
    among its leading `-- cladex:` lines. `deploy.sh` asks the new image first
    (`cladex migrate pending-exclusive`, read-only), and for one of these it stops the old
    slot before starting the new one; meanwhile nginx answers every request with a 503
    "Estamos actualizando Cladex" page that reloads itself (a form submitted then is
    lost). The pause lasts the migration plus startup. If the new build fails and none of
    the exclusive migrations got applied, the old slot serves again; if one did, neither
    build can serve the schema, so the page stays up until a fix is pushed.
  `TestBreakingMigrationsAreExclusive` fails CI for a drop or rename without the marker;
  changed values it can't see are the author's to mark. Often the better move is to
  split: add now, drop in a later release once no running code uses it, and no pause.
- Rolling back to an image older than 2026-10-24 (no `migrate` subcommand) makes
  `deploy.sh` refuse ("can't tell whether the new image's migrations are exclusive").
- Nothing in the app may be per-process: sessions, login throttling and (planned) jobs
  live in SQLite precisely so that two slots can serve side by side.
- Logs are per slot: `docker compose logs cladex-blue cladex-green`.
- `deploy.sh`, `nginx.conf` and `compose.yaml` live in the repo but reach the NAS only by
  hand (see Litestream below for the copy command); the cron pulls only images.

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
scp -O compose.yaml litestream.yml nginx.conf deploy.sh cladex-nas:/volume1/docker/cladex/   # -O, see above
ssh cladex-nas '/volume1/docker/cladex/deploy.sh restart'
```

Verify it caught:

```bash
ssh cladex-nas 'docker logs cladex-litestream --tail 20'   # expect "replica sync" lines
ssh cladex-nas 'ls -R /volume1/docker/cladex/backups/litestream/cladex | head'
```

Restore (the app must be stopped, and the target must not already exist):

```bash
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose stop cladex-blue cladex-green'
# Latest:
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm litestream \
    restore -config /etc/litestream.yml -o /data/cladex-recovered.db /data/cladex.db'
# A specific moment:
ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm litestream \
    restore -config /etc/litestream.yml -timestamp 2026-08-19T14:30:00Z \
    -o /data/cladex-recovered.db /data/cladex.db'
```

Restoring to `cladex-recovered.db` rather than over `cladex.db` is deliberate — inspect
it before swapping anything into place. Then `deploy.sh` (or the next cron tick) starts a
slot again.

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

## Secrets

Facturoporti's token and Cladex's CSD reach the app as files, never as environment
variables (`docker inspect` shows those). Each instance has its own directory, mounted
read-only at `/run/secrets`:

| Instance | Host directory |
|---|---|
| production | `/volume1/docker/cladex/secrets/` |
| staging | `/volume1/docker/cladex/staging/secrets/` |

| File | Contents |
|---|---|
| `facturoporti_token` | The API token (created once by hand with `/token/crear`; the account password never goes on the NAS) |
| `csd_cer` | The CSD certificate, the SAT's `.cer` as is |
| `csd_key` | The CSD private key, the SAT's encrypted `.key` as is |
| `csd_password` | The key's password, alone on one line |
| `resend_api_key` | Resend's API key (`re_…`), sending-only permission, for email notifications |
| `smtp_password` | The mailbox's SMTP password, only when `EMAIL_PROVIDER=smtp` |

- Both directories sit outside `data/` and `backups/`, so neither Litestream nor the
  nightly tarball nor the cloud sync of `backups/` carries them. The durable copy of the
  CSD stays with Cladex (and in 1Password); a lost NAS means putting the files back.
- Ownership `1000:10`, directory `0700`, files `0400`:
  `chown -R 1000:10 secrets && chmod 700 secrets && chmod 400 secrets/*`.
- **Staging only ever gets the sandbox token and the SAT's public test CSD**
  (`internal/facturoporti/testdata`, password `12345678a`). Its token is from Facturoporti's
  public sandbox user (`PruebasTimbrado`, published in their API docs), so staging stamps
  for real against the sandbox; the refresh script sets its emisor to the test CSD's
  company (EKU9003173C9). To invoice a customer there, give it one of the SAT's test RFCs
  (e.g. IIA040805DZ4, régimen 601, C.P. 62661). The real CSD and token exist only in
  production's directory. Even so, only an instance with `APP_ENV=production` talks to
  Facturoporti's production host (`internal/appenv`).
- The app checks them at startup: the key opens with the password, matches the
  certificate, and the certificate is in date. A missing or bad file leaves facturación
  off without stopping the app; the reason is in the `facturacion` startup log line and in
  **Configuración del sistema → Facturación**. After changing a file, roll production
  onto it (`/volume1/docker/cladex/deploy.sh restart`, no downtime).

## Email notifications

`internal/notify` sends email through Resend (or an SMTP mailbox) from a background job
(docs/planning/notificaciones-y-trabajos.md). The settings are environment variables in
`compose.yaml`; the key is a secret file, as above.

| Variable | Value |
|---|---|
| `EMAIL_PROVIDER` | `resend`, `smtp`, or `fake` (logs instead of sending, for local dev); unset turns email off |
| `EMAIL_FROM` | The sender, `Cladex <noreply@cladex.com.mx>` |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME` | Only for `smtp`: e.g. `smtpout.secureserver.net`, `465` (or `587` with STARTTLS), the mailbox address (defaults to `EMAIL_FROM`'s) |
| `NOTIFY_ALLOW` | Outside production, the only addresses and numbers that get anything, comma-separated |

Setting up Resend, once:

1. Create the Resend account and add the domain `cladex.com.mx` (verified 2026-11-08).
   The plan was a subdomain (`avisos.cladex.com.mx`) to keep any reputation problem away
   from the team's mailboxes; the root domain was verified instead, so the sender is
   `noreply@cladex.com.mx`. The root's DMARC is `p=quarantine`, and Resend's DKIM aligns.
2. Add the DNS records Resend shows in GoDaddy's DNS for `cladex.com.mx` (the DKIM `TXT`
   at `resend._domainkey`, the `send` record for SPF and bounces), and wait for Resend to
   mark the domain verified. They don't touch the team mailbox's MX, SPF or DMARC.
3. Create an API key with **sending access** only, and put it in `secrets/resend_api_key`
   (ownership and mode as above). Staging can use the same key: it only sends to
   `NOTIFY_ALLOW`.
4. Roll production onto it: `/volume1/docker/cladex/deploy.sh restart`. The startup log's
   `notificaciones` line and **Configuración del sistema → Notificaciones** should say
   email is on.
5. In the app, put your email in **Mi cuenta → Avisos**, then **Notificaciones → Enviarme
   una prueba**. It should turn *Aceptada* and arrive.

A channel without its settings is off, not fatal. A send that fails is retried by the job
runner and shows its error under **Notificaciones** (and the job's under **Trabajos**).

## Staging

`cladex-staging` is a second container in the same compose project: the same image,
LAN-only on **:8091** (`http://192.168.3.169:8091`), with its own `staging/data` and
`staging/secrets`. Being in the project, the deploy cron updates it with production on
every push to `main`. `APP_ENV=staging` puts a banner on every page, a *PRUEBA*
watermark on every PDF, and keeps facturación on the sandbox.

Its database is a copy of production's, refreshed on demand:

```bash
scp -O scripts/staging-refresh.sh cladex-nas:/volume1/docker/cladex/
ssh cladex-nas 'sh /volume1/docker/cladex/staging-refresh.sh'
```

It copies the live database with sqlite3's online backup (production keeps running),
drops the sessions so production cookies don't open staging, adds the test data
(`cladex seed-staging`) and restarts staging.

**Test data** (`internal/web/seed.go`), everything named or prefixed `PRUEBA`:

- Users `prueba-admin`, `prueba-admin2` (admin) and `prueba-vend1`–`3` (vendedor), with
  one random password the refresh prints.
- Five customers with SAT test RFCs that Facturoporti's sandbox accepts (601 and 612), so
  facturas to them stamp in staging; five suppliers.
- Five products copied from real ones (same cost, weight and materials) with SAT codes:
  two from the quote type priced by the copper price (CCS), one in KGM (invoiced in kilos)
  and one in MTR; two cables by the metre (CCA), one without SAT codes (to see "Faltan
  datos para facturar"); and a piece (a luminaria) in H87. Quote types are found by what
  they do, not by prefix or name, which admins change.
- Proyectos: a prospecto with an overdue follow-up, one at Inminente, one en curso with a
  confirmed supplier O.C. ready to invoice (with a KGM line), one en curso with an
  anticipo and a P.P.D. factura recorded by hand (Pendiente complemento de pago), one
  Perdido, and a draft quote.

`seed-staging` refuses unless `APP_ENV` is staging or development. It writes the setting
`seed_staging_at` last and does nothing once it's there; after a run that failed halfway,
running it again reuses what was created and finishes. A failed seed doesn't keep the
refresh from starting staging: it prints a warning; re-run it with
`docker compose run --rm --no-deps cladex-staging seed-staging`.
Everything done in staging since the last refresh is lost. Staging has no Litestream and
no backups: its data is disposable.

Sign in with your production user and password, as of the last refresh. Cookies don't
tell ports apart, so with production also open by LAN IP (`:8090`) in the same browser,
signing in to one signs you out of the other; production through
`cotizador.cladex.com.mx` is unaffected.

## Running one-off commands against production

- Cross-compile for the NAS's arch (`amd64`) before staging a binary there — the dev
  Mac is arm64, and this box is not.
- For anything that touches the live DB (like the 1.2 catalog import), dry-run first
  against the real production path, confirm it matches a local test run exactly, *then*
  run for real — and this always needs the user's explicit go-ahead, standing commit/push
  authorization doesn't cover writes to production data.
- Clean up scratch artifacts (staged binaries, workbooks, etc.) immediately after a
  one-off run — nothing sensitive should linger on the NAS longer than the run itself.

## Health check

`scripts/nas-check.sh` (skill `nas-check`) is the read-only check: whether production runs
`origin/main` (via `/healthz` on the LAN and publicly), and, when SSH is reachable,
containers, Litestream, the newest backup and disk. As of 2026-10-19 the NAS refused SSH on
port 22 from the LAN (SSH turned off in UGOS), while `:8090` answered.

## Quick reference

| Task | Command |
|---|---|
| Check the app is up | `ssh cladex-nas curl -s localhost:8090/healthz` (staging: `:8091`) |
| Check it from outside, like the `uptime` workflow | `scripts/uptime-check.sh` |
| Refresh staging from production | `ssh cladex-nas 'sh /volume1/docker/cladex/staging-refresh.sh'` |
| See resource usage | `ssh cladex-nas docker stats --no-stream` |
| Tail app logs | `ssh cladex-nas docker compose -f /volume1/docker/cladex/compose.yaml logs -f` |
| Copy a file to the NAS | `scp -O <file> cladex-nas:/volume1/docker/cladex/<dest>` |
| Provision a user | `ssh cladex-nas 'cd /volume1/docker/cladex && docker compose run --rm cladex user add <username> --name "..." --role vendedor'` |
| Force a deploy now (don't wait for cron) | `ssh cladex-nas /volume1/docker/cladex/deploy.sh` |
| Restart production without downtime | `ssh cladex-nas /volume1/docker/cladex/deploy.sh restart` |
| Which slot is live | `ssh cladex-nas /volume1/docker/cladex/deploy.sh active` |
| Deploy history | `ssh cladex-nas grep -E "rolling\|healthy\|live:\|not healthy" /volume1/docker/cladex/deploy.log` |
| Check replication is live | `ssh cladex-nas 'docker logs cladex-litestream --tail 20'` |
| Push a compose/proxy/deploy change (not automatic) | `scp -O compose.yaml litestream.yml nginx.conf deploy.sh cladex-nas:/volume1/docker/cladex/ && ssh cladex-nas /volume1/docker/cladex/deploy.sh restart` |
