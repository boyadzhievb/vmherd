package config

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"
)

const (
	// minVMMemoryMB is the smallest memory a VM may have; cloud images with
	// cloud-init don't boot reliably below it.
	minVMMemoryMB = 256
	// minVMCPUs is the smallest number of virtual CPUs a VM may have.
	minVMCPUs = 1
	// autoDNS selects the DNS server from the host's /etc/resolv.conf.
	autoDNS = "auto"
)

// validateVMResources checks that cpus and memory are usable. Omitted (0)
// values got their defaults earlier, so only explicit bad values fail here.
func validateVMResources(vmConfig VM) []error {
	var problems []error
	if vmConfig.CPUs < minVMCPUs {
		problems = append(problems, fmt.Errorf("VM %q: cpus is %d — use at least %d", vmConfig.Name, vmConfig.CPUs, minVMCPUs))
	}
	if vmConfig.Memory < minVMMemoryMB {
		problems = append(problems, fmt.Errorf("VM %q: memory is %d MB — use at least %d (the value is in megabytes, e.g. 2048)",
			vmConfig.Name, vmConfig.Memory, minVMMemoryMB))
	}
	return problems
}

// validateVMGateway checks a static VM's gateway override: an IPv4 address
// in the subnet (when the subnet is valid), not the network or broadcast
// address and not the VM's own address. DHCP VMs are checked elsewhere.
func validateVMGateway(vmConfig VM, subnetCIDR *net.IPNet) error {
	gateway := vmConfig.Network.Gateway
	if gateway == "" || vmConfig.Network.IP == "" {
		return nil
	}
	gatewayIP := net.ParseIP(gateway).To4()
	if gatewayIP == nil {
		return fmt.Errorf("VM %q: network.gateway %q is not an IPv4 address — omit it to use the subnet's .1", vmConfig.Name, gateway)
	}
	if gateway == vmConfig.Network.IP {
		return fmt.Errorf("VM %q: network.gateway %s is the VM's own address — use the router's address, usually the subnet's .1", vmConfig.Name, gateway)
	}
	if subnetCIDR == nil {
		return nil
	}
	if !subnetCIDR.Contains(gatewayIP) {
		return fmt.Errorf("VM %q: network.gateway %s is outside the network subnet %s", vmConfig.Name, gateway, subnetCIDR)
	}
	if lastOctet := gatewayIP[3]; lastOctet == 0 || lastOctet == 255 {
		return fmt.Errorf("VM %q: network.gateway %s is the subnet's network or broadcast address — use a host address such as the subnet's .1", vmConfig.Name, gateway)
	}
	return nil
}

// validateDNS checks network.dns: "auto" (taken from the host) or an IP address.
func validateDNS(dnsSetting string) error {
	if dnsSetting == autoDNS || net.ParseIP(dnsSetting) != nil {
		return nil
	}
	return fmt.Errorf("network.dns %q must be 'auto' (use the host's DNS server) or an IP address such as 192.168.1.1", dnsSetting)
}

// validateProvisionStep checks one provisioning step: a known type with the
// keys that type uses, and none of the other type's keys.
func validateProvisionStep(vmName string, stepNumber int, step Provision) []error {
	stepLabel := fmt.Sprintf("VM %q: provision step %d", vmName, stepNumber)
	switch step.Type {
	case ProvisionShell:
		var problems []error
		if strings.TrimSpace(step.Inline) == "" {
			problems = append(problems, fmt.Errorf("%s (shell) has no 'inline' script — add the commands or remove the step", stepLabel))
		}
		if step.Source != "" || step.Destination != "" {
			problems = append(problems, fmt.Errorf("%s (shell) has 'source'/'destination', which only file steps use — use type: file to copy a file", stepLabel))
		}
		return problems
	case ProvisionFile:
		return validateFileProvisionStep(stepLabel, step)
	case "":
		return []error{fmt.Errorf("%s has no 'type' — use type: shell or type: file", stepLabel)}
	default:
		return []error{fmt.Errorf("%s has unknown type %q — use shell or file", stepLabel, step.Type)}
	}
}

