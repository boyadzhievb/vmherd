# Implement Improvement

Implement a specific improvement or feature for vmherd.

## Steps

1. Read `CLAUDE.md` to understand the project spec and current state.
2. Identify the files that need to change by reading the relevant source code.
3. Implement the change following the code style rules in CLAUDE.md:
   - Comment every exported function (max 4 lines)
   - Use long descriptive variable names
   - No magic numbers — use named constants
   - Function length limit: 80 lines
   - Error messages must be actionable
4. Write tests that verify the new behavior.
5. Run `gofmt -w .`, `go vet ./...`, and `go test ./...` to ensure nothing is broken.
6. Update CLAUDE.md if the feature changes the project spec.

## Design Rules

- Idempotent operations — `vmherd up` is safe to repeat.
- No hardcoded DNS — derive from host /etc/resolv.conf.
- SHA256 verification on all downloaded images.
- Clean separation between packages (config, vm, cloudinit, image, ssh, network).
