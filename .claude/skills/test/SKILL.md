---
name: test
description: Run gofmt, go vet (including the integration-tagged tests), golangci-lint and the vmherd test suite, and summarize failures with root causes
---

# Run Tests

Run the vmherd checks and test suite and report results.

## Steps

1. Run `gofmt -l .`; it must print nothing.
2. Run `go vet ./...` to catch static analysis issues.
3. Run `go vet -tags integration ./...` so the libvirt tests compile, without running them.
4. Run `golangci-lint run`.
5. Run `go test ./... > <scratchpad>/test.out 2>&1; echo "go test exit: $?"`, writing the output to a file in the session's scratchpad. The `go test` exit status is the one that counts; a grep's exit status says nothing about the tests.
6. Show only the failures with `grep -v -e '^ok' -e 'no test files' <scratchpad>/test.out`, and count the passed packages with `grep -c '^ok' <scratchpad>/test.out`.
7. If any package fails, rerun just that package with `go test -v -run <TestName> ./<package>/` for detail.
8. Report a summary: which checks passed, how many packages passed, which (if any) failed, and the root cause of each failure.

## Notes

- Tests that need libvirt are behind the `integration` build tag. They run on the testbed or the self-hosted integration runner (`.github/workflows/integration.yml`), never on the Mac.
- If a test fails with file permission errors, check that temp directories and SSH keys have the right permissions.
