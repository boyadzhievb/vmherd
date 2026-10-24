package config

// Herdfile represents the top-level vmherd project configuration.
type Herdfile struct {
	VMs     []VM          `yaml:"vms"`
	Network NetworkConfig `yaml:"network"`
	SSH     SSHConfig     `yaml:"ssh"`
}

// VM defines a single virtual machine.
type VM struct {
	Name          string          `yaml:"name"`
	Image         string          `yaml:"image"`
	CPUs          int             `yaml:"cpus"`
	Memory        int             `yaml:"memory"`
	Disk          string          `yaml:"disk"`
	Network       VMNetworkConfig `yaml:"network"`
	SharedFolders []SharedFolder  `yaml:"shared_folders"`
	Ports         []PortForward   `yaml:"ports"`
	Provision     []Provision     `yaml:"provision"`
}

// SharedFolder maps a host directory into the guest filesystem.
type SharedFolder struct {
	Host  string `yaml:"host"`
	Guest string `yaml:"guest"`
}

// PortForward maps a host port to a guest port.
type PortForward struct {
	Host     int    `yaml:"host"`
	Guest    int    `yaml:"guest"`
	Protocol string `yaml:"protocol"`
}

// VMNetworkConfig holds per-VM network settings.
type VMNetworkConfig struct {
	IP      string `yaml:"ip"`
	Gateway string `yaml:"gateway"`
}

// NetworkConfig defines the libvirt network for the project.
type NetworkConfig struct {
	Name   string `yaml:"name"`
	Subnet string `yaml:"subnet"`
	DNS    string `yaml:"dns"`
}

// SSHConfig holds SSH-related defaults.
type SSHConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// Provision defines a single provisioning step.
type Provision struct {
	Type        string `yaml:"type"`
	Inline      string `yaml:"inline"`
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
}
