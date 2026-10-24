# Run Tests

Run the vmherd test suite and report results.

## Steps

1. Run `gofmt -l .` to check formatting.
2. Run `go vet ./...` to catch static analysis issues.
3. Run `go test ./...` for the full suite.
4. If any package fails, re-run that package with `-v` to get detailed output.
5. Report a summary: how many packages passed, which (if any) failed, and the root cause of failures.

## Notes

- Integration tests that require libvirt are behind a build tag and only run with `-tags integration`.
- If a test fails with file permission errors, check that temp directories and SSH keys have correct permissions.
