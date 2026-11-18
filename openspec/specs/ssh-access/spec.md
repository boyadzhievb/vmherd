# ssh-access Specification

## Purpose

Every project gets its own SSH key, so `vmherd ssh <vm>` and `vmherd scp` work without setup. Forwarded ports reach VMs through an SSH tunnel per VM, as in vagrant-libvirt, with no firewall rules on the host. A VM's serial console log can be read for boot problems SSH can't show.

## Requirements

### Requirement: Project SSH key
vmherd SHALL create an ed25519 key pair in the project state directory on first use (private key mode 0600), SHALL reuse it afterwards, and SHALL inject its public key into every VM for the SSH user.

#### Scenario: First use
- **WHEN** a project has no key yet
- **THEN** `id_ed25519` and `id_ed25519.pub` are created and the private key is readable only by the user
- **Test:** internal/ssh/TestEnsureKeyPairCreatesNewKey, internal/ssh/TestEnsureKeyPairSetsRestrictedPermissions

#### Scenario: Key exists
- **WHEN** the key already exists
- **THEN** it is used unchanged
- **Test:** internal/ssh/TestEnsureKeyPairIdempotent, internal/ssh/TestReadPublicKey, internal/ssh/TestReadPublicKeyMissingFile

#### Scenario: Key in the guest
- **WHEN** a VM is created
- **THEN** its user-data creates the SSH user with the project's public key and passwordless sudo
- **Test:** internal/cloudinit/TestGenerateFilesUserDataContent, internal/cloudinit/TestGenerateFilesMetaDataContent, internal/cloudinit/TestGenerateFilesCreatesAllFiles, internal/cloudinit/TestGenerateFilesPathsAreCorrect, internal/cloudinit/TestCloudInitFilePermissions

### Requirement: vmherd ssh
`vmherd ssh <vm>` SHALL open an interactive session as the SSH user at the VM's static address or current DHCP lease; `vmherd ssh <vm> -- <cmd...>` SHALL run the command, exit 0 when it succeeds, and exit 1 with "Error: exit status <n>" when it fails (the remote exit code is not passed through, unlike `vagrant ssh -c`). One argument after `--` SHALL be passed as a shell command line; several SHALL each be quoted. The VM SHALL be running, and a DHCP VM SHALL have a known address.

#### Scenario: Run a command
- **WHEN** the user runs `vmherd ssh fixed -- cat /tmp/marker`
- **THEN** the guest's file content is printed and vmherd exits 0
- **Test:** internal/ssh/TestRemoteCommandLine, internal/ssh/TestSSHBaseArgs, test/e2e/TestDistroLifecycle (checkProvisioningResults)

#### Scenario: Command fails
- **WHEN** the user runs `vmherd ssh fixed -- false`
- **THEN** vmherd exits 1 and prints "Error: exit status 1"
- **Test:** none yet — cmd/ code, needs M2.3

#### Scenario: One argument is a command line
- **WHEN** the user runs `vmherd ssh fixed -- 'ls | wc -l'`
- **THEN** the pipe runs in the guest
- **Test:** internal/ssh/TestRemoteCommandLine, test/e2e/TestDistroLifecycle (checkShareBothWays)

#### Scenario: VM not running
- **WHEN** the VM is shut off
- **THEN** ssh fails with "is not running … start it with 'vmherd up <vm>'" without trying to connect
- **Test:** none yet — cmd/ code, needs M2.3

#### Scenario: DHCP VM without a lease
- **WHEN** a DHCP VM has no address yet
- **THEN** ssh fails saying it may still be booting
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: vmherd scp
`vmherd scp <src> <dest>` SHALL copy files between the host and VMs, where `<vm>:<path>` names a path in a VM. An unknown VM name before `:` SHALL be refused listing the VMs; arguments whose prefix contains `/` or `.` SHALL be treated as local paths.

#### Scenario: Copy to a VM
- **WHEN** the user runs `vmherd scp ./a.txt fixed:/tmp/a.txt`
- **THEN** the file is copied as the SSH user to the VM's address
- **Test:** none yet — cmd/ code, needs M2.3

#### Scenario: Unknown VM
- **WHEN** the user runs `vmherd scp ./a.txt nope:/tmp/`
- **THEN** it fails with `unknown VM "nope" — available: …`
- **Test:** none yet — cmd/ code, needs M2.3

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

### Requirement: TCP only, no firewall rules
Port forwarding SHALL support TCP only and refuse `protocol: udp` with an explanation. vmherd SHALL add no firewall rules and SHALL remove the iptables rules v0.1.x added.

#### Scenario: UDP forward
- **WHEN** a port forward sets `protocol: udp`
- **THEN** loading fails explaining that SSH tunnels carry TCP only
- **Test:** internal/config/TestPortForwardRefusesUDP

#### Scenario: Old iptables rules
- **WHEN** the host still has DNAT rules in exactly the v0.1.x format for a VM
- **THEN** they are removed and no other rule is touched
- **Test:** internal/portfwd/TestMatchingLegacyRules

### Requirement: Serial console log
With `console_log: true` on a VM, its serial console SHALL be written to a log file in the project state, and `vmherd console-log <vm>` SHALL print it, reading it with sudo when libvirt made it root-only. Without the setting it SHALL say how to enable it.

#### Scenario: Readable log
- **WHEN** the log file is readable by the user
- **THEN** console-log prints it
- **Test:** cmd/vmherd/TestPrintConsoleLogReadable

#### Scenario: Root-only log
- **WHEN** libvirt wrote the log as root:root 0600
- **THEN** console-log prints it through `sudo cat`
- **Test:** cmd/vmherd/TestPrintConsoleLogRootOnly

#### Scenario: Not enabled
- **WHEN** the VM has no console log
- **THEN** console-log fails telling the user to set `console_log: true` and run `vmherd reload <vm>`
- **Test:** cmd/vmherd/TestPrintConsoleLogMissing
