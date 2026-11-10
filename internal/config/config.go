package config

// Herdfile represents the top-level vmherd project configuration.
type Herdfile struct {
	Images  map[string]string `yaml:"images"`  // image code to download URL (e.g. "ubuntu:noble" -> "https://...")
	VMs     []VM              `yaml:"vms"`     // virtual machines to manage
	Network NetworkConfig     `yaml:"network"` // shared libvirt network settings
	SSH     SSHConfig         `yaml:"ssh"`     // default SSH credentials
}

// VM defines a single virtual machine.
type VM struct {
	Name          string          `yaml:"name"`           // unique identifier within the Herdfile
	Image         string          `yaml:"image"`          // distro:codename format (e.g. ubuntu:noble)
	CPUs          int             `yaml:"cpus"`           // virtual CPU count
	Memory        int             `yaml:"memory"`         // RAM in megabytes
	Disk          string          `yaml:"disk"`           // disk size with unit (e.g. 20G)
	Network       VMNetworkConfig `yaml:"network"`        // per-VM network settings
	SharedFolders []SharedFolder  `yaml:"shared_folders"` // host-to-guest directory mounts
	Ports         []PortForward   `yaml:"ports"`          // host-to-guest port mappings
	Provision     []Provision     `yaml:"provision"`      // provisioning steps run after boot
	ConsoleLog    bool            `yaml:"console_log"`    // capture serial console to a log file in state dir
}

// SharedFolder maps a host directory into the guest filesystem.
type SharedFolder struct {
	Host             string `yaml:"host"`               // path on the host (relative to project dir)
	Guest            string `yaml:"guest"`              // absolute mount path inside the guest
	AllowOutsideHome bool   `yaml:"allow_outside_home"` // permit a host path outside the project dir and $HOME
	ResolvedHost     string `yaml:"-"`                  // absolute, symlink-free host path set during validation
}

// PortForward maps a host port to a guest port.
type PortForward struct {
	Host     int    `yaml:"host"`     // port number on the host
	Guest    int    `yaml:"guest"`    // port number inside the guest
	Protocol string `yaml:"protocol"` // only "tcp": ports are forwarded over SSH
	HostIP   string `yaml:"host_ip"`  // host address to listen on; defaults to 127.0.0.1 (this machine only)
}

// VMNetworkConfig holds per-VM network settings.
type VMNetworkConfig struct {
	IP      string `yaml:"ip"`      // static IPv4 address
	Gateway string `yaml:"gateway"` // gateway override (defaults to subnet .1)
}

// NetworkType selects how VMs reach the outside world.
type NetworkType string

const (
	// NetworkTypeNAT puts VMs on a vmherd-managed libvirt NAT network (the default).
	NetworkTypeNAT NetworkType = "nat"
	// NetworkTypeBridge attaches VMs to an existing host bridge, so they appear
	// on the LAN with their own addresses. No libvirt network is created.
	NetworkTypeBridge NetworkType = "bridge"
)

// NetworkConfig defines the project network: a vmherd-managed libvirt NAT
// network (type nat) or an existing host bridge (type bridge).
type NetworkConfig struct {
	Type    NetworkType `yaml:"type"`    // nat (default) or bridge
	Name    string      `yaml:"name"`    // libvirt network name (nat mode)
	Subnet  string      `yaml:"subnet"`  // CIDR notation (e.g. 192.168.122.0/24); in bridge mode defaults to the gateway's /24
	DNS     string      `yaml:"dns"`     // "auto" or explicit DNS server IP
	Bridge  string      `yaml:"bridge"`  // host bridge device VMs attach to (bridge mode, e.g. br1)
	Gateway string      `yaml:"gateway"` // LAN router IP the VMs route through (bridge mode)
}

// IsBridge reports whether the project attaches VMs to a host bridge instead
// of a vmherd-managed libvirt network.
func (networkConfig NetworkConfig) IsBridge() bool {
	return networkConfig.Type == NetworkTypeBridge
}

// SSHConfig holds SSH-related defaults.
type SSHConfig struct {
	User     string `yaml:"user"`     // default username created on all VMs
	Password string `yaml:"password"` // optional password for the SSH user
}

// ProvisionType identifies the kind of provisioning step.
type ProvisionType string

const (
	// ProvisionShell runs inline shell commands inside the guest.
	ProvisionShell ProvisionType = "shell"
	// ProvisionFile copies a file from the host into the guest.
	ProvisionFile ProvisionType = "file"
)

// Provision defines a single provisioning step.
type Provision struct {
	Type        ProvisionType `yaml:"type"`        // provisioning step kind (shell or file)
	Inline      string        `yaml:"inline"`      // shell commands to run (shell type)
	Source      string        `yaml:"source"`      // local file path (file type)
	Destination string        `yaml:"destination"` // guest file path (file type)
	Errexit     *bool         `yaml:"errexit"`     // shell: stop at the first failing command (default true; false = plain bash)
}

// BashOptions returns the bash options a shell step runs with, on first boot
// and in 'vmherd provision' alike: -e -o pipefail (stop at the first failing
// command, also inside a pipe, as GitHub Actions does) unless the step sets
// errexit: false (plain bash, where only the last command's status counts).
func (step Provision) BashOptions() []string {
	if step.Errexit != nil && !*step.Errexit {
		return nil
	}
	return []string{"-e", "-o", "pipefail"}
}

// VMNames returns the names of all VMs in the Herdfile, in declaration order.
func (herdfile *Herdfile) VMNames() []string {
	vmNames := make([]string, 0, len(herdfile.VMs))
	for _, vmDefinition := range herdfile.VMs {
		vmNames = append(vmNames, vmDefinition.Name)
	}
	return vmNames
}
