# Design

## Context

See proposal.md for why. Current state that shapes the approach:

- **The e2e test** (`test/e2e/e2e_test.go`, tag `integration`) already provides most of what the scenarios need:
  - `TestMain` builds the binary, or takes it from `VMHERD_E2E_BINARY`.
  - `e2eProject` runs vmherd with its own `VMHERD_STATE_DIR`.
  - `removeLeftovers` and `checkDestroyLeavesNothing` find domains, disks and seeds by the domain prefix `vmherd-<project dir name>-`.
  - Timeouts: 25 minutes for commands that boot VMs, 5 minutes for the rest.
- **CI** runs `go test -tags integration -count=1 -timeout=140m -v ./...` once, on one self-hosted runner, with `concurrency: integration`. The integration runner is not .71.
- **The scenarios as written:**
  - Both Herdfiles use network name `vmherd-terramino` and 192.168.122.0/24.
  - The SSH user is `vagrant` and shares go to `/home/vagrant/terramino-go`, as in the tutorial.
  - Single runs `docker compose up -d` from the upstream repo's `containerized` branch.
  - Multi runs `docker build` + `docker run -d -p …` per VM, with no container name, no restart policy, and `gpg --dearmor` without `--batch --yes`. Its `git checkout` runs as root in a 9p share without `safe.directory`.
- **What `down` + `up` does:** an existing VM boots without reprovisioning. Anything the application needs after a reboot must come back by itself.

## Goals / Non-Goals

**Goals:**
- One test, `TestAcceptanceScenarios`, that fails on the first broken user-visible step and names it: the command and exit code, or the URL and port.
- The scenario files stay copy-pasteable for users, and the test runs them unchanged apart from copying them.
- The test itself runs cleanly on the nightly runner. Each problem the first run finds gets its own milestone item; this change does not paper over it.

**Non-Goals:**
- Parallel scenarios. Both use host ports 8080 and 8081, so they run one after the other.
- Retrying flaky downloads inside the guests. A vendor or GitHub outage fails the nightly run and is visible as such.

## Decisions

1. **Same package as the e2e test** (`test/e2e/scenarios_test.go`), not a new package.
   - **Why:** it reuses `TestMain`'s binary and the helpers unchanged.
   - **Change needed:** `newE2EProject` writes a templated Herdfile. A small `newScenarioProject` copies a scenario directory instead (Herdfile, scripts, and the empty share directories the old comments asked users to `mkdir`) into `t.TempDir()/acceptance-<scenario>`. That directory name gives a domain prefix (`vmherd-acceptance-terramino-single-`) no user project on the runner has.
   - **Finding the scenarios:** the source is `../acceptance` relative to the package directory (where `go test` runs). `VMHERD_E2E_SCENARIO_DIR` overrides it for a cross-compiled `e2e.test` on .71, which has no source tree; the scenario directories are copied there next to the binary.
   - **Alternative:** a separate package with copied helpers. Rejected: that duplicates about 150 lines.

2. **Scenario table in Go**, with name, directory, network name, subnet, the URLs to check, the VMs and host ports, and the name checks (multi only).
   - **Why:** the Herdfile stays the user's file and the test's expectations stay in code.
   - **Alternative:** parse expectations from Herdfile comments. Rejected: comments are not a contract.

3. **Port check before up:**
   - `net.Listen("tcp", "127.0.0.1:<port>")` and close it.
   - On failure, run `sudo ss -ltnp 'sport = :<port>'` and include its output in the error, so a stale vmherd tunnel or a runner service is named.
   - A plain dial would miss a listener on another address. Listening matches what vmherd's own tunnel needs.

4. **HTTP checks:**
   - A plain `http.Client` with a per-request timeout, retried with exponential backoff (1 s up to 8 s) for up to `applicationReadyTimeout` (3 minutes), because containers may still be starting after cloud-init finishes.
   - A failure reports the URL, port, last status code or error, and the first 200 bytes of the last body.
   - Success means HTTP 200; the body is not compared, because upstream Terramino may change its markup.

5. **Name checks (multi):** `vmherd ssh backend -- getent hosts redis.local`, and the same from frontend for `backend.local`. Each must print one address inside 192.168.241.0/24. This proves the project network carries mDNS between VMs, which the tutorial relies on.

