package vm

import (
	"encoding/xml"
	"fmt"
)

// sharedFolderAccessMode is the 9p access mode for shared folders. "squash"
// keeps real file modes on the host and ignores guest chown requests.
const sharedFolderAccessMode = "squash"

// qemuGuestAgentChannel is the target name for the QEMU guest agent virtio channel.
const qemuGuestAgentChannel = "org.qemu.guest_agent.0"

// domainXML is the root element of a libvirt domain definition.
type domainXML struct {
	XMLName    xml.Name         `xml:"domain"`
	Type       string           `xml:"type,attr"`
	Name       string           `xml:"name"`
	Memory     domainMemory     `xml:"memory"`
	VCPU       domainVCPU       `xml:"vcpu"`
	OS         domainOS         `xml:"os"`
	Features   domainFeatures   `xml:"features"`
	CPU        domainCPU        `xml:"cpu"`
	Clock      domainClock      `xml:"clock"`
	OnPoweroff string           `xml:"on_poweroff"`
	OnReboot   string           `xml:"on_reboot"`
	OnCrash    string           `xml:"on_crash"`
	Devices    domainDevices    `xml:"devices"`
	SecLabels  []domainSecLabel `xml:"seclabel,omitempty"`
}

// domainMemory specifies the maximum memory for the domain.
type domainMemory struct {
	Unit  string `xml:"unit,attr"`
	Value int    `xml:",chardata"`
}

// domainVCPU specifies the virtual CPU count.
type domainVCPU struct {
	Placement string `xml:"placement,attr"`
	Value     int    `xml:",chardata"`
}

// domainOS describes the guest operating system boot configuration.
type domainOS struct {
	Type domainOSType `xml:"type"`
}

// domainOSType specifies the machine type and architecture.
type domainOSType struct {
	Arch    string `xml:"arch,attr"`
	Machine string `xml:"machine,attr"`
	Value   string `xml:",chardata"`
}

// domainFeatures lists the hypervisor features enabled for the domain.
type domainFeatures struct {
	ACPI *struct{} `xml:"acpi"`
	APIC *struct{} `xml:"apic"`
}

// domainCPU specifies the CPU model and features.
type domainCPU struct {
	Mode string `xml:"mode,attr"`
}

// domainClock specifies the clock configuration.
type domainClock struct {
	Offset string `xml:"offset,attr"`
}

// domainDevices contains all virtual devices attached to the domain.
type domainDevices struct {
	Disks       []domainDisk       `xml:"disk"`
	Controllers []domainController `xml:"controller,omitempty"`
	Interfaces  []domainInterface  `xml:"interface"`
	Serials     []domainSerial     `xml:"serial,omitempty"`
	Consoles    []domainConsole    `xml:"console,omitempty"`
	Channels    []domainChannel    `xml:"channel,omitempty"`
	Inputs      []domainInput      `xml:"input,omitempty"`
	Graphics    []domainGraphics   `xml:"graphics,omitempty"`
	RNGs        []domainRNG        `xml:"rng,omitempty"`
	Memballoon  *domainMemballoon  `xml:"memballoon,omitempty"`
	Filesystems []domainFilesystem `xml:"filesystem,omitempty"`
}

// domainDisk describes a block device attached to the domain.
type domainDisk struct {
	Type   string           `xml:"type,attr"`
	Device string           `xml:"device,attr"`
	Driver domainDiskDriver `xml:"driver"`
	Source domainDiskSource `xml:"source"`
	Target domainDiskTarget `xml:"target"`
}

// domainDiskDriver specifies the disk format and caching strategy.
type domainDiskDriver struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
}

// domainDiskSource identifies the backing file for a disk.
type domainDiskSource struct {
	File string `xml:"file,attr"`
}

// domainDiskTarget identifies the guest device name and bus.
type domainDiskTarget struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

// domainController describes a device controller (USB, PCI, etc).
type domainController struct {
	Type  string `xml:"type,attr"`
	Model string `xml:"model,attr,omitempty"`
}

// domainInterface describes a network interface.
type domainInterface struct {
	Type   string                `xml:"type,attr"`
	Source domainInterfaceSource `xml:"source"`
	MAC    *domainInterfaceMAC   `xml:"mac,omitempty"`
	Model  domainInterfaceModel  `xml:"model"`
}

