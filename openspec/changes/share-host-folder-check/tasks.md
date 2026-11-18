# Tasks

## 1. Failing tests first

- [ ] 1.1 Add `internal/config/TestCheckSharedFolderHostPathsExist` (table: missing folder, path is a file, two VMs each missing in Herdfile order, folders present, no shares; assert the VM name, the `host` value, the resolved path and the `mkdir -p` hint in the error). Run it and record here that it fails to compile before the function exists.
- [ ] 1.2 Add `internal/config/TestLoadAcceptsMissingShareFolder`: a Herdfile whose share folder doesn't exist loads without problems. It guards today's behaviour, so record that it passes before the change.
- [ ] 1.3 Add `test/e2e/TestUpRefusesMissingShareFolder` (tag `integration`): a temp project with one ubuntu:noble VM sharing `./missing`; `vmherd up` exits 1, the output names `missing` and `mkdir -p`, and afterwards there is no project domain, disk, network or SSH key. It needs no image download, because the check comes first. Cross-compile it and record how it fails on today's code. Because verification goes through CI, record the failure from the CI run of the test-only commit, or say "not run before the fix" and why.

## 2. Check

- [ ] 2.1 Implement `CheckSharedFolderHostPathsExist` in `internal/config/sharedpath.go` (design decision 1). Verify: 1.1 passes, 1.2 still passes.
- [ ] 2.2 Call it in `up` right after `FilterVMs` (decision 2) and at the start of `checkReloadPrerequisites` with the "reload stopped before changing any VM" suffix (decision 3). Verify: `go build ./...`; 1.3 passes in CI.

## 3. Docs

- [ ] 3.1 Terramino Herdfiles: keep the `mkdir -p` step and drop "(needed until M1.21: up fails on a missing share folder)". Verify: `go test ./test/acceptance/` passes.
- [ ] 3.2 README `shared_folders` row: "the host folder must exist; `up` and `reload` refuse to start otherwise".

## 4. Checks, review, result

- [ ] 4.1 `gofmt -l .` empty; `go vet ./...` and `go vet -tags integration ./...` clean; `golangci-lint run` 0 issues; `go test -count=1 ./...` passes; `openspec validate share-host-folder-check --strict` passes.
- [ ] 4.2 CI integration run (y510p) on the pushed commit: `TestUpRefusesMissingShareFolder` passes and the distro table is unaffected. Record the run link here.
- [ ] 4.3 Independent review with `code-reviewer`. No shell-out, sudo, download or key handling is added, so `security-audit` is not needed. Record each finding as fixed or rejected with the reason.
- [ ] 4.4 Tick M1.21 in milestone.md with its Result note and update the M1 done count in the Progress table, in the same commit. Tick M4.7 in that commit too if this is the first bug fix to follow the test-first rule end to end (see milestone.md M4.7).
