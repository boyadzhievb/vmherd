package vm

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func TestDetectHostIdentityRefusesRoot(t *testing.T) {
	_, err := detectHostIdentity(0, os.DevNull)
	if err == nil || !strings.Contains(err.Error(), "running as root") {
		t.Fatalf("expected root refusal, got %v", err)
	}
}

func TestDetectHostIdentityMissingKVM(t *testing.T) {
	_, err := detectHostIdentity(1001, filepath.Join(t.TempDir(), "kvm"))
	if err == nil || !strings.Contains(err.Error(), "KVM is required") {
		t.Fatalf("expected missing-KVM error, got %v", err)
	}
}

func TestDetectHostIdentityReadsDeviceGroup(t *testing.T) {
	fakeDevicePath := filepath.Join(t.TempDir(), "kvm")
	if err := os.WriteFile(fakeDevicePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fakeDeviceInfo, err := os.Stat(fakeDevicePath)
	if err != nil {
		t.Fatal(err)
	}
	wantGroupID := int(fakeDeviceInfo.Sys().(*syscall.Stat_t).Gid)

	hostIdentity, err := detectHostIdentity(1001, fakeDevicePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hostIdentity.UserID != 1001 || hostIdentity.KVMGroupID != wantGroupID {
		t.Errorf("got %+v, want UserID 1001 and KVMGroupID %d", hostIdentity, wantGroupID)
	}
	if got, want := hostIdentity.QEMUProcessLabel(), fmt.Sprintf("+1001:+%d", wantGroupID); got != want {
		t.Errorf("QEMUProcessLabel() = %q, want %q", got, want)
	}
}

func TestGuestUserUID(t *testing.T) {
	testCases := []struct {
		hostUserID int  // UID of the host user
		wantUID    int  // expected guest UID
		wantOK     bool // whether the host UID can be reused
	}{
		{hostUserID: 1000, wantUID: 1000, wantOK: true},
		{hostUserID: 1001, wantUID: 1001, wantOK: true},
		{hostUserID: 59999, wantUID: 59999, wantOK: true},
		{hostUserID: 999, wantOK: false},
		{hostUserID: 60000, wantOK: false},
		{hostUserID: 65534, wantOK: false},
	}
	for _, testCase := range testCases {
		gotUID, gotOK := HostIdentity{UserID: testCase.hostUserID, KVMGroupID: 106}.GuestUserUID()
		if gotUID != testCase.wantUID || gotOK != testCase.wantOK {
			t.Errorf("GuestUserUID() for host UID %d = (%d, %v), want (%d, %v)",
				testCase.hostUserID, gotUID, gotOK, testCase.wantUID, testCase.wantOK)
		}
	}
}

func TestSharedFolderArgs(t *testing.T) {
	if args, err := sharedFolderArgs(nil, "+1001:+106"); args != nil || err != nil {
		t.Errorf("expected no args and no error without shared folders, got %v, %v", args, err)
	}
	sharedFolders := []SharedFolderMount{
		{HostPath: "/home/bob/proj", GuestPath: "/vagrant", MountTag: "vmherd_share_0"},
		{HostPath: "/home/bob/data", GuestPath: "/data", MountTag: "vmherd_share_1"},
	}
	wantArgs := []string{
		"--seclabel", "type=static,model=dac,label=+1001:+106,relabel=yes",
		"--filesystem", `source.dir="/home/bob/proj",target.dir=vmherd_share_0,accessmode=squash`,
		"--filesystem", `source.dir="/home/bob/data",target.dir=vmherd_share_1,accessmode=squash`,
	}
	gotArgs, err := sharedFolderArgs(sharedFolders, "+1001:+106")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(gotArgs, wantArgs) {
		t.Errorf("sharedFolderArgs() =\n  %v\nwant\n  %v", gotArgs, wantArgs)
	}
}

func TestSharedFolderArgsRefusesQuote(t *testing.T) {
	sharedFolders := []SharedFolderMount{{HostPath: `/home/bob/a"b`, GuestPath: "/vagrant", MountTag: "vmherd_share_0"}}
	if _, err := sharedFolderArgs(sharedFolders, "+1001:+106"); err == nil || !strings.Contains(err.Error(), "/vagrant") {
		t.Errorf("expected an error naming the share, got %v", err)
	}
}

func TestCreateRequiresLabelForSharedFolders(t *testing.T) {
	err := Create(CreateOptions{
		VMName:        "web",
		SharedFolders: []SharedFolderMount{{HostPath: "/home/bob/proj", GuestPath: "/vagrant", MountTag: "vmherd_share_0"}},
	})
	if err == nil || !strings.Contains(err.Error(), "QEMUProcessLabel") {
		t.Fatalf("expected missing-label error, got %v", err)
	}
}