// domainInterfaceSource identifies what the interface connects to: a libvirt
// network (NAT mode) or a host bridge device (bridge mode).
type domainInterfaceSource struct {
	Network string `xml:"network,attr,omitempty"`
	Bridge  string `xml:"bridge,attr,omitempty"`
}

// domainInterfaceMAC sets the MAC address for the interface.
type domainInterfaceMAC struct {
	Address string `xml:"address,attr"`
}

// domainInterfaceModel specifies the emulated NIC model.
type domainInterfaceModel struct {
	Type string `xml:"type,attr"`
}

// domainSerial describes a serial port.
type domainSerial struct {
	Type   string              `xml:"type,attr"`
	Source *domainSerialSource `xml:"source,omitempty"`
	Target domainSerialTarget  `xml:"target"`
}

// domainSerialSource specifies the host-side backing for a serial port.
type domainSerialSource struct {
	Path string `xml:"path,attr"`
}

// domainSerialTarget identifies the serial port number.
type domainSerialTarget struct {
	Port int `xml:"port,attr"`
}

// domainConsole describes a console device mirroring a serial port.
type domainConsole struct {
	Type   string              `xml:"type,attr"`
	Target domainConsoleTarget `xml:"target"`
}

// domainConsoleTarget identifies the console type and port.
type domainConsoleTarget struct {
	Type string `xml:"type,attr"`
	Port int    `xml:"port,attr"`
}

// domainChannel describes a virtio channel device (e.g. guest agent).
type domainChannel struct {
	Type   string              `xml:"type,attr"`
	Target domainChannelTarget `xml:"target"`
}

// domainChannelTarget specifies the channel type and name.
type domainChannelTarget struct {
	Type string `xml:"type,attr"`
	Name string `xml:"name,attr"`
}

// domainInput describes an input device (mouse, keyboard, tablet).
type domainInput struct {
	Type string `xml:"type,attr"`
	Bus  string `xml:"bus,attr"`
}

// domainGraphics describes a graphics device (VNC, Spice).
type domainGraphics struct {
	Type       string `xml:"type,attr"`
	AutoSocket string `xml:"autoSocket,attr,omitempty"`
}

// domainRNG describes a virtio random number generator.
type domainRNG struct {
	Model   string        `xml:"model,attr"`
	Backend domainRNGBack `xml:"backend"`
}

// domainRNGBack specifies the host entropy source for the RNG.
type domainRNGBack struct {
	Model string `xml:"model,attr"`
	Value string `xml:",chardata"`
}

// domainMemballoon describes the memory balloon device.
type domainMemballoon struct {
	Model string `xml:"model,attr"`
}

// domainFilesystem describes a host-to-guest filesystem passthrough (9p).
type domainFilesystem struct {
	Type       string                 `xml:"type,attr"`
	AccessMode string                 `xml:"accessmode,attr"`
	Source     domainFilesystemSource `xml:"source"`
	Target     domainFilesystemTarget `xml:"target"`
}

// domainFilesystemSource identifies the host directory.
type domainFilesystemSource struct {
	Dir string `xml:"dir,attr"`
}

// domainFilesystemTarget identifies the 9p mount tag.
type domainFilesystemTarget struct {
	Dir string `xml:"dir,attr"`
}

// domainSecLabel controls QEMU's security label (DAC user/group, SELinux, etc).
type domainSecLabel struct {
	Type    string `xml:"type,attr"`
	Model   string `xml:"model,attr"`
	Relabel string `xml:"relabel,attr"`
	Label   string `xml:"label"`
}

