package cloudinit

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// TestBuildISO9660RoundTrip builds an ISO with the three cloud-init files,
// then parses the resulting bytes to verify the volume label, filenames, and content.
func TestBuildISO9660RoundTrip(t *testing.T) {
	userData := []byte("#cloud-config\nhostname: test\n")
	metaData := []byte("instance-id: test-vm-1234\nlocal-hostname: test-vm\n")
	networkConfig := []byte("network:\n  version: 2\n  ethernets:\n    ens3:\n      dhcp4: true\n")

	files := []isoFile{
		{name: "meta-data", content: metaData},
		{name: "network-config", content: networkConfig},
		{name: "user-data", content: userData},
	}

	isoBytes, err := buildISO9660("cidata", files)
	if err != nil {
		t.Fatalf("buildISO9660 failed: %v", err)
	}

	if len(isoBytes)%isoSectorSize != 0 {
		t.Fatalf("ISO size %d is not a multiple of sector size %d", len(isoBytes), isoSectorSize)
	}

	label := parseVolumeLabel(t, isoBytes)
	if label != "CIDATA" {
		t.Errorf("volume label = %q, want %q", label, "CIDATA")
	}

	foundFiles := parseRootDirectory(t, isoBytes)
	wantFiles := map[string][]byte{
		"meta-data":      metaData,
		"network-config": networkConfig,
		"user-data":      userData,
	}

	if len(foundFiles) != len(wantFiles) {
		t.Fatalf("found %d files, want %d", len(foundFiles), len(wantFiles))
	}

	for name, wantContent := range wantFiles {
		gotContent, ok := foundFiles[name]
		if !ok {
			t.Errorf("file %q not found in ISO", name)
			continue
		}
		if !bytes.Equal(gotContent, wantContent) {
			t.Errorf("file %q content mismatch:\n  got:  %q\n  want: %q", name, gotContent, wantContent)
		}
	}
}

// TestBuildISO9660EmptyFilesReturnsError verifies that an empty file list is rejected.
func TestBuildISO9660EmptyFilesReturnsError(t *testing.T) {
	_, err := buildISO9660("cidata", nil)
	if err == nil {
		t.Fatal("expected error for empty file list, got nil")
	}
}

// TestBuildISO9660LargeFile verifies that a file spanning multiple sectors is handled.
func TestBuildISO9660LargeFile(t *testing.T) {
	largeContent := bytes.Repeat([]byte("x"), isoSectorSize*3+500)
	files := []isoFile{
		{name: "user-data", content: largeContent},
		{name: "meta-data", content: []byte("instance-id: big\n")},
		{name: "network-config", content: []byte("network: {}\n")},
	}

	isoBytes, err := buildISO9660("cidata", files)
	if err != nil {
		t.Fatalf("buildISO9660 failed: %v", err)
	}

	foundFiles := parseRootDirectory(t, isoBytes)
	if got := foundFiles["user-data"]; !bytes.Equal(got, largeContent) {
		t.Errorf("large file content length = %d, want %d", len(got), len(largeContent))
	}
}

// TestToISO9660Name verifies the Level 1 name transformation.
func TestToISO9660Name(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user-data", "USER_DAT;1"},
		{"meta-data", "META_DAT;1"},
		{"network-config", "NETWORK_;1"},
		{"simple", "SIMPLE;1"},
		{"ALREADY", "ALREADY;1"},
	}
	for _, tt := range tests {
		got := toISO9660Name(tt.input)
		if got != tt.want {
			t.Errorf("toISO9660Name(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// parseVolumeLabel reads the volume identifier from the Primary Volume Descriptor.
func parseVolumeLabel(t *testing.T, isoBytes []byte) string {
	t.Helper()
	pvdOffset := isoSystemAreaSectors * isoSectorSize
	if len(isoBytes) < pvdOffset+isoSectorSize {
		t.Fatal("ISO too small to contain a PVD")
	}
	pvd := isoBytes[pvdOffset : pvdOffset+isoSectorSize]
	if pvd[0] != isoPrimaryVolumeDescriptorType {
		t.Fatalf("expected PVD type %d at sector %d, got %d", isoPrimaryVolumeDescriptorType, isoSystemAreaSectors, pvd[0])
	}
	if string(pvd[1:6]) != "CD001" {
		t.Fatalf("PVD standard identifier = %q, want %q", pvd[1:6], "CD001")
	}
	return strings.TrimRight(string(pvd[40:72]), " ")
}

// parseRootDirectory reads the root directory records and extracts file names
// (via Rock Ridge NM entries) and file content from the ISO.
func parseRootDirectory(t *testing.T, isoBytes []byte) map[string][]byte {
	t.Helper()

	pvdOffset := isoSystemAreaSectors * isoSectorSize
	pvd := isoBytes[pvdOffset : pvdOffset+isoSectorSize]

	rootRecord := pvd[156:190]
	rootSector := binary.LittleEndian.Uint32(rootRecord[2:6])
	rootSize := binary.LittleEndian.Uint32(rootRecord[10:14])

	dirStart := int(rootSector) * isoSectorSize
	dirEnd := dirStart + int(rootSize)
	if dirEnd > len(isoBytes) {
		t.Fatalf("root directory extends past end of ISO (sector %d, size %d)", rootSector, rootSize)
	}
	dirData := isoBytes[dirStart:dirEnd]

	files := make(map[string][]byte)
	offset := 0
	for offset < len(dirData) {
		recordLen := int(dirData[offset])
		if recordLen == 0 {
			break
		}

		idLen := int(dirData[offset+32])
		if idLen == 1 && (dirData[offset+33] == 0x00 || dirData[offset+33] == 0x01) {
			offset += recordLen
			continue
		}

		fileSector := binary.LittleEndian.Uint32(dirData[offset+2 : offset+6])
		fileSize := binary.LittleEndian.Uint32(dirData[offset+10 : offset+14])

		baseLen := 33 + idLen
		if idLen%2 == 0 {
			baseLen++
		}

		rockRidgeName := parseRockRidgeNM(dirData[offset+baseLen : offset+recordLen])

		dataStart := int(fileSector) * isoSectorSize
		dataEnd := dataStart + int(fileSize)
		if dataEnd > len(isoBytes) {
			t.Fatalf("file data extends past end of ISO for Rock Ridge name %q", rockRidgeName)
		}

		if rockRidgeName != "" {
			files[rockRidgeName] = isoBytes[dataStart:dataEnd]
		} else {
			iso9660Name := string(dirData[offset+33 : offset+33+idLen])
			files[iso9660Name] = isoBytes[dataStart:dataEnd]
		}

		offset += recordLen
	}

	return files
}

// parseRockRidgeNM scans system use entries for an NM entry and returns the alternate name.
func parseRockRidgeNM(systemUse []byte) string {
	offset := 0
	for offset+4 <= len(systemUse) {
		sig := [2]byte{systemUse[offset], systemUse[offset+1]}
		entryLen := int(systemUse[offset+2])
		if entryLen < 3 || offset+entryLen > len(systemUse) {
			break
		}
		if sig == rockRidgeSignatureNM && entryLen > 5 {
			return string(systemUse[offset+5 : offset+entryLen])
		}
		offset += entryLen
	}
	return ""
}
