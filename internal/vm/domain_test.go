package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// goldenTestCases returns the domain XML test configurations. Each case has a
// name (matching a testdata/<name>.xml file) and a CreateOptions.
func goldenTestCases() []struct {
	name string
	opts CreateOptions
} {
	return []struct {
		name string
		opts CreateOptions
	}{
		{
			name: "minimal",
			opts: CreateOptions{
				VMName:      "ctrl",
				DomainName:  "vmherd-myproject-1a2b3c4d-ctrl",
				DiskPath:    "/var/lib/libvirt/images/vmherd-myproject-1a2b3c4d-ctrl.qcow2",
				NetworkName: "vmherd-myproject-1a2b3c4d",
				MACAddress:  "52:54:00:aa:bb:cc",
				Memory:      4096,
				CPUs:        2,
				ImagePath:   "/home/user/.vmherd/images/ubuntu-noble-amd64.qcow2",
				DiskSize:    "20G",
				SeedPath:    "/var/lib/libvirt/images/vmherd-myproject-1a2b3c4d-ctrl-seed.iso",
			},
		},
		{
			name: "shares-seclabel-console",
			opts: CreateOptions{
				VMName:         "dev",
				DomainName:     "vmherd-webapp-abcdef01-dev",
				DiskPath:       "/var/lib/libvirt/images/vmherd-webapp-abcdef01-dev.qcow2",
				NetworkName:    "vmherd-webapp-abcdef01",
				MACAddress:     "52:54:00:11:22:33",
				Memory:         2048,
				CPUs:           4,
				ImagePath:      "/home/user/.vmherd/images/debian-bookworm-amd64.qcow2",
				DiskSize:       "40G",
				SeedPath:       "/var/lib/libvirt/images/vmherd-webapp-abcdef01-dev-seed.iso",
				ConsoleLogPath: "/home/user/.local/state/vmherd/webapp-abcdef01/dev/console.log",
				SharedFolders: []SharedFolderMount{
					{HostPath: "/home/user/projects/webapp", GuestPath: "/vagrant", MountTag: "vmherd_share_0"},
				},
				QEMUProcessLabel: "+1000:+992",
			},
		},
		{
			name: "dhcp",
			opts: CreateOptions{
				VMName:      "worker",
				DomainName:  "vmherd-cluster-fedcba98-worker",
				DiskPath:    "/var/lib/libvirt/images/vmherd-cluster-fedcba98-worker.qcow2",
				NetworkName: "vmherd-cluster-fedcba98",
				MACAddress:  "52:54:00:dd:ee:ff",
				Memory:      1024,
				CPUs:        1,
				ImagePath:   "/home/user/.vmherd/images/fedora-41-amd64.qcow2",
				DiskSize:    "10G",
				SeedPath:    "/var/lib/libvirt/images/vmherd-cluster-fedcba98-worker-seed.iso",
			},
		},
		{
			name: "bridge",
			opts: CreateOptions{
				VMName:     "lan",
				DomainName: "vmherd-edge-87654321-lan",
				DiskPath:   "/var/lib/libvirt/images/vmherd-edge-87654321-lan.qcow2",
				BridgeName: "br1",
				MACAddress: "52:54:00:12:34:56",
				Memory:     2048,
				CPUs:       2,
				ImagePath:  "/home/user/.vmherd/images/ubuntu-noble-amd64.qcow2",
				DiskSize:   "20G",
				SeedPath:   "/var/lib/libvirt/images/vmherd-edge-87654321-lan-seed.iso",
			},
		},
		{
			name: "multi-share",
			opts: CreateOptions{
				VMName:      "build",
				DomainName:  "vmherd-mono-12345678-build",
				DiskPath:    "/var/lib/libvirt/images/vmherd-mono-12345678-build.qcow2",
				NetworkName: "vmherd-mono-12345678",
				MACAddress:  "52:54:00:99:88:77",
				Memory:      8192,
				CPUs:        8,
				ImagePath:   "/home/user/.vmherd/images/ubuntu-noble-amd64.qcow2",
				DiskSize:    "50G",
				SeedPath:    "/var/lib/libvirt/images/vmherd-mono-12345678-build-seed.iso",
				SharedFolders: []SharedFolderMount{
					{HostPath: "/home/user/projects/mono", GuestPath: "/vagrant", MountTag: "vmherd_share_0"},
					{HostPath: "/home/user/projects/mono/data", GuestPath: "/data", MountTag: "vmherd_share_1"},
					{HostPath: "/home/user/.config/tools", GuestPath: "/tools", MountTag: "vmherd_share_2"},
				},
				QEMUProcessLabel: "+1000:+992",
			},
		},
	}
}

// TestRenderDomainXML compares the output of RenderDomainXML against golden files
// in testdata/. Set VMHERD_UPDATE_GOLDEN=1 to regenerate the golden files.
func TestRenderDomainXML(t *testing.T) {
	updateGolden := os.Getenv("VMHERD_UPDATE_GOLDEN") == "1"

	for _, tc := range goldenTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderDomainXML(tc.opts)
			if err != nil {
				t.Fatalf("RenderDomainXML failed: %v", err)
			}

			goldenPath := filepath.Join("testdata", tc.name+".xml")

			if updateGolden {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("writing golden file: %v", err)
				}
				t.Logf("updated golden file %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden file %s: %v — run with VMHERD_UPDATE_GOLDEN=1 to generate", goldenPath, err)
			}

			if string(got) != string(want) {
				t.Errorf("RenderDomainXML output differs from %s\n\ngot:\n%s\n\nwant:\n%s", goldenPath, got, want)
			}
		})
	}
}

// TestRenderDomainXMLSharesRequireLabel verifies that shared folders without a
// QEMUProcessLabel produce an error.
func TestRenderDomainXMLSharesRequireLabel(t *testing.T) {
	opts := CreateOptions{
		DomainName:  "test-domain",
		DiskPath:    "/tmp/test.qcow2",
		NetworkName: "test-net",
		MACAddress:  "52:54:00:00:00:01",
		Memory:      512,
		CPUs:        1,
		SeedPath:    "/tmp/test-seed.iso",
		SharedFolders: []SharedFolderMount{
			{HostPath: "/tmp/share", GuestPath: "/mnt", MountTag: "share0"},
		},
	}

	_, err := RenderDomainXML(opts)
	if err == nil {
		t.Fatal("expected error for shared folders without QEMUProcessLabel")
	}
}