// RenderDomainXML returns the libvirt domain XML for the given options. All values
// are escaped by encoding/xml, so path injection through filenames is not possible.
func RenderDomainXML(opts CreateOptions) ([]byte, error) {
	domain := domainXML{
		Type: "kvm",
		Name: opts.DomainName,
		Memory: domainMemory{
			Unit:  "MiB",
			Value: opts.Memory,
		},
		VCPU: domainVCPU{
			Placement: "static",
			Value:     opts.CPUs,
		},
		OS: domainOS{
			Type: domainOSType{
				Arch:    "x86_64",
				Machine: "q35",
				Value:   "hvm",
			},
		},
		Features: domainFeatures{
			ACPI: &struct{}{},
			APIC: &struct{}{},
		},
		CPU: domainCPU{
			Mode: "host-passthrough",
		},
		Clock: domainClock{
			Offset: "utc",
		},
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "destroy",
	}

	domain.Devices.Disks = []domainDisk{
		{
			Type:   "file",
			Device: "disk",
			Driver: domainDiskDriver{Name: "qemu", Type: "qcow2"},
			Source: domainDiskSource{File: opts.DiskPath},
			Target: domainDiskTarget{Dev: "vda", Bus: "virtio"},
		},
		{
			Type:   "file",
			Device: "cdrom",
			Driver: domainDiskDriver{Name: "qemu", Type: "raw"},
			Source: domainDiskSource{File: opts.SeedPath},
			Target: domainDiskTarget{Dev: "sda", Bus: "sata"},
		},
	}

	domain.Devices.Interfaces = []domainInterface{buildPrimaryInterface(opts)}

	if opts.ConsoleLogPath != "" {
		domain.Devices.Serials = []domainSerial{
			{
				Type:   "file",
				Source: &domainSerialSource{Path: opts.ConsoleLogPath},
				Target: domainSerialTarget{Port: 0},
			},
		}
	} else {
		domain.Devices.Serials = []domainSerial{
			{
				Type:   "pty",
				Target: domainSerialTarget{Port: 0},
			},
		}
	}
	domain.Devices.Consoles = []domainConsole{
		{
			Type:   "pty",
			Target: domainConsoleTarget{Type: "serial", Port: 0},
		},
	}

	domain.Devices.Channels = []domainChannel{
		{
			Type:   "unix",
			Target: domainChannelTarget{Type: "virtio", Name: qemuGuestAgentChannel},
		},
	}

	domain.Devices.Inputs = []domainInput{
		{Type: "tablet", Bus: "usb"},
	}

	domain.Devices.Graphics = []domainGraphics{
		{Type: "vnc", AutoSocket: "yes"},
	}

	domain.Devices.RNGs = []domainRNG{
		{
			Model:   "virtio",
			Backend: domainRNGBack{Model: "random", Value: "/dev/urandom"},
		},
	}

	domain.Devices.Memballoon = &domainMemballoon{Model: "virtio"}

	for _, sharedFolder := range opts.SharedFolders {
		domain.Devices.Filesystems = append(domain.Devices.Filesystems, domainFilesystem{
			Type:       "mount",
			AccessMode: sharedFolderAccessMode,
			Source:     domainFilesystemSource{Dir: sharedFolder.HostPath},
			Target:     domainFilesystemTarget{Dir: sharedFolder.MountTag},
		})
	}

	if len(opts.SharedFolders) > 0 {
		if opts.QEMUProcessLabel == "" {
			return nil, fmt.Errorf("shared folders need QEMUProcessLabel so QEMU runs as the host user, not as root or libvirt-qemu")
		}
		domain.SecLabels = []domainSecLabel{
			{
				Type:    "static",
				Model:   "dac",
				Relabel: "yes",
				Label:   opts.QEMUProcessLabel,
			},
		}
	}

	output, err := xml.MarshalIndent(domain, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling domain XML for %s: %w", opts.DomainName, err)
	}
	return append([]byte(xml.Header), output...), nil
}

// buildPrimaryInterface returns the VM's network interface: attached to the
// host bridge when BridgeName is set, to the libvirt network otherwise.
func buildPrimaryInterface(opts CreateOptions) domainInterface {
	interfaceSource := domainInterfaceSource{Network: opts.NetworkName}
	interfaceType := "network"
	if opts.BridgeName != "" {
		interfaceSource = domainInterfaceSource{Bridge: opts.BridgeName}
		interfaceType = "bridge"
	}
	return domainInterface{
		Type:   interfaceType,
		Source: interfaceSource,
		MAC:    &domainInterfaceMAC{Address: opts.MACAddress},
		Model:  domainInterfaceModel{Type: "virtio"},
	}
}
