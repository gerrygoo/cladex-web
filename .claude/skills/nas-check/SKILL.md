---
name: nas-check
description: Check on the production NAS running Cladex — is the app up, is the latest main deployed, are containers, Litestream, backups and disk healthy. Use when asked to check the NAS, production, the deploy, or whether a push went live.
---

# NAS check

Read-only. Never changes anything on the NAS, the VPS or GitHub. Background and every
operational detail live in `docs/NAS_OPERATIONS.md` — read it before doing anything
beyond this check.

## Run it

```bash
scripts/nas-check.sh
```

It checks, cheapest first:

1. **Deploy**: `origin/main`'s SHA, the last GitHub Actions run, and the SHA that
   `/healthz` reports on the LAN (`http://192.168.3.169:8090/healthz`) and publicly
   (`https://cotizador.cladex.com.mx/healthz`, through the VPS/FRP). Production should run
   `main` within ~15 min of the deploy run finishing (Actions build → NAS cron pulls every
   5 min).
2. **Over SSH** (`ssh cladex-nas`), only if port 22 answers: the live blue/green slot,
   container state, image build time, Litestream errors in the last 30 min, age of the
   newest nightly backup tarball, disk use of `/volume1`, memory/CPU per container.

Exit status: `0` all good, `1` something needs attention (⚠), `2` something couldn't be
checked (✗).

## Reading the result

- **LAN fails, public fails**: the app or the NAS is down.
- **LAN ok, public fails**: the VPS (nginx/FRP) side; `ssh cladex-vps` needs the
  1Password SSH agent, which may not be available here — hand it to the user.
- **Production SHA behind main long after the run**: the NAS pull cron is stuck. Force a
  redeploy only with the user's go-ahead (`docs/NAS_OPERATIONS.md` → Quick reference).
- **SSH port closed**: SSH is turned off in UGOS (Control Panel → Terminal). As of
  2026-10-19 it was off; the HTTP checks still say whether the right build is live. Ask
  the user to enable it if the deeper checks are needed. Don't treat it as the NAS being
  down.
- Anything that needs `sudo` (crontab) or writes to production data: hand to the user.

## Report

Summarize in a few lines: deployed SHA vs `main`, up/down, and any ⚠/✗ with the likely
cause from the list above. Don't paste the whole output unless asked.
