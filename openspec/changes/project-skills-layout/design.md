# Design

## Context

See proposal.md (Why). The four skill files are flat; `.claude/skills/openspec-*` are symlinks to directories in `.agents/skills/` (OpenSpec 2.0). ccattler's `commit`, `test` and `reduce-comments` are the template for layout and frontmatter. The pre-commit hook (`.claude/hooks/pre-commit-gate.sh`) gates any Bash command whose text contains `git commit` or `git tag` and checks gofmt, go vet, golangci-lint and the absence of Co-Authored-By.

## Goals / Non-Goals

**Goals:**
- Every project skill loads and appears in Claude Code's skill list.
- A test catches a skill that breaks the layout.

**Non-Goals:**
- Duplicating CLAUDE.md rules in skills: the skills point at CLAUDE.md instead of copying code-style rules.

## Decisions

1. **Who may invoke each skill.**
   - `commit` and `reduce-comments` get `disable-model-invocation: true`: they write to the repository, so only the user starts them (`/commit`), as in ccattler. The user saying "commit" in a message is still a request to commit; the skill holds the steps.
   - `test` stays model-invocable: it only reads and runs tests. ccattler disables it; vmherd doesn't need to, because running tests is always allowed.
2. **The commit skill runs the commit from a script file in the scratchpad.**
   - The hook matches command text, so a long compound command or a heredoc message that contains `git commit` trips it in ways that are hard to read. A script file also keeps the message easy to review.
   - The hook then sees only `<scratchpad>/commit.sh`, not the commit inside it, so it does not run its checks. The skill therefore runs the same checks itself first (gofmt, go vet, golangci-lint, go test) and stops on any failure.
   - Alternative rejected: changing the hook. It stays the safety net for direct commits.
3. **Delete `improve` instead of moving it.** Alternative: rewrite it like ccattler's ("implement one milestone item"). Rejected: vmherd's path for that is `openspec-propose` → `openspec-apply-change`; a second path would bypass the change and its tasks.
4. **The layout test lives in a new `test/tooling/` package.** It reads `.claude/skills/` relative to the repository root, follows symlinks (`os.Stat`), and for every entry checks: it is a directory, it contains `SKILL.md`, the frontmatter parses (gopkg.in/yaml.v3, already a dependency), `name` equals the directory name, and `description` is not empty. Every problem is reported in one run. M4.9's spec reference check can live next to it later.

## Risks / Trade-offs

- [The skill list is only refreshed in a new Claude Code session] → the manual check (task 2.1) is done in a fresh session.
- [`openspec update` could regenerate a skill in a different layout] → the test covers the `openspec-*` entries too.
- [A skill's frontmatter fields change upstream (Claude Code)] → the test checks only `name` and `description`, the two fields Claude Code requires.

## Migration Plan

`git mv` each file into its directory, so history follows the file. Nothing else depends on the old paths (checked with `grep -rn 'skills/.*\.md'` in task 1.4). Rollback: revert the commit.
