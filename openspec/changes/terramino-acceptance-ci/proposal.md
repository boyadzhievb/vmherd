# Proposal

## Why

Milestone item **M6.9**. The two Terramino projects in `test/acceptance/` (ported from HashiCorp's learn-vagrant-get-started tutorial) are vmherd's only user-level scenarios: a real application, forwarded ports, `type: file` steps, extra shares and three VMs that talk to each other. Their steps are comments in the Herdfiles and nothing runs them. The M6.4 e2e test checks vmherd's own behaviour per distro, but none of the above. While planning this change, reading the scenarios showed they would not pass as written (see Impact), which is exactly what an unrun acceptance test hides.

## What Changes

- A new `TestAcceptanceScenarios` in `test/e2e/` (build tag `integration`) copies each scenario to a temporary project and runs the steps the Herdfile comments describe:
  - `up`, HTTP checks through the forwarded ports, `ssh`, `provision`, `down` + `up` with a marker file kept, `destroy` with no leftovers;
  - for multi, also that the VMs reach each other by name over the project network.
- Before `up`, the test checks that the scenario's host ports are free, and fails naming the port and the process holding it.
- The scenarios run only when `VMHERD_E2E_SCENARIOS=1`. CI sets it for the nightly run and for manual dispatch (new `scenarios` input), not for pull requests or pushes, because the guests download packages and take longer.
- The Herdfiles get their own subnets (192.168.240.0/24 single, 192.168.241.0/24 multi) and their own network names, instead of libvirt's stock `default` subnet and one shared name. Host ports stay as in the tutorial, so the files remain copy-pasteable.
- Multi's provisioning becomes rerunnable: containers get names, are replaced on rerun, and restart with Docker. Its clone works on a 9p share as root, as single's already does.
- The step comments in the Herdfiles become a short pointer to the test, so the steps live in one place.

**Out of scope:**
- Making the scenarios faster: no pre-built images, no package cache.
- Running them on pull requests.
- New vmherd features. Any vmherd bug the scenarios find becomes its own milestone item with its own failing test (M4.7).
- Other distros: the scenarios stay on ubuntu:noble, as in the tutorial.

**Vagrant comparison:** Vagrant has no acceptance runner for user projects. The tutorial's steps are manual: `vagrant up`, then `curl` in a browser. vagrant-libvirt runs its own acceptance suite (vagrant-spec) against real boxes. vmherd follows vagrant-libvirt: the tool's own CI boots the tutorial projects. Port forwarding works the same way as vagrant-libvirt's (an SSH tunnel per VM).

## Capabilities

### New Capabilities
- `acceptance-scenarios`: the example projects vmherd ships in `test/acceptance/`. They must work exactly as a user would copy and run them, and CI must prove it nightly.

### Modified Capabilities
- `ssh-access`: the "Forwarded port answers" scenario of *Port forwarding through SSH* gets its test (it says "none yet — planned in M6.9" today).

## Impact

- **Code:** new `test/e2e/scenarios_test.go`. Shared helpers move out of `e2e_test.go` where both tests use them (binary, state dir, leftover cleanup). No production code changes are planned.
- **Scenario files:** `test/acceptance/terramino-single/Herdfile`, `test/acceptance/terramino-multi/Herdfile`, `test/acceptance/terramino-multi/common-dependencies.sh`.
- **CI:** `.github/workflows/integration.yml` gets a `scenarios` dispatch input and `VMHERD_E2E_SCENARIOS` set on schedule and dispatch. The nightly run grows by an estimated 25–40 minutes; the 150-minute job timeout is checked against the first real run.
- **Runner needs:** internet access from the guests (Ubuntu archive, download.docker.com, github.com), host ports 8080, 8081 and 6379 free, and about 5 GB RAM for the three multi VMs plus Docker image builds.
- **Known problems the first run is expected to show** (found while reading the scenarios; to be confirmed by running them):
  - Both Herdfiles use the network name `vmherd-terramino`.
  - Multi's `docker run -d -p …` fails on a second `provision` (port already allocated), and its containers don't come back after `down` + `up`.
  - Multi's `git checkout` runs as root in a 9p share without `safe.directory`, which single's script explicitly works around.
