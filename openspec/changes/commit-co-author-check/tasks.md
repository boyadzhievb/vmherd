# Tasks

M4.10. Implementation tasks (0.1–1.3) go to the `lead-developer` subagent (CLAUDE.md workflow item 9), which hands back an uncommitted diff; the main session reruns the checks and records the results here.

## 0. Failing test first

- [ ] 0.1 Add `.claude/hooks/co-author-check.sh` (100755, `set -uo pipefail`) with today's rule unchanged: read stdin, `grep -qi 'co-authored-by'`, exit 1 with a message on a match, 0 otherwise. Add `TestCoAuthorCheckRefusesOnlyTrailers` in `test/tooling/co_author_check_test.go`: a keyed table that runs `bash .claude/hooks/co-author-check.sh` with each sample on stdin and checks the exit code, and for refusals that stderr names the line. Samples to refuse: a trailer line in a message; an indented one; `co-authored-by:` in lower case; a heredoc command with a trailer line; `-m "Co-Authored-By: x"`; `--message='Co-Authored-By: x'`; `$'subj\n\nCo-Authored-By: x'`; `--trailer "Co-authored-by: x"` and `--trailer=Co-authored-by=x`. Samples to pass: the M4.3 body line ("no amend, reset, rebase, push or Co-Authored-By"); `git commit -m "Ban Co-Authored-By mentions"`; the phrase without a colon at line start; empty input. Run `go test ./test/tooling/` and record here how it fails. Expected: the mention-only samples are refused.

## 1. Check script and callers

- [ ] 1.1 Change the script to the trailer patterns in design.md decision 2, and exit 2 on its own error (e.g. `grep` status 2). `go test ./test/tooling/` passes.
- [ ] 1.2 `.claude/hooks/pre-commit-gate.sh`: replace the inline `grep -qi 'Co-Authored-By'` with `"$REPO_ROOT/.claude/hooks/co-author-check.sh"` fed `$COMMAND` on stdin. Exit 1 means deny and names the line; any other non-zero exit, or a missing or non-executable script, means deny and says the check could not run. Check by hand with the hook's JSON input (`{"tool_input":{"command":"…"}}` on stdin): `git commit -m "Ban Co-Authored-By mentions"` is not denied for co-authorship, and `git commit -m s -m "Co-Authored-By: x"` is denied. Record both.
- [ ] 1.3 `.claude/skills/commit/SKILL.md` step 5: the script runs `bash .claude/hooks/co-author-check.sh < <scratchpad>/message.txt || { echo 'refusing: Co-Authored-By trailer or failed check'; exit 1; }` instead of the inline `grep`. Add `TestCommitGuardsUseCoAuthorCheck` to the same test file: it fails if the hook or the skill no longer names `co-author-check.sh`, or still contains `grep -qi 'co-authored-by'` (any case). `go test ./test/tooling/` passes, and `TestProjectSkillsUseSkillLayout` still passes.

## 2. Verification and record

- [ ] 2.1 `gofmt -l .` is empty; `go vet ./...` passes; `golangci-lint run` reports nothing; `go test ./...` passes; `openspec validate commit-co-author-check --strict` passes. Record the output. No testbed run on .71: no vmherd code changes. The CI run after the user pushes covers GNU grep (Linux).
- [ ] 2.2 Independent review of the diff with the `code-reviewer` agent (Opus, high); record each finding here as fixed or rejected, with one line of reasoning.
- [ ] 2.3 The commit for this change goes through the updated skill. Record that the guard passed its message, and tick M4.10 in milestone.md with a short Result note and the M4 done count in the same commit.

## Workflow follow-up

- Archive the change with the `openspec-archive-change` skill (no specs to sync, `skip_specs: true`) and point M4.10's link at `openspec/changes/archive/<date>-commit-co-author-check/`.
