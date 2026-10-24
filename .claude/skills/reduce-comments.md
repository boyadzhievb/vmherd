---
description: Reduce Go function comments to 4 lines max
---

# Reduce Go Function Comments

Scan Go source files and condense verbose function doc comments. Any function doc comment longer than 4 lines gets shortened in place.

## Inputs

- **Path** (optional): a Go file or directory to scan. Defaults to the current working directory. When a directory is given, scan all `.go` files recursively (skip `vendor/`, `.git/`, and any generated files).

## Procedure

### 1. Identify long doc comments

For every `.go` file in scope, find function and method doc comments that exceed 4 lines. A "doc comment" is the contiguous block of `//` lines immediately preceding a `func` declaration.

Count only comment-body lines (the text after `//`), not the `func` signature itself.

Skip:
- Comments inside function bodies (inline comments).
- Comments on types, vars, consts, or package declarations -- only function/method comments.
- Test files (`_test.go`).
- Files under `vendor/`.

### 2. For each long comment, condense to at most 4 lines

- Keep the first sentence (the Go doc convention summary line).
- Retain the most important behavioral details (what it returns, key side effects).
- Drop examples, rationale, history, parameter-by-parameter descriptions, and implementation notes.
- Preserve the `//` style.
- Make sure the condensed comment still reads well and follows Go doc conventions.

### 3. Post-processing

- Run `gofmt` on every modified `.go` file after editing.
- Run `go vet ./...` on affected packages to confirm nothing broke.

### 4. Report

After processing, print a summary:

```
Reduced comments in N functions across M files.
```

If no comments exceeded 4 lines, print:

```
No function comments exceed 4 lines. Nothing to do.
```

## Rules

- Never delete a doc comment entirely. Every function must keep at least a one-line summary.
- Never modify function signatures, code, or non-doc comments.
- Preserve copyright headers and build tags untouched.
