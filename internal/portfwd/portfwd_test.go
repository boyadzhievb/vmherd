package portfwd

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/boyadzhievb/vmherd/internal/ssh"
)

func TestDialHostFor(t *testing.T) {
	testCases := map[string]string{
		"127.0.0.1":      "127.0.0.1",
		"0.0.0.0":        "127.0.0.1",
		"::":             "::1",
		"::1":            "::1",
		"192.168.100.49": "192.168.100.49",
	}
	for hostIP, wantDialHost := range testCases {
		if gotDialHost := dialHostFor(hostIP); gotDialHost != wantDialHost {
			t.Errorf("dialHostFor(%q) = %q, want %q", hostIP, gotDialHost, wantDialHost)
		}
	}
}

// listenOnFreePort opens a TCP listener on 127.0.0.1 and returns its port.
func listenOnFreePort(t *testing.T) (net.Listener, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Logf("closing listener: %v", closeErr)
		}
	})
	return listener, listener.Addr().(*net.TCPAddr).Port
}

func TestCheckHostPortsFree(t *testing.T) {
	_, busyPort := listenOnFreePort(t)
	err := checkHostPortsFree([]ssh.LocalForward{{HostIP: "0.0.0.0", HostPort: busyPort, GuestPort: 80}})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Errorf("expected an in-use error for port %d, got %v", busyPort, err)
	}

	freeListener, freePort := listenOnFreePort(t)
	if err := freeListener.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}
	if err := checkHostPortsFree([]ssh.LocalForward{{HostIP: "127.0.0.1", HostPort: freePort, GuestPort: 80}}); err != nil {
		t.Errorf("expected port %d to be free, got %v", freePort, err)
	}
}

func TestTunnelCommandPrivilegedPorts(t *testing.T) {
	tunnel := Tunnel{KeyPath: "/k", User: "vmherd", GuestIP: "192.168.150.10"}

	tunnel.Forwards = []ssh.LocalForward{{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 80}}
	command, privileged, err := tunnelCommand(tunnel)
	if err != nil || privileged || command[0] != "ssh" {
		t.Errorf("port 8080: got %v privileged=%v err=%v, want plain ssh", command, privileged, err)
	}

	tunnel.Forwards = append(tunnel.Forwards, ssh.LocalForward{HostIP: "127.0.0.1", HostPort: 80, GuestPort: 80})
	if unprivilegedPortStart() <= 80 {
		t.Skip("this host lets normal users bind port 80")
	}
	command, privileged, err = tunnelCommand(tunnel)
	if err != nil || !privileged || command[0] != "sudo" || command[1] != "-n" || command[2] != "ssh" {
		t.Errorf("port 80: got %v privileged=%v err=%v, want sudo -n ssh", command, privileged, err)
	}
}

func TestRecordRoundTripAndStaleRecord(t *testing.T) {
	vmStateDir := t.TempDir()
	if _, running, err := readRunningRecord(vmStateDir); err != nil || running {
		t.Fatalf("missing record: running=%v err=%v, want not running and no error", running, err)
	}

	stale := tunnelRecord{PID: 999999999, Command: []string{"ssh", "-N"}}
	if err := writeRecord(vmStateDir, stale); err != nil {
		t.Fatalf("writing record: %v", err)
	}
	record, running, err := readRunningRecord(vmStateDir)
	if err != nil || running || record.PID != stale.PID {
		t.Errorf("stale record: %+v running=%v err=%v, want the record, not running", record, running, err)
	}
	if err := Stop(vmStateDir); err != nil {
		t.Fatalf("stopping with a stale record: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vmStateDir, tunnelRecordFile)); !os.IsNotExist(err) {
		t.Errorf("Stop should remove a stale record, stat err = %v", err)
	}
}

func TestCorruptRecordIsActionable(t *testing.T) {
	vmStateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(vmStateDir, tunnelRecordFile), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("writing corrupt record: %v", err)
	}
	if _, _, err := readRunningRecord(vmStateDir); err == nil || !strings.Contains(err.Error(), "delete it") {
		t.Errorf("expected an actionable error, got %v", err)
	}
}

