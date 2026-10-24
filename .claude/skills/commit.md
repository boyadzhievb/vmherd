# Commit Changes

Create a commit for the current changes following vmherd conventions.

## Steps

1. Run `git status` to see what changed.
2. Run `git diff` to review the actual changes.
3. Run `gofmt -w .` and `go vet ./...` to check formatting and static analysis.
4. Run `go test ./...` to verify everything passes before committing.
5. Stage the relevant files (avoid staging unrelated changes).
6. Write a commit message with a brief description of what changed and why.

## Commit Message Format

```
short description of the change
```

Keep it concise — one line for simple changes, a body paragraph for complex ones.
