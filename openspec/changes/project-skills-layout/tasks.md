# Tasks

M4.3. Implementation tasks (0.1–1.4) go to the `lead-developer` subagent (CLAUDE.md workflow item 9), which hands back an uncommitted diff; the main session reruns the checks and records the results here.

## 0. Failing test first

- [ ] 0.1 Add `test/tooling/skills_test.go` (package `tooling`, no build tag) with `TestProjectSkillsUseSkillLayout`. For every entry in `.claude/skills/` (following symlinks), it checks: a directory, containing `SKILL.md`, frontmatter that parses, `name` equal to the directory name, a non-empty `description`. All problems are reported in one run, each naming the entry. Run `go test ./test/tooling/` against today's skills and record the output here. Expected: it fails on `commit.md`, `test.md`, `reduce-comments.md` and `improve.md` (not directories), and passes the six `openspec-*` entries.

## 1. Skills

- [ ] 1.1 `git mv .claude/skills/commit.md .claude/skills/commit/SKILL.md` and rewrite it, following ccattler's layout:
  - frontmatter `name: commit`, a description, `disable-model-invocation: true`;
  - steps: review `git status` and `git diff`; run `gofmt -l .`, `go vet ./...`, `golangci-lint run` and `go test ./...` (plus `openspec validate --all --strict` when `openspec/` changed), stopping on any failure; stage by name; split unrelated work into separate commits (part of a file via `git update-index --cacheinfo` with a temp copy); write the commit command and message into a script file in the scratchpad and run it (the pre-commit hook matches command text);
  - message format as in the history: `<what changed> (Mx.y, task n.m)`, then a body with why and results;
  - never: `--amend`, reset, rebase, push, Co-Authored-By, `[skip ci]`, `rm -rf`.
- [ ] 1.2 `git mv .claude/skills/test.md .claude/skills/test/SKILL.md`: frontmatter `name: test` and a description, model-invocable (design decision 1). Steps: `gofmt -l .`, `go vet ./...`, `go vet -tags integration ./...` (compiles the libvirt tests without running them), `golangci-lint run`, `go test ./...` printing only failures, rerun a failing package with `-v -run <Test>`, report a summary. Note that the `integration` tests run on the testbed, never on the Mac.
- [ ] 1.3 `git mv .claude/skills/reduce-comments.md .claude/skills/reduce-comments/SKILL.md`: add `name`, `argument-hint: "[file or directory]"` and `disable-model-invocation: true`; the body stays as it is.
- [ ] 1.4 `git rm .claude/skills/improve.md`. Check nothing refers to the old paths: `grep -rn -e 'skills/[a-z-]*\.md' -e 'improve' .claude CLAUDE.md milestone.md README.md`; fix or record any hit. `go test ./test/tooling/` passes.

## 2. Check in Claude Code

- [ ] 2.1 In a new Claude Code session in the repository: `commit`, `test` and `reduce-comments` appear in the skill list, and `improve` does not. Record what the list showed. A testbed run on .71 is not needed: no vmherd code changes.

## 3. Verification and record

- [ ] 3.1 `gofmt -l .` is empty; `go vet ./...` passes; `golangci-lint run` reports nothing; `go test ./...` passes; `openspec validate project-skills-layout --strict` passes. Record the output.
- [ ] 3.2 Independent review of the diff with the `code-reviewer` agent; record each finding here as fixed or rejected, with one line of reasoning.
- [ ] 3.3 Tick M4.3 in milestone.md with a short Result note, and update the Progress table (M4 done count) in the same commit.

## Workflow follow-up

- Archive the change with the `openspec-archive-change` skill (no specs to sync, `skip_specs: true`) and point M4.3's link at `openspec/changes/archive/<date>-project-skills-layout/`.
