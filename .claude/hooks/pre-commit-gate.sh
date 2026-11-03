#!/usr/bin/env bash
# PreToolUse gate for Bash tool calls.
#
# Only acts on `git commit` and `git tag`; every other command passes through
# immediately so a dirty working tree never blocks ordinary shell use.
#
#   git commit -> no Co-Authored-By, gofmt clean, go vet, golangci-lint
#   git tag    -> annotated tags only (git tag -a ...)
#
# Denies by printing a PreToolUse permissionDecision JSON object on stdout.

set -uo pipefail

HOOK_INPUT=$(cat)

# deny_with_reason prints a deny decision. Uses jq for safe JSON escaping when
# available, otherwise a fixed message (the reason must then be JSON-safe).
deny_with_reason() {
    if command -v jq >/dev/null 2>&1; then
        jq -n --arg reason "$1" \
            '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $reason}}'
    else
        printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}\n' "$1"
    fi
    exit 0
}

# Patterns match the git subcommand anywhere in a compound command
# (e.g. "cd x && git commit -m ..."), optionally with "git -C <dir>".
GIT_PREFIX='(^|[;&|[:space:](){"'"'"'])git([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+'
COMMIT_PATTERN="${GIT_PREFIX}commit([[:space:]]|$)"
TAG_PATTERN="${GIT_PREFIX}tag([[:space:]]|$)"

if ! command -v jq >/dev/null 2>&1; then
    # Fail closed, but only for the commands this gate is responsible for.
    if echo "$HOOK_INPUT" | grep -qE "$COMMIT_PATTERN|$TAG_PATTERN"; then
        deny_with_reason "BLOCKED: jq is not installed, so the commit/tag checks cannot run. Install jq (brew install jq / apt install jq) and retry."
    fi
    exit 0
fi

COMMAND=$(echo "$HOOK_INPUT" | jq -r '.tool_input.command // ""')

IS_COMMIT=false
IS_TAG=false
echo "$COMMAND" | grep -qE "$COMMIT_PATTERN" && IS_COMMIT=true
echo "$COMMAND" | grep -qE "$TAG_PATTERN" && IS_TAG=true

if [[ "$IS_COMMIT" == false && "$IS_TAG" == false ]]; then
    exit 0
fi

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$REPO_ROOT" || exit 0

if [[ "$IS_TAG" == true ]]; then
    # Reject "git tag <name>" (lightweight); allow flags like -a, -l, -d.
    if echo "$COMMAND" | grep -qE "${GIT_PREFIX}tag[[:space:]]+[^-[:space:]]"; then
        deny_with_reason "BLOCKED: Use annotated tags (git tag -a v0.X.Y -m summary). No lightweight tags."
    fi
fi

if [[ "$IS_COMMIT" == true ]]; then
    if echo "$COMMAND" | grep -qi 'Co-Authored-By'; then
        deny_with_reason "BLOCKED: Do not add Co-Authored-By to commits. Remove it and retry."
    fi

    if ! command -v go >/dev/null 2>&1 || ! command -v gofmt >/dev/null 2>&1; then
        deny_with_reason "BLOCKED: go/gofmt not found on PATH, so the commit checks cannot run. Install Go or fix PATH and retry."
    fi

    # Tracked plus untracked-but-not-ignored Go files: covers files that the
    # same command stages with "git add ... && git commit".
    UNFORMATTED=$(git ls-files -z -co --exclude-standard -- '*.go' | xargs -0 gofmt -l 2>/dev/null)
    if [[ -n "$UNFORMATTED" ]]; then
        deny_with_reason "BLOCKED: gofmt check failed. Run gofmt -w on: $(echo "$UNFORMATTED" | tr '\n' ' ')"
    fi

    if ! VET_OUTPUT=$(go vet ./... 2>&1); then
        deny_with_reason "BLOCKED: go vet failed. Fix vet warnings before committing:
$VET_OUTPUT"
    fi

    if command -v golangci-lint >/dev/null 2>&1; then
        if ! LINT_OUTPUT=$(golangci-lint run ./... 2>&1); then
            deny_with_reason "BLOCKED: golangci-lint failed. Fix lint issues before committing:
$LINT_OUTPUT"
        fi
    fi
fi

exit 0
