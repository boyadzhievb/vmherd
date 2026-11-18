# Spec Delta

## MODIFIED Requirements

### Requirement: Port forwarding through SSH
For each VM with `ports:`, vmherd SHALL run one background `ssh -N -L` tunnel once SSH is ready (after up, restart and reload), listening on `host_ip` (default 127.0.0.1). An identical running tunnel SHALL be kept, a changed one replaced, a dead one restarted; `down`, `destroy` and `reload` SHALL stop it. Busy host ports and sudo access SHALL be checked before an old tunnel is stopped.

#### Scenario: Port validation
- **WHEN** a port is outside 1–65535, a host port is forwarded twice, `host_ip` is not an IP, or the protocol is not tcp
- **THEN** loading fails naming the VM and port
- **Test:** internal/config/TestPortForwardInvalidHostPort, internal/config/TestPortForwardInvalidGuestPort, internal/config/TestPortForwardDuplicateHostPort, internal/config/TestPortForwardHostIP, internal/config/TestPortForwardInvalidProtocol, internal/config/TestPortForwardRefusesUDP, internal/config/TestLoadReportsEveryBadPortAndShare

#### Scenario: Busy host port
- **WHEN** a host port is already in use by another process
- **THEN** starting the tunnel fails naming the port, and an existing tunnel keeps running
- **Test:** internal/portfwd/TestCheckHostPortsFree, internal/portfwd/TestForwardsNotHeldBy, internal/portfwd/TestDialHostFor

#### Scenario: Privileged port
- **WHEN** a host port is below the unprivileged port start (1024 by default)
- **THEN** the tunnel runs with sudo
- **Test:** internal/portfwd/TestTunnelCommandPrivilegedPorts, internal/portfwd/TestUnprivilegedPortStartDefault, internal/ssh/TestTunnelArgs

#### Scenario: Tunnel record
- **WHEN** vmherd checks a VM's tunnel
- **THEN** it reads the recorded PID and command line, never stops a process whose command line differs (a reused PID), and reports a corrupt record with how to fix it
- **Test:** internal/portfwd/TestRecordRoundTripAndStaleRecord, internal/portfwd/TestCorruptRecordIsActionable, internal/portfwd/TestProcessRunsCommandAndStop, internal/portfwd/TestDescribeForwards

#### Scenario: Forwarded port answers
- **WHEN** up finishes for a VM forwarding host 8080 to guest 8080 where a web server listens
- **THEN** `curl http://127.0.0.1:8080` reaches it
- **Test:** test/e2e/TestAcceptanceScenarios (checkForwardedPorts)
