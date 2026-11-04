package vm

import (
	"fmt"
	"os"
	"syscall"
)

// kvmDevicePath is the KVM device QEMU must open; its group is used for the QEMU process.
const kvmDevicePath = "/dev/kvm"

// minRegularUserUID is the lowest UID treated as a regular (non-system) account,
// so it can be reused for the guest SSH user without clashing with system users.
const minRegularUserUID = 1000

// maxRegularUserUID is the highest UID reused for the guest SSH user; higher IDs
// are typically reserved (e.g. nobody = 65534) or used by container ranges.
const maxRegularUserUID = 59999

// HostIdentity is who QEMU runs as for VMs with shared folders: the invoking
// host user plus the group that owns /dev/kvm.
type HostIdentity struct {
	UserID     int // UID of the user running vmherd
	KVMGroupID int // GID owning /dev/kvm, so QEMU can open it
}

// DetectHostIdentity returns the invoking user and the group of /dev/kvm.
// It refuses to run as root, because QEMU would then run as root and guest
// root could create root-owned files in shared folders.
func DetectHostIdentity() (HostIdentity, error) {
	return detectHostIdentity(os.Getuid(), kvmDevicePath)
}

// detectHostIdentity is DetectHostIdentity with the UID and device path injected for tests.
func detectHostIdentity(userID int, kvmPath string) (HostIdentity, error) {
	if userID == 0 {
		return HostIdentity{}, fmt.Errorf("vmherd is running as root: run it as your normal user " +
			"(it calls sudo itself), so shared folders are not served by a root QEMU process")
	}
	kvmInfo, err := os.Stat(kvmPath)
	if err != nil {
		return HostIdentity{}, fmt.Errorf("cannot access %s: %w — KVM is required; check that virtualization is enabled "+
			"and the kvm module is loaded ('lsmod | grep kvm')", kvmPath, err)
	}
	kvmStat, ok := kvmInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return HostIdentity{}, fmt.Errorf("cannot read the owning group of %s on this platform", kvmPath)
	}
	return HostIdentity{UserID: userID, KVMGroupID: int(kvmStat.Gid)}, nil
}

// QEMUProcessLabel returns the libvirt DAC label ("+UID:+GID") that makes QEMU
// run as the host user with the KVM group.
func (hostIdentity HostIdentity) QEMUProcessLabel() string {
	return fmt.Sprintf("+%d:+%d", hostIdentity.UserID, hostIdentity.KVMGroupID)
}

// GuestUserUID returns the UID the guest SSH user should get so files in shared
// folders have the same owner on both sides. ok is false for UIDs outside the
// regular-user range, where reusing the UID could clash with guest system users.
func (hostIdentity HostIdentity) GuestUserUID() (guestUID int, ok bool) {
	if hostIdentity.UserID < minRegularUserUID || hostIdentity.UserID > maxRegularUserUID {
		return 0, false
	}
	return hostIdentity.UserID, true
}
