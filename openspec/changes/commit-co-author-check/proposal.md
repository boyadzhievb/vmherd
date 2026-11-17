# Proposal

Implements **M4.10 — Refuse only real Co-Authored-By trailers** (milestone.md, M4).

## Why

Commits must not carry a `Co-Authored-By` trailer, and two guards enforce that: the commit skill's script runs `grep -qi 'co-authored-by'` on the message file, and the pre-commit hook runs `grep -qi 'Co-Authored-By'` on the command text. Both match the phrase anywhere. On 2026-11-17 the skill refused M4.3's implementation commit because its body only listed the rule ("no … push or Co-Authored-By"). The message had to be reworded to get past the guard. The hook would block `git commit -m "Ban Co-Authored-By mentions"` the same way. Neither guard has a test, so this went unnoticed.

## What Changes

- A new script, `.claude/hooks/co-author-check.sh`, reads text on stdin and exits 1, naming the line, only when it finds a trailer-shaped `Co-Authored-By:` (case-insensitive). That means the phrase followed by a colon at the start of a line, after optional whitespace or an opening quote, or as the value of `-m`/`--message`. A mention inside a sentence passes.
- The commit skill's step 5 calls the script on `<scratchpad>/message.txt` instead of the inline `grep`.
- The pre-commit hook pipes the command text through the same script instead of its inline `grep`. Its other checks are unchanged.
- A new table test in `test/tooling/` runs the script on sample messages and commands: trailers in their different positions and cases are refused, and mentions pass.

Out of scope: the hook's other checks (gofmt, vet, lint, tags), the `[skip ci]` rule, and the skills' layout test.

Vagrant has no equivalent: this is developer tooling for working on vmherd, not vmherd behaviour.

## Capabilities

### New Capabilities
None. Developer tooling only, so `.openspec.yaml` sets `skip_specs: true`.

### Modified Capabilities
None.

## Impact

- New `.claude/hooks/co-author-check.sh` (100755).
- `.claude/hooks/pre-commit-gate.sh`: the Co-Authored-By check calls the script.
- `.claude/skills/commit/SKILL.md`: step 5 calls the script.
- New test file in `test/tooling/` (no build tag; runs in `go test ./...` and CI). It needs `bash`, which CI and the Mac have.
- milestone.md: M4.10 links this change; on completion, ticked with a Result note and the M4 done count.