// validateFileProvisionStep checks a file step's source and destination keys
// (the source path policy is checked by validateProvisionSteps).
func validateFileProvisionStep(stepLabel string, step Provision) []error {
	var problems []error
	if step.Source == "" {
		problems = append(problems, fmt.Errorf("%s (file) has no 'source' — the path of the file to copy, relative to the project", stepLabel))
	}
	if step.Inline != "" {
		problems = append(problems, fmt.Errorf("%s (file) has 'inline', which only shell steps use — use a separate type: shell step", stepLabel))
	}
	switch {
	case step.Destination == "":
		problems = append(problems, fmt.Errorf("%s (file) has no 'destination' — the absolute path in the VM, e.g. /tmp/setup.sh", stepLabel))
	case !isCleanGuestPath(step.Destination):
		problems = append(problems, fmt.Errorf("%s (file) destination %q must be an absolute path in the VM (start with /) without control characters", stepLabel, step.Destination))
	}
	return problems
}

// isCleanGuestPath reports whether guestPath is absolute and free of control
// characters (newlines and the like have no place in a path and would break
// the generated cloud-init and shell lines).
func isCleanGuestPath(guestPath string) bool {
	return path.IsAbs(guestPath) && strings.IndexFunc(guestPath, unicode.IsControl) < 0
}

// validateImages checks the images section (every key 'distro:codename',
// every value an https URL) and that each VM's image is defined there.
func validateImages(images map[string]string, vms []VM) []error {
	var problems []error
	imageNames := make([]string, 0, len(images))
	for imageName := range images {
		imageNames = append(imageNames, imageName)
	}
	sort.Strings(imageNames)

	for _, imageName := range imageNames {
		if !isDistroCodename(imageName) {
			problems = append(problems, fmt.Errorf("images: key %q must be 'distro:codename' (e.g. ubuntu:noble)", imageName))
		}
		if err := validateImageURL(imageName, images[imageName]); err != nil {
			problems = append(problems, err)
		}
	}
	return append(problems, validateVMImagesDefined(images, imageNames, vms)...)
}

// validateImageURL checks that an image's download URL is an https URL with a
// host; vmherd refuses to download over plain http.
func validateImageURL(imageName, imageURL string) error {
	parsedURL, err := url.Parse(imageURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return fmt.Errorf("images: %s URL %q must be an https:// URL of the vendor's cloud image (vmherd only downloads over HTTPS)", imageName, imageURL)
	}
	return nil
}

// validateVMImagesDefined checks that every VM's image is a key of images.
// VMs with a malformed image are skipped (validateVMIdentity reports them).
func validateVMImagesDefined(images map[string]string, imageNames []string, vms []VM) []error {
	if len(images) == 0 {
		if len(vms) == 0 {
			return nil
		}
		return []error{fmt.Errorf("no 'images:' section — add one with a download URL for each image the VMs use, e.g.\n      images:\n        %s: https://…\n    (run 'vmherd init' in an empty directory for an example)", vms[0].Image)}
	}
	var problems []error
	for vmIndex, vmConfig := range vms {
		if !isDistroCodename(vmConfig.Image) {
			continue
		}
		if _, defined := images[vmConfig.Image]; !defined {
			problems = append(problems, fmt.Errorf("VM %s: image %q is not defined under 'images:' — defined: %s",
				describeVM(vmConfig, vmIndex), vmConfig.Image, strings.Join(imageNames, ", ")))
		}
	}
	return problems
}

// isDistroCodename reports whether an image name has the 'distro:codename' form.
func isDistroCodename(imageName string) bool {
	distro, codename, found := strings.Cut(imageName, ":")
	return found && distro != "" && codename != ""
}

// describeVM names a VM in messages: its quoted name, or "#<position>" for a
// VM without one.
func describeVM(vmConfig VM, vmIndex int) string {
	if vmConfig.Name == "" {
		return fmt.Sprintf("#%d", vmIndex+1)
	}
	return fmt.Sprintf("%q", vmConfig.Name)
}
