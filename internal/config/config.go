package config

// Herdfile represents the top-level vmherd project configuration.
type Herdfile struct {
	VMs     []VM          `yaml:"vms"`     // virtual machines to manage
	Network NetworkConfig `yaml:"network"` // shared libvirt network settings
	SSH     SSHConfig     `yaml:"ssh"`     // default SSH credentials
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
}

// SharedFolder maps a host directory into the guest filesystem.
type SharedFolder struct {
	Host  string `yaml:"host"`  // path on the host (relative to project dir)
	Guest string `yaml:"guest"` // absolute mount path inside the guest
}

// PortForward maps a host port to a guest port.
type PortForward struct {
	Host     int    `yaml:"host"`     // port number on the host
	Guest    int    `yaml:"guest"`    // port number inside the guest
	Protocol string `yaml:"protocol"` // "tcp" or "udp"
}

// VMNetworkConfig holds per-VM network settings.
type VMNetworkConfig struct {
	IP      string `yaml:"ip"`      // static IPv4 address
	Gateway string `yaml:"gateway"` // gateway override (defaults to subnet .1)
}

// NetworkConfig defines the libvirt network for the project.
type NetworkConfig struct {
	Name   string `yaml:"name"`   // libvirt network name
	Subnet string `yaml:"subnet"` // CIDR notation (e.g. 192.168.122.0/24)
	DNS    string `yaml:"dns"`    // "auto" or explicit DNS server IP
}

// SSHConfig holds SSH-related defaults.
type SSHConfig struct {
	User     string `yaml:"user"`     // default username created on all VMs
	Password string `yaml:"password"` // optional password for the SSH user
}

// Provision defines a single provisioning step.
type Provision struct {
	Type        string `yaml:"type"`        // "shell" or "file"
	Inline      string `yaml:"inline"`      // shell commands to run (shell type)
	Source      string `yaml:"source"`      // local file path (file type)
	Destination string `yaml:"destination"` // guest file path (file type)
}
