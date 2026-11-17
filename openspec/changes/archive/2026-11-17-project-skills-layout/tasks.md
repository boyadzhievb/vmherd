# Tasks

M4.3. Implementation tasks (0.1–1.4) go to the `lead-developer` subagent (CLAUDE.md workflow item 9), which hands back an uncommitted diff; the main session reruns the checks and records the results here.

## 0. Failing test first

- [x] 0.1 Add `test/tooling/skills_test.go` (package `tooling`, no build tag) with `TestProjectSkillsUseSkillLayout`. For every entry in `.claude/skills/` (following symlinks), it checks: a directory, containing `SKILL.md`, frontmatter that parses, `name` equal to the directory name, a non-empty `description`. All problems are reported in one run, each naming the entry. Run `go test ./test/tooling/` against today's skills and record the output here. Expected: it fails on `commit.md`, `test.md`, `reduce-comments.md` and `improve.md` (not directories), and passes the six `openspec-*` entries. — 2026-11-17, `lead-developer`: before any skill changed, `go test ./test/tooling/` failed with `commit.md: a skill must be a directory .claude/skills/commit/ containing SKILL.md`, the same for `improve.md`, `reduce-comments.md` and `test.md`; the six `openspec-*` symlinks passed. Added: `TestCheckSkillEntryReportsEachProblem`, which runs the check on fixture skills (flat file, no SKILL.md, no or unclosed frontmatter, invalid YAML, name mismatch, no description, one valid skill), so a check that always passes cannot hide a broken skill.
- [x] 0.1a (added 2026-11-17) `TestCheckSkillEntryReportsEachProblem`: the layout check run on fixture skills, so a check that always passes cannot hide a broken skill. Extended in review rework (3.2 L1–L3) with `TestListSkillEntryNamesSkipsHiddenEntries`. Before the rework fixes, three new cases failed as expected: `expected entries ["commit.md" "test"], got [".DS_Store" "commit.md" "test"]`; `leading_space_before_opening_delimiter: expected 1 problems ["test/SKILL.md: no frontmatter"], got 0`; `indented_delimiter_inside_a_block_scalar: expected 0 problems [], got 1: [… frontmatter has no description …]`. All pass after.

## 1. Skills

- [x] 1.1 `git mv .claude/skills/commit.md .claude/skills/commit/SKILL.md` and rewrite it, following ccattler's layout:
  - frontmatter `name: commit`, a description, `disable-model-invocation: true`;
  - steps: review `git status` and `git diff`; run `gofmt -l .`, `go vet ./...`, `golangci-lint run` and `go test ./...` (plus `openspec validate --all --strict` when `openspec/` changed), stopping on any failure; stage by name; split unrelated work into separate commits (part of a file via `git update-index --cacheinfo` with a temp copy); write the commit command and message into a script file in the scratchpad and run it (the pre-commit hook matches command text);
  - message format as in the history: `<what changed> (Mx.y, task n.m)`, then a body with why and results;
  - never: `--amend`, reset, rebase, push, Co-Authored-By, `[skip ci]`, `rm -rf`. — 2026-11-17: as specified; adds `git diff --staged` to the review step and a closing `git log --oneline -3` / `git status --short`. The message is passed with `git commit -F <scratchpad>/message.txt`.
- [x] 1.2 `git mv .claude/skills/test.md .claude/skills/test/SKILL.md`: frontmatter `name: test` and a description, model-invocable (design decision 1). Steps: `gofmt -l .`, `go vet ./...`, `go vet -tags integration ./...` (compiles the libvirt tests without running them), `golangci-lint run`, `go test ./...` printing only failures, rerun a failing package with `-v -run <Test>`, report a summary. Note that the `integration` tests run on the testbed, never on the Mac. — 2026-11-17: as specified (filter `grep -v -e '^ok' -e 'no test files'`). Claude Code listed `test` as a model-invocable skill in this session right after the move.
- [x] 1.3 `git mv .claude/skills/reduce-comments.md .claude/skills/reduce-comments/SKILL.md`: add `name`, `argument-hint: "[file or directory]"` and `disable-model-invocation: true`; the body stays as it is. — 2026-11-17: frontmatter only (`name`, the existing description, `disable-model-invocation: true`, `argument-hint`); body unchanged.
- [x] 1.4 `git rm .claude/skills/improve.md`. Check nothing refers to the old paths: `grep -rn -e 'skills/[a-z-]*\.md' -e 'improve' .claude CLAUDE.md milestone.md README.md`; fix or record any hit. `go test ./test/tooling/` passes. — 2026-11-17: grep hits are only the English word "improvement" in `code-reviewer.md` and `performance-engineer.md`, and M4.3's own Fix text in milestone.md; no reference to the old paths outside this change. After the changes `go test -count=1 ./test/tooling/` passes; main session reran gofmt, `go vet ./...` and `-tags integration`, `golangci-lint run` (0 issues), `go test -count=1 ./...` (all ok) and `openspec validate --all --strict` (10 passed).

