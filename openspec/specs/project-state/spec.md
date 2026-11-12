# project-state Specification

## Purpose

Each project directory gets its own state directory, SSH key and libvirt names, so two projects never touch each other's VMs, disks or files, and vmherd never deletes files it did not create.

## Requirements

### Requirement: Project-unique names
vmherd SHALL name each VM's libvirt domain and disk `vmherd-<project>-<hash>-<vm>`, where `<project>` is the directory name reduced to `[a-z0-9-]` and `<hash>` is the first 8 hex characters of the state-directory hash, and SHALL store disks under `/var/lib/libvirt/images/`.

#### Scenario: Two projects with the same directory name
- **WHEN** two projects in different paths are both named `web`
- **THEN** their domain names differ
- **Test:** internal/config/TestDomainNameDiffersForSameBasename, internal/config/TestDomainNameHashMatchesStateDir

#### Scenario: Unsafe directory names
- **WHEN** the project directory name has spaces, commas, quotes or uppercase letters
- **THEN** the domain name contains only `[a-z0-9-]`
- **Test:** internal/config/TestDomainNameUnsafeProjectNames, internal/config/TestDomainName, internal/config/TestDiskPath

### Requirement: Pre-M0.8 domains are kept
A VM created by an older vmherd under `vmherd-<project>-<vm>` SHALL keep working while its MAC address matches the one saved in this project's state; a domain with that name but another MAC SHALL be left alone with a note. `vmherd reload` SHALL move the VM to the new name.

#### Scenario: Old-style domain of this project
- **WHEN** only the old-style domain exists and its MAC matches the saved network config
- **THEN** status, ssh and destroy act on the old-style domain
- **Test:** internal/config/TestLegacyDomainNameOverride, internal/cloudinit/TestReadSavedMACAddressRoundTrip, internal/cloudinit/TestReadSavedMACAddressMissing, internal/cloudinit/TestReadSavedMACAddressCorrupt

#### Scenario: Old-style domain of another project
- **WHEN** an old-style domain with the same name exists but its MAC differs
- **THEN** vmherd prints a note and uses the new name
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: State directory
vmherd SHALL keep per-project state (SSH key, cloud-init files, port-forward records) in `$XDG_STATE_HOME/vmherd/<project>-<hash>/` (by default `~/.local/state/vmherd/`), or in `VMHERD_STATE_DIR` when set. It SHALL mark directories it creates with a `.vmherd-state` file and create them mode 0700.

#### Scenario: Global state directory
- **WHEN** a project runs for the first time without `VMHERD_STATE_DIR`
- **THEN** its state directory is created under the XDG state home and marked
- **Test:** internal/config/TestResolveStateDirMarksGlobalDir

#### Scenario: New directory from the environment
- **WHEN** `VMHERD_STATE_DIR` names a directory that does not exist yet
- **THEN** vmherd creates and marks it
- **Test:** internal/config/TestResolveStateDirMarksNewEnvDir

#### Scenario: Foreign directory from the environment
- **WHEN** `VMHERD_STATE_DIR` names an existing directory with other files and no marker
- **THEN** vmherd refuses to use it and lists some of the foreign files
- **Test:** internal/config/TestResolveStateDirRefusesForeignEnvDir

#### Scenario: State directory from an older vmherd
- **WHEN** `VMHERD_STATE_DIR` holds only files an older vmherd wrote
- **THEN** vmherd adopts and marks it
- **Test:** internal/config/TestResolveStateDirAdoptsLegacyEnvDir

### Requirement: Safe removal of state
`vmherd destroy` SHALL remove only marked state directories and only the files vmherd writes; unknown files SHALL be kept and reported.

#### Scenario: Unmarked directory
- **WHEN** destroy is asked to remove a state directory without the marker
- **THEN** nothing is removed and the error says why
- **Test:** internal/config/TestRemoveProjectStateDirRefusesUnmarkedDir, internal/config/TestRemoveVMStateDirRefusesUnmarkedDir

#### Scenario: Marked directory with a user file
- **WHEN** a marked state directory also holds a file vmherd did not write
- **THEN** vmherd's files are removed, the unknown file and its directory are kept
- **Test:** internal/config/TestRemoveProjectStateDirRemovesMarkedDir, internal/config/TestRemoveProjectStateDirKeepsUnknownFiles, internal/config/TestRemoveProjectStateDirReportsRemovalErrors

#### Scenario: VM name with path characters
- **WHEN** removal of a VM state directory is asked for a name like `../x`
- **THEN** it is refused
- **Test:** internal/config/TestRemoveVMStateDirRejectsInvalidName

### Requirement: Querying paths
`vmherd info ssh-key` and `vmherd info state-dir` SHALL print the project's private key path and state directory, one line each, for scripts.

#### Scenario: Print the state directory
- **WHEN** the user runs `vmherd info state-dir` in a project
- **THEN** the only output is the state directory path
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: Never as root
vmherd SHALL refuse to create VMs with shared folders when run as root, because QEMU then runs as the invoking user.

#### Scenario: Run as root
- **WHEN** vmherd runs with UID 0 and a VM has shared folders
- **THEN** it refuses and tells the user to run it as their own user
- **Test:** internal/vm/TestDetectHostIdentityRefusesRoot

### Requirement: Quiet mode
The global `--quiet` (`-q`) flag SHALL suppress status messages and progress bars; the error a command fails with SHALL still be printed.

#### Scenario: Quiet output
- **WHEN** a command runs with `-q`
- **THEN** status lines and progress output are not printed
- **Test:** internal/ui/TestStdoutQuietMode, internal/ui/TestStderrQuietMode, internal/ui/TestPrintfQuietMode, internal/ui/TestPrintlnQuietMode

#### Scenario: Quiet failure
- **WHEN** a command run with `-q` fails
- **THEN** its error is printed and it exits 1
- **Test:** none yet — cmd/ code, needs M2.3
