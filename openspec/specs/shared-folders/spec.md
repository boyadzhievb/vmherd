# shared-folders Specification

## Purpose

Every VM sees the project directory at `/vagrant` by default, like Vagrant, so users moving from Vagrant keep their scripts. Shares use 9p; files keep the host user's ownership on both sides, and guest root can't create root-owned files on the host.

## Requirements

### Requirement: Default project share
When `shared_folders` is omitted, a VM SHALL share the project directory at `/vagrant`. `shared_folders: []` SHALL mean no shares. An explicit list SHALL replace the default.

#### Scenario: Omitted
- **WHEN** a VM has no `shared_folders` key
- **THEN** it shares `.` at `/vagrant`
- **Test:** internal/config/TestDefaultSharedFolders

#### Scenario: Empty list
- **WHEN** a VM has `shared_folders: []`
- **THEN** it has no shares and its cloud-init has no mount commands
- **Test:** internal/config/TestEmptySharedFoldersMeansNoShares, internal/cloudinit/TestGenerateFilesWithoutSharesHasNoHomeFixOrRuncmd

#### Scenario: Explicit list
- **WHEN** a VM lists `{host: ./src, guest: /src}` only
- **THEN** it shares `./src` at `/src` and not the project at `/vagrant`
- **Test:** internal/config/TestExplicitSharedFolders

### Requirement: Every share is announced
`vmherd up` SHALL print one line per share it creates: `<vm>: sharing <host> → <guest> (read-write)`.

#### Scenario: Default share announced
- **WHEN** up creates a VM with the default share
- **THEN** the output shows the project path shared to `/vagrant`
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: Safe host paths
A share's host path SHALL resolve, after following symlinks, inside the project directory or `$HOME`; other paths need `allow_outside_home: true` on the entry. `/`, `/home`, `/Users`, `$HOME` itself and system directories (`/etc`, `/usr`, `/var/lib`, `/root`, …) SHALL always be refused. The guest path SHALL be absolute and free of control characters.

#### Scenario: Path rules
- **WHEN** a host path is `/`, `$HOME`, `/etc/x`, a symlink into a system directory, or a directory outside `$HOME` without `allow_outside_home`
- **THEN** loading fails naming the path and the rule
- **Test:** internal/config/TestResolveSharedFolderHostPath, internal/config/TestValidateSharedFolderPathTraversal, internal/config/TestValidateSharedFolderRelativeTraversal

#### Scenario: Project directory is $HOME
- **WHEN** the project directory is `$HOME` and the default share applies
- **THEN** loading fails: `$HOME` can't be shared as a whole
- **Test:** internal/config/TestResolveSharedFolderHostPathRejectsProjectAtHome

#### Scenario: Guest path
- **WHEN** a guest path is relative or contains a newline
- **THEN** loading fails asking for an absolute path
- **Test:** internal/config/TestSharedFolderRelativeGuestPath, internal/config/TestValidateFieldRules

#### Scenario: Resolved path is used
- **WHEN** the Herdfile loads
- **THEN** each share carries its resolved absolute host path, which is what the VM gets
- **Test:** internal/config/TestLoadRecordsResolvedSharedFolderHost

### Requirement: Same owner on both sides
For VMs with shares, QEMU SHALL run as the invoking host user with the group of `/dev/kvm`, using 9p with `accessmode=squash`, and the guest SSH user SHALL get the host user's UID. VMs without shares SHALL keep libvirt's default QEMU user. When the host UID is outside the regular user range, the guest user SHALL keep its default UID and up SHALL warn that only guest root can write to shares.

#### Scenario: Files both ways
- **WHEN** the host writes a file in the project and the guest writes one in `/vagrant`
- **THEN** each is visible on the other side, and the guest's file is owned by the host user on the host
- **Test:** test/e2e/TestDistroLifecycle (checkShareBothWays)

#### Scenario: QEMU identity in the domain
- **WHEN** a VM with shares is defined
- **THEN** its domain XML has a static DAC seclabel `+UID:+GID` and refuses to render without one
- **Test:** internal/vm/TestRenderDomainXML, internal/vm/TestRenderDomainXMLSharesRequireLabel, internal/vm/TestRenderDomainXMLRequiresLabelForSharedFolders, internal/vm/TestDetectHostIdentityReadsDeviceGroup, internal/vm/TestDetectHostIdentityMissingKVM

#### Scenario: Guest UID
- **WHEN** the host user's UID is in the regular user range
- **THEN** the guest SSH user is created with that UID; otherwise the default UID is kept
- **Test:** internal/vm/TestGuestUserUID, internal/cloudinit/TestGenerateFilesWithoutUIDLeavesDefault

