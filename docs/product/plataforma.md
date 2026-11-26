# Track 6 · Plataforma

What every other track stands on: auth and roles, the audit log, background jobs,
notifications, PDFs, and running it all on the NAS (deploys, backups, staging,
monitoring). Users don't ask for this track by name; they notice it when it fails.

## As built

- **Stack**: Go + templ + htmx, SQLite (pure Go), Typst for PDFs, one binary in a
  distroless image on a UGREEN NAS behind FRP on a Hetzner VPS.
- **Deploys**: CI (staticcheck, govulncheck, templ drift) → GHCR → blue/green on the NAS
  with `/healthz` reporting the SHA; migrations that the old build can't run alongside
  pause production (`-- cladex:exclusive`).
- **Data safety**: Litestream (point-in-time) plus nightly tarballs kept 14 days;
  restore tested. Offsite depends on NAS cloud sync (see baseline).
- **Staging** at pruebas.cladex.com.mx, built from synthetic data plus production's
  configuration (#81), sandbox PAC.
- **Audit log** by triggers on every business table; the dashboards derive history
  from it.
- **Jobs** (N1) in SQLite with leases and backoff; **notifications** (N2) by email
  through Resend; avisos (N3) per user.
- **Roles**: `admin` and `vendedor` only.
- **Monitoring**: an uptime workflow every 10 min, Beszel on the NAS, Trabajos and
  Notificaciones dashboards.

## Signals

- **72 commit subjects** on ops: about a quarter of all work. Reliability has been
  bought deliberately (blue/green, staging rebuilt from synthetic data); keep it
  proportional to a 4-user app.
- **Long-open gaps**: finer roles and approvals (backlog since 2026-08-12), WhatsApp
  (#84, blocked on Meta), screen-reader review (#10), GoDaddy SMTP (N6).
- Storage is dominated by uploads, not data ([baseline](baseline.md#storage)).

## Growth edges

| # | Edge | Trigger | Functional | Non-functional | Size | Depends on |
|---|---|---|---|---|---|---|
| P1 | **Roles: sysadmin / admin / ventas, and approvals** | Someone other than an admin needs to stamp, confirm O.C.s or approve discounts; or headcount grows | Permissions per action rather than per role pair; lesser roles request, an admin approves (C2) | Every handler's guard changes: needs a permissions table tested route by route (the `authed`/`admin` helpers are the seam). Audit of approvals | L / Decision / med | Clarity on who does what at Cladex |
| P2 | **WhatsApp for the team** (N4) | Blocked: Meta business verification (#84) | Avisos on WhatsApp, delivery visible | Templates approved by Meta; webhook endpoint (first public non-login endpoint); per-message cost | M / External (Meta) / med | Meta |
| P3 | **Client-facing notifications** (N5) | C1, S2 or F4 is picked | Attachments, resend, bounce webhook, client WhatsApp with opt-in | One attachment model for quotes, O.C.s and facturas. Opt-in record for clients (privacy) | M / Internal / high | — |
| P4 | **Uploads out of SQLite** | DB > 500 MB or tarball > 250 MB, or before K1/K4 add more files | No visible change | Content-addressed files under the data dir, referenced by hash; migration copies blobs out in batches (exclusive deploy); backups include the folder; Litestream only for the DB | M / Internal / high | — |
| P5 | **Offsite backup, verified** | Now, if NAS cloud sync isn't actually on: one disk/NAS failure loses everything | Nothing visible | Nightly offsite copy of tarball + Litestream; a quarterly restore drill from offsite with its time written down (RTO) | S / Internal / high | — |
| P6 | **Latency and error visibility** | The first "it's slow" or a failed job nobody saw | Admins see p95 per route and failed jobs; failures email the sysadmin | Lightweight: request timing to logs plus a daily summary job; alert via the existing notifications | S / Internal / high | — |
| P7 | **2FA / SSO** | Anyone outside the company gets an account, or a vendedor's password leaks | Second factor for admins at least | TOTP is enough; recovery codes; the admin CLI resets it | S–M / Internal / high | — |
| P8 | **Accessibility pass** (#10) | Open since October | Screen reader and keyboard work across the main flows | `docs/UX_QUALITY.md` sets the bar; done flow by flow | M / Internal / med | — |
| P9 | **Mobile-first screens** | Vendedores working from the phone on visits | Mis pendientes, a proyecto's comment and next date, a lead, all comfortable at 360 px | Lists are already tested at 320 px (#9); this is about flows, not overflow | M / Decision / med | — |

## Notes

- P5 is the one edge here with a real downside if it's not done: confirm the NAS cloud
  sync is on, and write the restore time down.
- P1 has been deferred four months without hurting; its trigger is organizational, so
  ask Emilio rather than building ahead of need. C2 (margin approvals) may be the first
  concrete reason.
- P3 is shared plumbing for three tracks (C1, S2, F4, K4): one design, then each of those
  becomes XS–S.
