# Spec Delta

## Purpose

vmherd ships example projects in `test/acceptance/` that a user can copy and run as they are. CI boots them nightly and checks the running application the way a user would, so a regression in any feature they use shows up as a failed scenario.

Test names below are the tests this change adds; none exist yet. `test/e2e/TestAcceptanceScenarios` has one subtest per scenario (`terramino-single`, `terramino-multi`); the name in parentheses is the phase helper that fails.

## ADDED Requirements

### Requirement: Scenarios are valid, self-contained projects
Each scenario directory SHALL hold a Herdfile that loads without problems. Its network name and /24 subnet SHALL be its own, never libvirt's stock 192.168.122.0/24 or another scenario's. Its host ports SHALL stay as in the tutorial it ports.

#### Scenario: Herdfiles load
- **WHEN** each scenario's Herdfile is loaded from its directory
- **THEN** it loads without problems
- **Test:** test/acceptance/TestScenarioHerdfilesLoad

#### Scenario: No shared network
- **WHEN** the scenarios' Herdfiles are compared
- **THEN** no two share a network name or a subnet, and none uses 192.168.122.0/24
- **Test:** test/acceptance/TestScenarioNetworksAreDistinct

### Requirement: Host ports are checked before up
Before `up`, the scenario test SHALL check that every host port the scenario forwards is free, and SHALL fail naming the port and, when the host reports it, the process holding it. Nothing SHALL be created in that case.

#### Scenario: Port taken
- **WHEN** another process listens on 127.0.0.1:8080 before the single scenario starts
- **THEN** the subtest fails naming port 8080 and creates no VM, disk or network
- **Test:** test/e2e/TestAcceptanceScenarios (checkHostPortsFree)

### Requirement: The application answers through forwarded ports
After `up` exits 0, the frontend SHALL answer HTTP 200 on 127.0.0.1:8081/score, which its nginx proxies to the backend, and the backend SHALL answer HTTP 200 on 127.0.0.1:8080/api/scores, both through vmherd's port forwarding. The checks SHALL retry within a bounded time, and a failure SHALL name the URL, port and last response or error.

#### Scenario: Single VM
- **WHEN** `vmherd up` finishes for terramino-single
- **THEN** both URLs answer HTTP 200
- **Test:** test/e2e/TestAcceptanceScenarios (checkForwardedPorts)

#### Scenario: Three VMs
- **WHEN** `vmherd up` finishes for terramino-multi
- **THEN** both URLs answer HTTP 200, and the frontend's answer on 8081/score comes from the backend on another VM through the frontend's proxy (nginx answers 503 when the backend is down). The redis link is not checked here: `/score` answers 0 without redis.
- **Test:** test/e2e/TestAcceptanceScenarios (checkForwardedPorts)

#### Scenario: Application not started
- **WHEN** a copy of the single scenario lacks its `docker compose up -d` step
- **THEN** `up` exits 0 but the subtest fails on the HTTP check, naming port 8081
- **Test:** test/e2e/TestAcceptanceScenarios (checkForwardedPorts), run once by hand with the step removed; the result is recorded in the M6.9 result note

### Requirement: VMs reach each other by name
In terramino-multi, the backend SHALL resolve `redis.local` and the frontend `backend.local` to addresses inside the scenario's subnet.

#### Scenario: Names resolve
- **WHEN** the test runs `getent hosts redis.local` on backend and `getent hosts backend.local` on frontend over `vmherd ssh`
- **THEN** each prints one address inside 192.168.241.0/24
- **Test:** test/e2e/TestAcceptanceScenarios (checkNamesResolveAcrossVMs)

### Requirement: Provisioning can be rerun
`vmherd provision` SHALL exit 0 on a running scenario, and the application SHALL still answer through the forwarded ports afterwards.

#### Scenario: Rerun
- **WHEN** `vmherd provision` runs after a successful `up`
- **THEN** it exits 0 and both URLs answer HTTP 200 again
- **Test:** test/e2e/TestAcceptanceScenarios (checkProvisionRerun)

### Requirement: down and up keep the VMs' data
After `down` and `up`, a marker file written into a VM before `down` SHALL still be there, and the application SHALL answer through the forwarded ports again without reprovisioning.

#### Scenario: Restart cycle
- **WHEN** the test writes a marker in each VM, runs `vmherd down`, then `vmherd up`
- **THEN** both commands exit 0, every marker is still there, and both URLs answer HTTP 200
- **Test:** test/e2e/TestAcceptanceScenarios (checkDownUpKeepsData)

### Requirement: destroy leaves nothing behind
`vmherd destroy` SHALL leave no domain, disk, seed ISO or network with the scenario's prefix, and no port-forward tunnel listening on the scenario's host ports.

#### Scenario: Clean host
- **WHEN** `vmherd destroy` finishes
- **THEN** none of those objects remain and ports 8080, 8081 (and 6379 for multi) are free
- **Test:** test/e2e/TestAcceptanceScenarios (checkDestroyLeavesNothing)

### Requirement: Scenarios run nightly, not on every change
The scenario test SHALL run only when `VMHERD_E2E_SCENARIOS=1` and otherwise skip with a message naming the variable. CI SHALL set it for the nightly schedule and when manual dispatch asks for scenarios, and SHALL NOT set it for pull requests or pushes.

#### Scenario: Not requested
- **WHEN** `go test -tags integration ./test/e2e/` runs without `VMHERD_E2E_SCENARIOS`
- **THEN** TestAcceptanceScenarios is skipped and says how to enable it
- **Test:** test/e2e/TestAcceptanceScenarios (skip check), observed in the pull-request CI log

#### Scenario: Nightly
- **WHEN** the nightly integration workflow runs
- **THEN** both scenario subtests run and their results are in the job log
- **Test:** the first nightly run after merge, linked in the M6.9 result note
