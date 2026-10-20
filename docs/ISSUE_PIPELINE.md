# Cladex — Issue pipeline

How GitHub issues become changes. Issues are mostly written by a non-technical
person, in Spanish; Claude triages them, asks questions, and implements the small
ones. The process itself lives in `.claude/skills/issue-pipeline/SKILL.md` (run it as
`/issue-pipeline 22`, or with no argument to sweep); this file covers the setup
around it.

## Flow in one paragraph

Triage against the code and `docs/guia/` → ask the author (comments in Spanish,
prefixed `🤖 **Claude:**`) until clear → classify. **Small and well-defined**: Claude
implements with tests, pushes straight to `main`, and once `scripts/nas-check.sh` shows
it live in production, comments how to verify, asks the author to take a look and close,
and labels `listo-para-verificar`. **Large or sensitive** (migration, major restructure, money,
regulatory/CFDI/fiscal, permissions, business-model state changes; read-only reporting
that only displays existing money figures is not sensitive): label
`requiere-diseño` and stop; Gerry leads. Commits say `Refs #N`, never `Closes #N` —
**only the issue author closes an issue.**

## Labels

`necesita-info` · `listo-para-implementar` · `requiere-diseño` · `listo-para-verificar`.
One state label at a time; no label means untriaged. Claude's comments come from
Gerry's GitHub account, so the `🤖` prefix is what distinguishes whose turn it is.

> Pushing to `main` deploys (`.github/workflows/deploy.yml`, gated on tests).

## Automation (off by default)

`scripts/issue-pipeline.sh` is the cron entry point. Nothing schedules it yet. To
turn it on:

```
*/20 * * * * /home/ggo/code/cladex-web/scripts/issue-pipeline.sh >> ~/.cache/cladex-issue-pipeline.log 2>&1
```

Each run goes through these gates, cheapest first, and exits quietly if any fails:

1. **Lock** — one run at a time (`flock`).
2. **Usage guard** — `scripts/usage-guard.sh` skips when the 5-hour or weekly
   usage is over 70%. It **fails closed**: missing, stale (>30 min) or unparseable
   usage data means don't run.
3. **Anything to do?** — a `gh`+`jq` check for issues where it is Claude's turn.
4. **Own worktree** — the run never touches the shared checkout. It works in a dedicated
   worktree (`~/code/cladex-web-pipeline`, branch `pipeline`, reset to `origin/main`) and
   pushes `HEAD:main`; it skips if that worktree has leftovers from a previous run. Any
   parallel work on this repo follows the same rule: its own worktree, and only the main
   session (or this cron run) pushes `main`.
5. **Spend cap** — `claude -p --max-budget-usd` (default 3) bounds the run; the
   skill also handles at most 3 issues per run.

Tunables (env): `CLADEX_USAGE_MAX_5H`, `CLADEX_USAGE_MAX_7D` (default 70),
`CLADEX_USAGE_MAX_AGE` (seconds, default 1800), `CLADEX_USAGE_FILE`,
`CLADEX_PIPELINE_MAX_USD` (default 3).

### Where usage data comes from

There is no documented way to read subscription usage outside a session. The
statusline JSON (`rate_limits.five_hour` / `seven_day` `.used_percentage`) is the only
source, so `~/.claude/statusline.sh` (registered as `statusLine` in
`~/.claude/settings.json`; per-machine, not in this repo) writes it to
`~/.cache/claude-statusline.json`. Consequence: the file only refreshes while an
interactive Claude Code session is open, so **cron effectively runs only while someone
is using Claude.** Check it with `jq .rate_limits ~/.cache/claude-statusline.json`.

To reproduce on another machine, the script must save stdin to that file
(atomically) and print a status line.
