# Design

## Context

See proposal.md (Why). Two guards keep `Co-Authored-By` trailers out of commits:

- The commit skill's step 5 runs `grep -qi 'co-authored-by' <scratchpad>/message.txt` inside the commit script, because the hook never sees a commit made from a script file.
- `.claude/hooks/pre-commit-gate.sh` runs `grep -qi 'Co-Authored-By'` on the text of any Bash command containing `git commit`, then runs gofmt, go vet and golangci-lint.

Git only treats `Token: value` lines as trailers when they start a line in the message's last paragraph, or when they are added with `git commit --trailer`. A phrase inside a sentence is never a trailer.

## Goals / Non-Goals

**Goals:**
- Refuse every way a `Co-Authored-By` trailer can get into a commit from the message file or the command text.
- Let a message or command that only mentions the phrase pass.
- Keep both guards on one tested rule, so they can't drift apart.

**Non-Goals:**
- Reading a `-F <file>` message from the hook. The hook only sees command text, as today. The skill checks its own message file.
- Changing the hook's other checks.

## Decisions

1. **One shell script, `.claude/hooks/co-author-check.sh`, shared by both guards.** It reads stdin and exits 0 when it finds no trailer and 1 when it finds one, printing the offending line to stderr. It exits 2 on its own error, for example when `grep` fails. The hook pipes `$COMMAND` into it, and the skill runs it on the message file.
   - Alternative rejected: the same regex copied into both places. That copying is how they would drift apart, and a test of one copy proves nothing about the other.
   - Alternative rejected: a Go helper. The hook and the skill are shell, and a binary would need building before each session.
   - It lives in `.claude/hooks/` beside the hook, the stricter of its two callers.
2. **What counts as a trailer.** These patterns are case-insensitive, and `X` stands for `co-authored-by`:
   - `X:` at the start of a line, after optional whitespace and an optional opening quote (`"` or `'`). This covers message files, heredocs in command text, and multi-line `-m "…"` values.
   - `X:` right after a literal `\n` in the command text, for `$'…\nCo-Authored-By: …'` quoting.
   - `X:` as the start of a `-m` or `--message` value (`-m "Co-Authored-By: x"`, `--message=…`). A second `-m` is its own paragraph and becomes the last one.
   - `X:` or `X=` as a `--trailer` value.
   Anything else passes, including `Co-Authored-By` with no colon after it and the phrase in mid-sentence.
3. **Both callers fail closed.** If the script is missing or exits with anything but 0, the hook denies the commit and names the reason, and the skill's script stops before `git commit`. Today a failing `grep` (exit 2) counts as "no match" and the commit passes. After this change it blocks the commit.
4. **The test lives in `test/tooling/` as a Go table test** (`TestCoAuthorCheckRefusesOnlyTrailers`). It runs `bash .claude/hooks/co-author-check.sh` on each sample's stdin and checks the exit code, and on a refusal that stderr names the line. A second test, `TestCommitGuardsUseCoAuthorCheck`, reads the hook and the skill and fails if either no longer calls the script or still has an inline `grep -qi 'co-authored-by'`.
5. **Test first.** The first task moves today's rule into the script unchanged (`grep -qi 'co-authored-by'`). The table test then fails for the reported reason, refusing the mention-only samples, and the next task fixes the patterns.

## Failure cases

| Case | Result |
|---|---|
| Message file or command has a trailer in any form above | Refused: exit 1, the line is named; hook denies, skill stops |
| Phrase only in a sentence, or without a colon | Passes |
| Empty stdin | Passes (exit 0) |
| Script missing, not executable, or `grep` errors | Hook denies with the reason; skill stops (fail closed) |
| Command without `git commit` | Hook never calls the script (unchanged) |
| `git commit -F file` typed directly | Hook checks the command text only, as today (non-goal) |

## Risks / Trade-offs

- [A trailer form not listed above, e.g. a `git interpret-trailers` pipeline] → the table test lists the known forms; a new one is added there first. The pre-push review and the user's push are the last check.
- [Command text has a line that starts with the phrase but isn't a message, e.g. an `echo` in the same compound command] → it is refused. That is acceptable for a guard: the fix is to split the command.
- [The script calls `grep -E` with POSIX classes] → works with BSD grep (macOS) and GNU grep (CI). The test runs on both.

## Migration Plan

Add the script, then switch the hook and the skill to it in the same commit. The hook path in `.claude/settings.json` is unchanged. Rollback: revert the commit, which restores the inline `grep`s.