6. **Provision and restart:**
   - `provision` uses the long timeout: it reruns the apt and Docker installs and the builds.
   - Before `down`, the test writes `/var/tmp/acceptance-marker` in each VM.
   - After `up`, it checks the markers and repeats the HTTP checks.

7. **Fixes to the scenario files** (they are vmherd's own files; the aim is to match the tutorial's intent):
   - Single network `vmherd-terramino-single`, 192.168.240.0/24, VM at .10.
   - Multi network `vmherd-terramino-multi`, 192.168.241.0/24, VMs at .10–.12.
   - Multi: `docker rm -f <name>` before `docker run --name <name> --restart unless-stopped`; `gpg --batch --yes`; clone with `git -c safe.directory=<repo>` as single does.
   - Single: after `docker compose up -d`, `docker update --restart unless-stopped $(docker compose ps -q)`, so the app returns after `down` + `up`. The compose file is upstream's and can't be edited here.
   - The "Test steps" comments become one line each: "Run by test/e2e TestAcceptanceScenarios (VMHERD_E2E_SCENARIOS=1); manual steps: vmherd up, then open http://localhost:8081".
   - **Vagrant:** the tutorial's Vagrantfiles have the same rerun problem; `vagrant provision` would fail there too. vmherd's copy fixes it rather than mirroring a bug.

8. **Fast checks without libvirt** in `test/acceptance/acceptance_test.go` (no build tag, so it runs on every PR): both Herdfiles load with `config.Load`, and their networks are distinct and not 192.168.122.0/24. A broken scenario Herdfile then fails in seconds, not at 02:00.

9. **Gate and CI:**
   - `VMHERD_E2E_SCENARIOS=1` enables the test; otherwise it calls `t.Skip` with a message naming the variable.
   - The workflow sets it to `1` on `schedule`, and on `workflow_dispatch` when the new boolean input `scenarios` is true (default true). Pull requests and pushes leave it unset.
   - Scenarios run after the distro table in the same `go test` call, so the 140/150-minute timeouts must cover both. Check against the first run and raise them if needed.

## Risks / Trade-offs

- **[Risk]** Docker may start containers before the 9p share is mounted after a reboot, if a container bind-mounts the share. → The restart check in step 6 shows it. If it happens, that's a vmherd finding (mount ordering), filed as its own item, not hidden by the test.
- **[Risk]** Upstream `terramino-go` (branch `containerized`) changes or disappears. → The clone uses the branch, not a commit, as the tutorial does. A failure names the clone step. Pinning a commit is a follow-up if upstream turns out to be unstable.
- **[Risk]** A stale port-forward tunnel from a killed earlier run holds 8080. → The port check names the holding process. Cleanup on failure runs `vmherd destroy`, which stops tunnels.
- **[Risk]** The nightly run grows by 25–40 minutes and needs about 5 GB RAM. → Scenarios run one at a time after the distro table; runner RAM is already logged by the workflow.
- **[Trade-off]** The "application not started" check is run once by hand, not kept as a permanent variant, which would cost another full boot every night. Its result goes into the M6.9 result note.
- **Failure cases and what the test does:**
  - `up` exits non-zero → fail with its output. The scenario's later phases are skipped, and cleanup runs `destroy` plus `removeLeftovers`.
  - An HTTP check times out → fail naming the URL and port; cleanup as above.
  - `destroy` leaves objects → fail listing them; `removeLeftovers` removes them so the next night starts clean.
  - Partial failure across VMs (one multi VM fails provisioning): `up` attempts every VM and exits 1 naming the VM and step (vm-lifecycle spec), and the test fails on that exit code.

## Migration Plan

No user-facing change, so no migration. Users who copied the old scenario Herdfiles keep working copies; the new ones only change subnet and network name. Rollback is reverting the commit; the workflow change is in the same commit.

## Open Questions

- Does the integration runner allow outbound HTTPS to download.docker.com and github.com from guests? The first dispatch run answers this. If not, the scenarios need a proxy setting, which is a follow-up item, not a change to this design.
