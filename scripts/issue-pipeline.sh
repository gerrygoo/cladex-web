#!/usr/bin/env bash
# Headless sweep of the issue pipeline (see .claude/skills/issue-pipeline/SKILL.md).
# Intended for cron, e.g.:
#   */20 * * * * /home/ggo/code/cladex-web/scripts/issue-pipeline.sh >> ~/.cache/cladex-issue-pipeline.log 2>&1
set -euo pipefail

cd "$(dirname "$0")/.."
export PATH="$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin:$PATH"

# Single-run lock so overlapping cron ticks don't collide.
exec 9>/tmp/cladex-issue-pipeline.lock
flock -n 9 || { echo "$(date -Is) already running"; exit 0; }

# Quota guard: runs first, costs nothing. Fails closed when usage is unknown.
if ! reason=$("$(dirname "$0")/usage-guard.sh"); then
  echo "$(date -Is) skipping: $reason"
  exit 0
fi

# Cheap pre-check: skip spawning Claude when no issue needs action.
# Claude's turn = no state label, or a non-🤖 comment after the last 🤖 comment on a
# necesita-info / listo-para-verificar issue, or listo-para-implementar.
needs_action=$(gh issue list --state open --limit 100 --json number,labels,comments | jq '
  [ .[] | select(([.labels[].name] | index("requiere-diseño") | not)
                 and ([.labels[].name] | any(. == "wontfix" or . == "duplicate" or . == "invalid") | not))
        | . as $i | ([$i.labels[].name] | map(select(. == "necesita-info" or . == "listo-para-verificar" or . == "listo-para-implementar"))) as $st
        | ($i.comments | map(.body | startswith("🤖")) ) as $bot
        | select(
            ($st | length) == 0
            or ($st | index("listo-para-implementar"))
            or (($bot | length) > 0 and ($bot | last | not))
          ) ] | length')

if [ "$needs_action" -eq 0 ]; then
  echo "$(date -Is) nothing to do"
  exit 0
fi

# Work in a dedicated worktree, never the shared checkout someone may be using (see
# "Parallel work" in the skill). It sits on the `pipeline` branch, reset to origin/main each
# run, and pushes with `git push origin HEAD:main`.
worktree="${CLADEX_PIPELINE_WORKTREE:-$HOME/code/cladex-web-pipeline}"
git fetch -q origin main
if [ ! -d "$worktree" ]; then
  git worktree add -q -B pipeline "$worktree" origin/main
fi
cd "$worktree"
# Never run over leftovers of a previous run: a human should look at them first.
if [ -n "$(git status --porcelain)" ] || [ -n "$(git log origin/main..HEAD --oneline)" ]; then
  echo "$(date -Is) pipeline worktree $worktree has uncommitted or unpushed work, skipping"
  exit 0
fi
git reset -q --hard origin/main

echo "$(date -Is) $needs_action issue(s) need action"
claude -p "/issue-pipeline (you are in the pipeline worktree on branch 'pipeline'; push with git push origin HEAD:main)" \
  --max-budget-usd "${CLADEX_PIPELINE_MAX_USD:-3}" \
  --permission-mode acceptEdits \
  --allowedTools "Bash(gh issue:*) Bash(gh label:*) Bash(git:*) Bash(go:*) Bash(ls:*) Bash(grep:*) Read Edit Write Glob Grep"