#### Scenario: Mounts in cloud-init
- **WHEN** a VM has one or more shares
- **THEN** cloud-init mounts each tag at its guest path, with paths quoted for the shell and YAML
- **Test:** internal/cloudinit/TestGenerateFilesWithSharedMounts, internal/cloudinit/TestGenerateFilesWithMultipleSharedMounts, internal/cloudinit/TestGenerateFilesWithSharedMountsAndProvisioning, internal/cloudinit/TestSharedMountRuncmdsQuoteGuestPath, internal/cloudinit/TestYAMLSpecialCharactersInPaths

### Requirement: Guests without 9p are named
Before mounting, the guest SHALL check `/proc/filesystems` for 9p. When it is missing (RHEL-family kernels such as Rocky and AlmaLinux, and Debian's cloud kernel), up SHALL fail for that VM with "no 9p support" and tell the user to set `shared_folders: []` and run `vmherd reload <vm>`.

#### Scenario: Rocky guest with the default share
- **WHEN** up creates a Rocky 9 VM with the default share
- **THEN** up exits 1 and the error names the missing 9p support and the fix
- **Test:** internal/cloudinit/TestReportsMissingNineP, internal/cloudinit/TestNoNinePCheckWithoutShares, cmd/vmherd/TestMissingNinePSummary, test/e2e/TestDistroLifecycle (checkFirstUp on debian-trixie, rocky-9, almalinux-9)

### Requirement: Host folders must exist before VMs are created or started
Before `vmherd up` creates or starts anything, before `vmherd reload` destroys anything and before `vmherd restart` stops anything, vmherd SHALL check that every share's host path of the selected VMs exists and is a directory. If any doesn't, the command SHALL exit 1 naming each VM, host path and resolved path with how to fix it, and SHALL leave no new SSH key, network, disk or domain behind. Loading the Herdfile SHALL still accept a missing folder.

#### Scenario: Missing folder
- **WHEN** a VM shares `./terramino-go` and that folder doesn't exist
- **THEN** the check fails naming the VM, `./terramino-go`, its resolved path, and says to run `mkdir -p '<resolved path>'` (single-quoted, `'` escaped) or remove the share from the Herdfile
- **Test:** internal/config/TestCheckSharedFolderHostPathsExist (missing, missing with space and quote)

#### Scenario: Dangling symlink
- **WHEN** a share's host path, or one of its parents, is a symlink whose target doesn't exist
- **THEN** the check fails naming the VM, the path, the link and its missing target, and says to create the target or fix the link, without a `mkdir -p` hint
- **Test:** internal/config/TestCheckSharedFolderHostPathsExist (dangling symlink, dangling symlinked parent)

#### Scenario: Path is a file
- **WHEN** a share's host path exists but is a regular file
- **THEN** the check fails saying the host path is not a directory, naming the VM and path
- **Test:** internal/config/TestCheckSharedFolderHostPathsExist (file)

#### Scenario: Every problem at once
- **WHEN** two VMs each share a missing folder
- **THEN** one error lists both, in Herdfile order
- **Test:** internal/config/TestCheckSharedFolderHostPathsExist (two VMs)

#### Scenario: Folders present or no shares
- **WHEN** every share's host folder exists, or the VMs have `shared_folders: []`
- **THEN** the check passes
- **Test:** internal/config/TestCheckSharedFolderHostPathsExist (present, no shares)

#### Scenario: up leaves nothing behind
- **WHEN** the user runs `vmherd up` in a project whose VM shares a missing folder
- **THEN** it exits 1, the output names the folder, and no domain, disk or libvirt network for the project exists afterwards
- **Test:** test/e2e/TestUpRefusesMissingShareFolder

#### Scenario: reload changes nothing
- **WHEN** the user runs `vmherd reload` for a VM whose share folder was deleted after it was created
- **THEN** it exits 1 naming the folder before destroying anything, and the VM and its disk are untouched
- **Test:** cmd/vmherd/TestCheckReloadPrerequisitesStopsWhenShareFolderMissing (fails before any image download); reload's RunE calls it before destroying anything

#### Scenario: restart stops nothing
- **WHEN** the user runs `vmherd restart` for a VM whose share folder was deleted
- **THEN** it exits 1 naming the folder before stopping any VM, and running VMs keep running
- **Test:** cmd/vmherd/TestCheckRestartPrerequisitesStopsWhenShareFolderMissing; restart's RunE calls it before stopping anything

#### Scenario: Loading still accepts a missing folder
- **WHEN** a share's host folder was deleted and the user runs `vmherd status` or `vmherd destroy`
- **THEN** the Herdfile loads and the command works
- **Test:** internal/config/TestLoadAcceptsMissingShareFolder
