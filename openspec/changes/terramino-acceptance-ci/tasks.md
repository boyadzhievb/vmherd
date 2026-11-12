# Tasks

## 1. Failing tests first

- [x] 1.1 Add `test/acceptance/acceptance_test.go` (no build tag) with `TestScenarioHerdfilesLoad` and `TestScenarioNetworksAreDistinct`. Run `go test ./test/acceptance/` against the unchanged Herdfiles and record the failure in the M6.9 checklist: expect "same network name vmherd-terramino" and "uses 192.168.122.0/24".
- [x] 1.2 Add `test/e2e/scenarios_test.go` (tag `integration`):
  - the `VMHERD_E2E_SCENARIOS` gate and `VMHERD_E2E_SCENARIO_DIR` override;
  - the scenario table and `newScenarioProject`;
  - the phases `checkHostPortsFree`, `checkForwardedPorts`, `checkNamesResolveAcrossVMs`, `checkProvisionRerun`, `checkDownUpKeepsData`, and `checkDestroyLeavesNothing` (reused).

  Move any helper both tests need into a shared file without changing its behaviour. Verify: `go vet -tags integration ./test/e2e/` is clean, `golangci-lint run --build-tags integration ./test/e2e/` reports nothing new, and `go test -tags integration -run TestAcceptanceScenarios ./test/e2e/` without the variable prints the skip message.
- [x] 1.3 Cross-compile `e2e.test` and run `TestAcceptanceScenarios` on .71 against the unchanged scenario files (`VMHERD_E2E_SCENARIOS=1`, scenario dir copied). Record how each subtest fails (expected: subnet overlap with libvirt's `default` network) in the M6.9 checklist.

## 2. Scenario files

- [x] 2.1 Give single network `vmherd-terramino-single` with 192.168.240.0/24 (VM .10) and multi `vmherd-terramino-multi` with 192.168.241.0/24 (VMs .10–.12). Verify: `go test ./test/acceptance/` passes.
- [ ] 2.2 Rerun the scenarios on .71 and record the next failures in the M6.9 checklist (expected for multi: provision rerun fails on a busy port or gpg prompt, the app is gone after down + up, possibly git's dubious-ownership check at first boot). Anything that is a vmherd bug rather than a scenario-file bug becomes its own milestone item with a failing test; this change does not work around it.
- [ ] 2.3 Make multi rerunnable:
  - `docker rm -f <name>` before `docker run --name <name> --restart unless-stopped`;
  - `gpg --batch --yes`;
  - clone with `git -c safe.directory=<repo>`, as single does.

  Verify: on .71, the multi subtest passes `checkProvisionRerun` and `checkDownUpKeepsData`.
- [ ] 2.4 Single: add `docker update --restart unless-stopped $(docker compose ps -q)` after `docker compose up -d`. Verify: the single subtest passes `checkDownUpKeepsData` on .71.
- [ ] 2.5 Replace the "Test steps" comments in both Herdfiles with a pointer to `TestAcceptanceScenarios` and the two manual steps (`vmherd up`, open http://localhost:8081). Verify: `go test ./test/acceptance/` still passes, and a copy of each directory still boots with plain `vmherd up` on .71.
- [ ] 2.6 Full run of both scenarios on .71: both subtests pass. Record per-scenario durations and peak RAM in the M6.9 checklist.
- [ ] 2.7 Negative check: run a copy of single without its `docker compose up -d` step. Verify: `up` exits 0 and the subtest fails in `checkForwardedPorts` naming port 8081. Record the output in the M6.9 checklist.

## 3. CI

- [ ] 3.1 In `.github/workflows/integration.yml`, add the boolean `workflow_dispatch` input `scenarios` (default true) and set `VMHERD_E2E_SCENARIOS` to `1` for `schedule` and for dispatch when `scenarios` is true. Pull requests and pushes leave it empty. Verify by reading the expression for each of the four triggers, and with `actionlint` if it is installed.
- [ ] 3.2 Check the job timeouts (150 min, `go test` 140 min) against the measured full-table time plus the 2.6 durations. Raise them only if the sum leaves less than 30 minutes of headroom; record the numbers in the M6.9 checklist.

## 4. Verification and record

- [ ] 4.1 `gofmt -l .` is empty; `go vet ./...` passes; `golangci-lint run` and `golangci-lint run --build-tags integration` report nothing new; `go test ./...` passes; `openspec validate terramino-acceptance-ci --strict` passes.
- [ ] 4.2 Independent review of the diff with the `code-reviewer` agent; record each finding as fixed or rejected (with the reason) in the M6.9 checklist.
- [ ] 4.3 Tick M6.9 in milestone.md and add its Result note: the failing runs from 1.1, 1.3 and 2.2, the fixes, the .71 results and durations, the negative check, and any new items filed. Update the Progress table (M6 done count) in the same commit.

## Workflow follow-up

- After the user pushes: trigger the integration workflow by dispatch with `scenarios: true`, check that both subtests run and pass, and that a pull-request run logs the skip message.
- After the first green nightly run: link it in the M6.9 result note, then `/opsx:archive terramino-acceptance-ci`. This merges the `acceptance-scenarios` spec and the `ssh-access` test link into `openspec/specs/`.
