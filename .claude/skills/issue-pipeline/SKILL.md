---
name: issue-pipeline
description: Triage and implement Cladex GitHub issues. Use when asked to work on, triage or process issues (e.g. `/issue-pipeline 22`, or no argument to sweep all open issues that need action).
---

# Issue pipeline

Issues are mostly written by a **non-technical person** (Emilio, `emiliofloressolbes-prog`) in
Spanish. Claude's comments are posted from Gerry's GitHub account (`gerrygoo`), so every
comment Claude writes **must start with `🤖 **Claude:**`** — that marker is how turns are told apart.

## State machine (labels)

| Label | Meaning | Whose turn |
|---|---|---|
| *(none)* | New / unseen | Claude: triage |
| `necesita-info` | Claude asked questions | Author — Claude acts only if a non-🤖 comment came after the last 🤖 one |
| `listo-para-implementar` | Scope clear, small, not sensitive | Claude: implement |
| `requiere-diseño` | Large or sensitive | **Gerry** — Claude does nothing until Gerry comments a decision |
| `listo-para-verificar` | Implemented, pushed | Author — verify and close |

Exactly one state label at a time; swap with `gh issue edit N --add-label X --remove-label Y`.
Ignore issues already labeled `wontfix`, `duplicate`, `invalid`.

## Step 1 — Triage

Read the issue and all comments, then check it **against the code and docs**: `docs/guia/`
(current user-facing behavior), `docs/planning/`, `docs/PLAN.md`, and the relevant `internal/` code.
Decide:

- **Already exists / premise is wrong / contradicts other behavior** → comment what you found, ask. `necesita-info`.
- **Ambiguous or missing detail** → ask. `necesita-info`.
- **Clear** → classify (step 3).

## Step 2 — Clarify

Write in Spanish, plain language, no jargon, no file names. Ask **concrete, answerable** questions
(offer the options you see) instead of open ones. Batch all questions in one comment. When answers
remove all ambiguity, post `🤖 **Claude:** Entendí esto: …` (short bullets) and move on — don't wait
for a confirmation unless the summary reveals a real choice.

## Step 3 — Classify

**Large / needs Gerry → `requiere-diseño`**, if ANY of:
- needs a DB migration or schema change
- major restructuring of a component or flow
- **sensitive: touches money (prices, totals, payments, invoicing, purchase orders) or regulatory behavior (CFDI, fiscal data, PAC, audit/permissions)**
  (Exception: **read-only reporting** — dashboards/tables that only display existing totals, margins or pipeline amounts without changing how they're computed or stored — counts as small.)
- changes the business model/state machine (quote/proyecto states)
- would take more than a focused change in a few files

For these: post a plain-language comment to the author ("esto es grande, Gerry lo revisará") and
write up options + your recommendation for Gerry in the *terminal/run output* (not in a long public
comment). Then stop. Gerry leads; implement only after he decides.

**Small and well-defined → `listo-para-implementar`**, then implement.

## Step 4 — Implement (small path only)

1. `git pull --rebase origin main`; work on `main` (no branches/PRs).
2. Follow `docs/TESTING.md` and `docs/UX_QUALITY.md`. Add/adjust tests. `go vet ./... && go test ./...`
   must pass (needs `typst` on PATH). If tests can't run, do not push.
3. Update `docs/guia/` if user-visible behavior changed.
4. Commit message style: imperative, one line (see `git log`), body ends with `Refs #N`.
   **Never `Closes/Fixes #N`** — only the author closes. Include the Co-Authored-By trailer.
5. `git push origin main`. **Pushing to main deploys** (CI `deploy.yml`, gated on tests), so never push
   red or half-done work.
6. Comment: what changed in plain Spanish, **how to verify** (numbered steps in the app), ask them to
   check and close. Label `listo-para-verificar`.

If implementation reveals it's bigger or more sensitive than thought: don't push; revert local
changes, move to `requiere-diseño`, explain to Gerry.

## Step 5 — Follow-ups

If the author reports a problem on a `listo-para-verificar` issue, treat it as new context: remove the
label and return to step 1/2. **Never close issues.** (Issues authored by Gerry: he closes them.)

## Sweep mode (no argument / cron)

List open issues with `gh issue list --json number,labels,comments,author`. Act only on issues where
it's Claude's turn per the table above. Process at most 3 per run, oldest first. Print a one-line
summary per issue of what you did. If nothing needs action, print `nothing to do` and exit.