// TestProcessRunsCommandAndStop starts a real process, checks it is matched
// by its exact command line (not by a different one), and stops it.
func TestProcessRunsCommandAndStop(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc")
	}
	command := []string{"sleep", "30"}
	sleepProcess := exec.Command(command[0], command[1:]...)
	if err := sleepProcess.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- sleepProcess.Wait() }()

	// Until the child has exec'd, /proc/<pid>/cmdline still shows the test binary.
	deadline := time.Now().Add(tunnelStopTimeout)
	for !processRunsCommand(sleepProcess.Process.Pid, command) {
		if time.Now().After(deadline) {
			t.Fatal("expected the running sleep to match its command line")
		}
		time.Sleep(tunnelPollInterval)
	}
	if processRunsCommand(sleepProcess.Process.Pid, []string{"sleep", "31"}) {
		t.Error("a different command line must not match")
	}

	vmStateDir := t.TempDir()
	if err := writeRecord(vmStateDir, tunnelRecord{PID: sleepProcess.Process.Pid, Command: command}); err != nil {
		t.Fatalf("writing record: %v", err)
	}
	if err := Stop(vmStateDir); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if exitErr := <-exited; exitErr == nil || !strings.Contains(exitErr.Error(), "terminated") {
		t.Errorf("expected sleep to be terminated, got %v", exitErr)
	}
}

func TestDescribeForwards(t *testing.T) {
	got := describeForwards([]ssh.LocalForward{
		{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 80},
		{HostIP: "::1", HostPort: 8443, GuestPort: 443},
	})
	want := "127.0.0.1:8080 → 80, [::1]:8443 → 443"
	if got != want {
		t.Errorf("describeForwards() = %q, want %q", got, want)
	}
}

func TestUnprivilegedPortStartDefault(t *testing.T) {
	if _, err := os.Stat(unprivilegedPortStartPath); err == nil {
		t.Skip("host has " + unprivilegedPortStartPath)
	}
	if got := unprivilegedPortStart(); got != defaultUnprivilegedPortStart {
		t.Errorf("unprivilegedPortStart() = %d, want %s", got, strconv.Itoa(defaultUnprivilegedPortStart))
	}
}

func TestForwardsNotHeldBy(t *testing.T) {
	heldForward := ssh.LocalForward{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 80}
	newForward := ssh.LocalForward{HostIP: "127.0.0.1", HostPort: 8081, GuestPort: 80}
	samePortOtherAddress := ssh.LocalForward{HostIP: "0.0.0.0", HostPort: 8080, GuestPort: 80}
	runningRecord := tunnelRecord{Command: append([]string{"ssh"}, ssh.TunnelArgs("/k", "vmherd", "192.168.150.10", []ssh.LocalForward{heldForward})...)}
	forwards := []ssh.LocalForward{heldForward, newForward, samePortOtherAddress}

	gotNotHeld := forwardsNotHeldBy(forwards, runningRecord, true)
	wantNotHeld := []ssh.LocalForward{newForward, samePortOtherAddress}
	if !slices.Equal(gotNotHeld, wantNotHeld) {
		t.Errorf("forwardsNotHeldBy() = %v, want %v", gotNotHeld, wantNotHeld)
	}
	if gotAll := forwardsNotHeldBy(forwards, runningRecord, false); !slices.Equal(gotAll, forwards) {
		t.Errorf("without a running tunnel every forward must be checked, got %v", gotAll)
	}
}

func TestMatchingLegacyRules(t *testing.T) {
	ruleListing := `-P PREROUTING ACCEPT
-A PREROUTING -j CCA_SERVICES
-A PREROUTING -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22
-A PREROUTING -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.99:22
-A PREROUTING -p tcp -m tcp --dport 5000 -j DNAT --to-destination 172.17.0.2:5000
-A OUTPUT -d 127.0.0.1/32 -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22
-A OUTPUT -d 127.0.0.1/32 -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22
-A DOCKER ! -i docker0 -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22
`
	got := matchingLegacyRules(ruleListing, "192.168.151.10", []ssh.LocalForward{{HostIP: "127.0.0.1", HostPort: 18090, GuestPort: 22}})
	want := []string{
		"-A PREROUTING -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22",
		"-A OUTPUT -d 127.0.0.1/32 -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22",
		"-A OUTPUT -d 127.0.0.1/32 -p tcp -m tcp --dport 18090 -j DNAT --to-destination 192.168.151.10:22",
	}
	if !slices.Equal(got, want) {
		t.Errorf("matchingLegacyRules() =\n%v\nwant (only exact v0.1.x rules for this VM, duplicates kept)\n%v", got, want)
	}
}
