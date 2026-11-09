package config

import (
	"strings"
	"testing"
)

// validBaseHerdfile returns a Herdfile that passes validation: one static VM
// with a shell and a file step, no shares, defaults applied.
func validBaseHerdfile(t *testing.T, projectDir string) *Herdfile {
	t.Helper()
	herdfile := &Herdfile{
		Images: map[string]string{"ubuntu:noble": "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"},
		VMs: []VM{{
			Name:          "web",
			Image:         "ubuntu:noble",
			Network:       VMNetworkConfig{IP: "192.168.150.10"},
			SharedFolders: []SharedFolder{},
			Provision: []Provision{
				{Type: ProvisionShell, Inline: "apt-get update"},
				{Type: ProvisionFile, Source: "setup.sh", Destination: "/tmp/setup.sh"},
			},
		}},
		Network: NetworkConfig{Name: "vmherd-web", Subnet: "192.168.150.0/24"},
	}
	applyDefaults(herdfile, projectDir)
	return herdfile
}

// TestValidateFieldRules has one case per M1.10 rule: each change to the
// valid base Herdfile must produce exactly the expected problem.
func TestValidateFieldRules(t *testing.T) {
	testCases := []struct {
		name         string
		change       func(herdfile *Herdfile)
		wantProblems string // "" means the change is valid
	}{
		{"negative cpus", func(h *Herdfile) { h.VMs[0].CPUs = -2 }, `VM "web": cpus is -2 — use at least 1`},
		{"memory below minimum", func(h *Herdfile) { h.VMs[0].Memory = 128 }, `VM "web": memory is 128 MB — use at least 256`},
		{"memory at minimum", func(h *Herdfile) { h.VMs[0].Memory = 256 }, ""},
		{"unknown provision type", func(h *Herdfile) { h.VMs[0].Provision[0].Type = "ansible" }, `provision step 1 has unknown type "ansible"`},
		{"missing provision type", func(h *Herdfile) { h.VMs[0].Provision[0].Type = "" }, "provision step 1 has no 'type'"},
		{"empty shell inline", func(h *Herdfile) { h.VMs[0].Provision[0].Inline = "  \n" }, "provision step 1 (shell) has no 'inline' script"},
		{"shell step with destination", func(h *Herdfile) { h.VMs[0].Provision[0].Destination = "/tmp/x" }, "provision step 1 (shell) has 'source'/'destination'"},
		{"file step without source", func(h *Herdfile) { h.VMs[0].Provision[1].Source = "" }, "provision step 2 (file) has no 'source'"},
		{"file step with inline", func(h *Herdfile) { h.VMs[0].Provision[1].Inline = "echo" }, "provision step 2 (file) has 'inline'"},
		{"file step without destination", func(h *Herdfile) { h.VMs[0].Provision[1].Destination = "" }, "provision step 2 (file) has no 'destination'"},
		{"relative destination", func(h *Herdfile) { h.VMs[0].Provision[1].Destination = "tmp/setup.sh" }, `destination "tmp/setup.sh" must be an absolute path`},
		{"destination with newline", func(h *Herdfile) { h.VMs[0].Provision[1].Destination = "/tmp/a\n  owner: root" }, "must be an absolute path in the VM (start with /) without control characters"},
		{"gateway not an IP", func(h *Herdfile) { h.VMs[0].Network.Gateway = "router" }, `network.gateway "router" is not an IPv4 address`},
		{"gateway IPv6", func(h *Herdfile) { h.VMs[0].Network.Gateway = "fe80::1" }, `network.gateway "fe80::1" is not an IPv4 address`},
		{"gateway outside subnet", func(h *Herdfile) { h.VMs[0].Network.Gateway = "10.0.0.1" }, "network.gateway 10.0.0.1 is outside the network subnet 192.168.150.0/24"},
		{"gateway is broadcast", func(h *Herdfile) { h.VMs[0].Network.Gateway = "192.168.150.255" }, "network or broadcast address"},
		{"gateway is the VM", func(h *Herdfile) { h.VMs[0].Network.Gateway = "192.168.150.10" }, "is the VM's own address"},
		{"gateway valid", func(h *Herdfile) { h.VMs[0].Network.Gateway = "192.168.150.254" }, ""},
		{"dns not an IP", func(h *Herdfile) { h.Network.DNS = "dns.example.com" }, `network.dns "dns.example.com" must be 'auto'`},
		{"dns IPv6", func(h *Herdfile) { h.Network.DNS = "2001:db8::53" }, ""},
		{"dns IPv4", func(h *Herdfile) { h.Network.DNS = "192.168.1.1" }, ""},
		{"share guest path with newline", func(h *Herdfile) {
			h.VMs[0].SharedFolders = []SharedFolder{{Host: ".", Guest: "/vagrant\nx"}}
		}, `guest path "/vagrant\nx" must be absolute (start with /) without control characters`},
		{"image key not distro:codename", func(h *Herdfile) { h.Images["noble"] = "https://example.org/noble.img" }, `images: key "noble" must be 'distro:codename'`},
		{"image URL over http", func(h *Herdfile) { h.Images["ubuntu:noble"] = "http://cloud-images.ubuntu.com/noble.img" }, `images: ubuntu:noble URL "http://cloud-images.ubuntu.com/noble.img" must be an https:// URL`},
		{"image URL without host", func(h *Herdfile) { h.Images["ubuntu:noble"] = "https:///noble.img" }, "must be an https:// URL"},
		{"VM image not defined", func(h *Herdfile) { h.VMs[0].Image = "debian:trixie" }, `VM "web": image "debian:trixie" is not defined under 'images:' — defined: ubuntu:noble`},
		{"no images section", func(h *Herdfile) { h.Images = nil }, "no 'images:' section"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectDir := t.TempDir()
			herdfile := validBaseHerdfile(t, projectDir)
			testCase.change(herdfile)
			err := validate(herdfile, projectDir)

			if testCase.wantProblems == "" {
				if err != nil {
					t.Fatalf("expected no problem, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected a problem mentioning %q, got none", testCase.wantProblems)
			}
			if !strings.Contains(err.Error(), testCase.wantProblems) {
				t.Errorf("problem does not mention %q:\n%v", testCase.wantProblems, err)
			}
			if strings.Contains(err.Error(), "problems:") {
				t.Errorf("expected exactly one problem, got several:\n%v", err)
			}
		})
	}
}

// TestValidateBaseHerdfileIsValid guards the table above: the base must pass.
func TestValidateBaseHerdfileIsValid(t *testing.T) {
	projectDir := t.TempDir()
	if err := validate(validBaseHerdfile(t, projectDir), projectDir); err != nil {
		t.Fatalf("base Herdfile is invalid: %v", err)
	}
}