## 2. Check in Claude Code

- [x] 2.1 In a new Claude Code session in the repository: `commit`, `test` and `reduce-comments` appear in the skill list, and `improve` does not. Record what the list showed. A testbed run on .71 is not needed: no vmherd code changes. — 2026-11-17, new session (model's skill list): `test` and the six `openspec-*` skills listed, `improve` absent. `commit` and `reduce-comments` are not in the model's list, as intended by `disable-model-invocation: true`; the user then ran `/commit` in that session, and it loaded from `.claude/skills/commit/SKILL.md`. `reduce-comments` has the same layout and frontmatter keys and passes `TestProjectSkillsUseSkillLayout`, but it was not invoked. The user confirmed `improve` is not in their `/` menu. Later in the same session: `/reduce-comments` ran from `.claude/skills/reduce-comments/SKILL.md`, and `/improve` was reported as not available.

## 3. Verification and record

- [x] 3.1 `gofmt -l .` is empty; `go vet ./...` passes; `golangci-lint run` reports nothing; `go test ./...` passes; `openspec validate project-skills-layout --strict` passes. Record the output. — 2026-11-17, main session: `gofmt -l .` empty; `go vet ./...` and `go vet -tags integration ./...` clean; `golangci-lint run`: `0 issues.`; `go test -count=1 ./...`: every package ok, including `test/tooling`; `openspec validate project-skills-layout --strict`: `Change 'project-skills-layout' is valid`.
- [x] 3.2 Independent review of the diff with the `code-reviewer` agent; record each finding here as fixed or rejected, with one line of reasoning. — 2026-11-17, `code-reviewer` (Opus, high): request changes, 1 medium, 8 low, 8 nits; each verified in the files by the main session. Rework round 1 by the same `lead-developer`:
  - fixed (medium) M1 — partial staging hardcoded mode 100644; `scripts/release.sh`, `reference/libvirt-vm-ccattler.sh` and the pre-commit hook are 100755, so the hook would lose its exec bit and stop running. The mode now comes from `git ls-files -s`; a file not yet tracked is staged whole (main session).
  - fixed (low) L1 — `TrimSpace` on `---` lines accepted ` ---` and closed the frontmatter early on an indented `  ---` in a block scalar; now only trailing whitespace is allowed (fixtures failed first, see 0.1a).
  - fixed (low) L2 — `.DS_Store` failed the real-tree test with an empty name; hidden entries are skipped, a flat `*.md` still fails (fixture failed first).
  - fixed (low) L3 — fixture test missed broken symlink, followed symlink, CRLF and two problems in one entry; added, keyed table, exact counts (N4).
  - fixed (low) L4 — `test` skill's grep filter exited 1 when all passed and removed the `ok` lines it then counted; output goes to a file, `go test`'s exit status counts.
  - fixed (low) L5, lighter form — the skill states the checks run on the working tree and gives a throwaway-worktree build for partial commits of Go code; not made mandatory (heavy for doc-only splits).
  - fixed (low) L6 — a path staged with `update-index` is never also `git add`ed in the script.
  - fixed (low) L7 — archive example carries `(spec <capability>)`, left out for `skip_specs`.
  - fixed (low) L8 — the Never list bans `reset <commit>`, not unstaging (`git restore --staged`).
  - fixed (nits) N1 frontmatter comment says it is vmherd's rule; N2 dead `len == 0`; N3 stat message; N5 integration runner named; N6 fix only what belongs to the change; N7 the script refuses Co-Authored-By; N8 the added test has its own line (0.1a).
  - rejected: none.
  Checks after the rework (main session): gofmt empty, `go vet` and `-tags integration` clean, `golangci-lint run` 0 issues, `go test -count=1 ./...` exit 0 (11 packages ok; `test/tooling` 17 tests and subtests pass), `openspec validate --all --strict` 10 passed.
- [x] 3.3 Tick M4.3 in milestone.md with a short Result note, and update the Progress table (M4 done count) in the same commit. — 2026-11-17: ticked, M4 done count 1 → 2.

## Workflow follow-up

- Archive the change with the `openspec-archive-change` skill (no specs to sync, `skip_specs: true`) and point M4.3's link at `openspec/changes/archive/<date>-project-skills-layout/`.
