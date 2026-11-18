## ADDED Requirements

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
