---
name: commit
description: Commit the current changes following vmherd conventions (checks first, staged by name, message as in the history, commit run from a script file)
disable-model-invocation: true
---

# Commit Changes

Create a commit for the current changes following vmherd conventions. The user decides when to commit; the user always pushes.

## Steps

1. Run `git status` and `git diff` (and `git diff --staged`) to review what changed.
2. Run the checks and stop on any failure. Report the failure; fix it only if it belongs to the change being committed (CLAUDE.md workflow item 6: no opportunistic changes). Never commit around it.
   - `gofmt -l .` prints nothing
   - `go vet ./...`
   - `golangci-lint run`
   - `go test ./...`
   - `openspec validate --all --strict`, when anything under `openspec/` changed
3. Stage the relevant files by name (`git add <path>...`), never `git add -A` or `git add .`.
4. Split unrelated work into separate commits, one change per commit. To commit only part of a file:
   - build the wanted version in a temp file in the scratchpad,
   - `git hash-object -w <temp file>` prints a blob id,
   - `git update-index --cacheinfo "$(git ls-files -s <path> | cut -d' ' -f1)",<blob>,<path>` stages that version; the working tree keeps the rest. The mode comes from the index so an executable file (e.g. `.claude/hooks/pre-commit-gate.sh`, 100755) keeps its exec bit. A file not tracked yet has no index entry: stage it whole with `git add`.
   - These lines go in the script (step 5) instead of `git add` for that path; never also `git add` it, which restages the whole file.
   - The checks in step 2 ran on the working tree, not on what a partial commit contains. For a partial commit of Go code, check the commit builds in a throwaway worktree: `git worktree add <scratchpad>/wt HEAD`, then `go build ./... && go test ./...` there, then `git worktree remove <scratchpad>/wt`.
5. Write the staging and commit commands, with the message, into a script file in the session's scratchpad directory (e.g. `<scratchpad>/commit.sh`, message via `git commit -F <scratchpad>/message.txt`) and run that file. The pre-commit hook (`.claude/hooks/pre-commit-gate.sh`) matches command text, so a direct `git commit` command is gated; the script file is why step 2 runs the checks first.
6. Run `git log --oneline -3` and `git status --short` and report the new commit and anything left uncommitted.

## Commit Message Format

Follow the history (`git log -10`). The first line says what changed, with the milestone.md item and task in parentheses:

```
Record the second Terramino run on .71 (M6.9, task 2.2)
```

Proposing and archiving an OpenSpec change use a prefix instead:

```
M4.3: propose OpenSpec change project-skills-layout
Mx.y: archive OpenSpec change <name> (spec <capability>)
```

The ` (spec <capability>)` suffix is left out when the change sets `skip_specs: true`.

After a blank line, the body says why, and the results: checks run, test output, how a test-first test failed before the fix (CLAUDE.md workflow item 7), testbed results.

## Never

- `--amend`, `git reset <commit>` in any mode, or rebase: fix mistakes with a new commit. To unstage a file, use `git restore --staged <path>`.
- Push: the user pushes.
- `Co-Authored-By` lines or `[skip ci]`.
- `rm -rf`.
