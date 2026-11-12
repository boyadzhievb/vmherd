# herdfile Specification

## Purpose

The Herdfile is the YAML file in the project directory that declares a project's VMs, images, network and SSH user. vmherd reads and checks it completely before it changes anything on the host.

## Requirements

### Requirement: Loading
Every command that acts on a project SHALL read `Herdfile` from the current directory, and SHALL fail with a hint to run `vmherd init` when the file is missing or empty.

#### Scenario: Valid Herdfile
- **WHEN** the Herdfile defines VMs, images, network and ssh sections with valid values
- **THEN** it loads with every value as written
- **Test:** internal/config/TestLoadValidHerdfile

#### Scenario: Missing Herdfile
- **WHEN** a command runs in a directory without a Herdfile
- **THEN** it fails with "cannot read Herdfile" and suggests `vmherd init`
- **Test:** internal/config/TestLoadMissingHerdfile

#### Scenario: Empty Herdfile
- **WHEN** the Herdfile is empty
- **THEN** loading fails, asks for at least one VM under `vms:` and points at `vmherd init`
- **Test:** internal/config/TestLoadRejectsEmptyHerdfile, internal/config/TestValidateNoVMs

### Requirement: Defaults
vmherd SHALL fill omitted values: SSH user `vmherd`, network type `nat`, network name `vmherd-<directory name>`, subnet `192.168.122.0/24`, DNS `auto`, disk `20G`, the default `.` → `/vagrant` share, and for forwarded ports protocol `tcp` and host address `127.0.0.1`.

#### Scenario: Minimal Herdfile
- **WHEN** a VM declares only name, image, cpus and memory
- **THEN** the loaded Herdfile carries the defaults above
- **Test:** internal/config/TestLoadDefaultValues

#### Scenario: Port forward defaults
- **WHEN** a port forward gives only `host` and `guest`
- **THEN** it forwards TCP and listens on 127.0.0.1 only
- **Test:** internal/config/TestPortForwardDefaults

### Requirement: Every problem reported at once
vmherd SHALL check the whole Herdfile before any operation and SHALL report all problems in one error, each naming the VM or section and the bad value, with a hint to fix it.

#### Scenario: Several problems
- **WHEN** the Herdfile has several invalid values in different VMs
- **THEN** one error lists every problem, numbered by count ("Herdfile has N problems")
- **Test:** internal/config/TestLoadReportsAllValidationProblems

#### Scenario: Unknown key
- **WHEN** the Herdfile contains a misspelled key such as `memroy`
- **THEN** the error names the key, its line number and the closest known key
- **Test:** internal/config/TestLoadReportsUnknownKeysWithLineNumbers, internal/config/TestClosestKeyIgnoresUnrelatedKeys

#### Scenario: Wrong type
- **WHEN** a value has the wrong YAML type (a string for `cpus`)
- **THEN** the error names the field and its line
- **Test:** internal/config/TestLoadReportsWronglyTypedValues

#### Scenario: YAML syntax error
- **WHEN** the Herdfile is not valid YAML
- **THEN** the error says so with the line of the syntax error
- **Test:** internal/config/TestLoadReportsYAMLSyntaxError

#### Scenario: One bad value reported once
- **WHEN** the subnet is invalid and several VMs have IPs
- **THEN** the bad subnet is reported once, not once per VM
- **Test:** internal/config/TestLoadReportsBadSubnetOnce

### Requirement: VM identity and resources
Each VM SHALL have a unique name of letters, digits and hyphens (at most 63 characters, starting and ending alphanumeric), an image in `distro:codename` form, at least the minimum CPUs and memory (memory in MB), and a disk size of a number followed by K, M, G or T.

#### Scenario: Duplicate name
- **WHEN** two VMs have the same name
- **THEN** loading fails naming the duplicate
- **Test:** internal/config/TestValidateDuplicateNames

#### Scenario: Invalid names
- **WHEN** a VM name contains special characters, is longer than 63 characters or starts with a hyphen
- **THEN** loading fails with the naming rule
- **Test:** internal/config/TestValidateVMNameSpecialChars, internal/config/TestValidateVMNameTooLong, internal/config/TestValidateVMNameStartsWithHyphen, internal/config/TestValidateVMNameValid

#### Scenario: Unnamed VM
- **WHEN** a VM has no name
- **THEN** it is reported as `#<position>` and its other problems are still reported
- **Test:** internal/config/TestLoadReportsUnnamedVMsOtherProblems

#### Scenario: Image missing or malformed
- **WHEN** a VM has no image, or an image without `distro:codename`
- **THEN** loading fails with an example such as `ubuntu:noble`
- **Test:** internal/config/TestValidateMissingImage, internal/config/TestValidateBadImageFormat

#### Scenario: Resources and disk
- **WHEN** cpus or memory is below the minimum, or disk is not like `20G`
- **THEN** loading fails naming the field and the accepted form
- **Test:** internal/config/TestValidateFieldRules, internal/config/TestValidateDiskSizeInvalid, internal/config/TestValidateDiskSizeValid

### Requirement: Images section
The Herdfile SHALL map every image a VM uses to an `https://` download URL under `images:`; vmherd SHALL NOT fall back to built-in URLs for a project.

#### Scenario: Image not defined
- **WHEN** a VM uses an image that is not under `images:`
- **THEN** loading fails naming the image and the defined ones
- **Test:** internal/config/TestValidateFieldRules

#### Scenario: Non-HTTPS URL
- **WHEN** an image URL is not `https://`
- **THEN** loading fails: vmherd only downloads over HTTPS
- **Test:** internal/config/TestValidateFieldRules

### Requirement: SSH user
`ssh.user` SHALL start with a lowercase letter or underscore and contain only lowercase letters, digits, hyphens and underscores. A plaintext `ssh.password` SHALL produce a warning on `up` and `reload`.

#### Scenario: Invalid user
- **WHEN** `ssh.user` contains uppercase letters or starts with a digit
- **THEN** loading fails with the naming rule
- **Test:** internal/config/TestValidateSSHUserInvalid, internal/config/TestValidateSSHUserValid

#### Scenario: Plaintext password
- **WHEN** the Herdfile sets `ssh.password` and the user runs `vmherd up`
- **THEN** a warning says not to commit the file
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: Selecting VMs by name
Commands that take VM names SHALL act on all VMs when no name is given, and SHALL fail before acting when a name is not in the Herdfile.

#### Scenario: Unknown name
- **WHEN** the user runs `vmherd up web` and no VM is named `web`
- **THEN** the command fails naming `web` and the defined VMs
- **Test:** internal/config/TestFilterVMs

### Requirement: Example Herdfile
`vmherd init` SHALL write an example Herdfile that loads without problems, and SHALL refuse to overwrite an existing one.

#### Scenario: Example is valid
- **WHEN** the example Herdfile is generated
- **THEN** it is valid YAML and passes validation
- **Test:** internal/config/TestExampleHerdfileIsValidYAML, internal/config/TestExampleHerdfilePassesValidation, internal/config/TestBuiltInImagesAreValid

#### Scenario: Herdfile exists
- **WHEN** the user runs `vmherd init` where a Herdfile exists
- **THEN** it fails with "Herdfile already exists" and leaves the file unchanged
- **Test:** none yet — cmd/ code, needs M2.3
