// Package portfwd forwards host ports to VMs through one background SSH tunnel
// per VM ('ssh -N -L …'), like vagrant-libvirt. It needs no firewall rules, so
// it can't clash with Docker or libvirt rules, and it can listen on localhost.
package portfwd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

const (
	// tunnelRecordFile holds the running tunnel's PID and command, in the VM's state directory.
	tunnelRecordFile = "port-forward.json"
	// tunnelLogFile receives the tunnel's ssh output, in the VM's state directory.
	tunnelLogFile = "port-forward.log"
	// tunnelReadyTimeout is how long Start waits for every forwarded port to accept connections.
	tunnelReadyTimeout = 15 * time.Second
	// tunnelStopTimeout is how long Stop waits for a tunnel to exit after SIGTERM.
	tunnelStopTimeout = 5 * time.Second
	// tunnelPollInterval is the wait between readiness and exit checks.
	tunnelPollInterval = 200 * time.Millisecond
	// portCheckDialTimeout bounds each connection attempt when checking a host port.
	portCheckDialTimeout = 500 * time.Millisecond
	// unprivilegedPortStartPath holds the lowest port a normal user may bind.
	unprivilegedPortStartPath = "/proc/sys/net/ipv4/ip_unprivileged_port_start"
	// defaultUnprivilegedPortStart is the Linux default when that file can't be read.
	defaultUnprivilegedPortStart = 1024
	// recordFilePermissions keeps the tunnel record private to the user.
	recordFilePermissions = 0o600
)

// Tunnel describes the SSH tunnel that carries one VM's forwarded ports.
type Tunnel struct {
	VMName     string             // Herdfile VM name, for messages
	VMStateDir string             // per-VM state directory holding the record and log
	KeyPath    string             // project SSH private key
	User       string             // SSH user on the VM
	GuestIP    string             // VM address to connect to
	Forwards   []ssh.LocalForward // ports to forward; empty stops any running tunnel
}

// tunnelRecord is the JSON state of a started tunnel.
type tunnelRecord struct {
	PID        int      `json:"pid"`        // process ID of ssh, or of sudo for privileged tunnels
	Command    []string `json:"command"`    // full argv, compared with /proc/<pid>/cmdline
	Privileged bool     `json:"privileged"` // started via sudo, so it must be stopped via sudo
}

// Start makes sure the VM's tunnel runs with exactly tunnel.Forwards: an
// identical running tunnel is kept, a different one is replaced, and a dead
// one is restarted. It returns once every port accepts connections.
func Start(tunnel Tunnel) error {
	if len(tunnel.Forwards) == 0 {
		return Stop(tunnel.VMStateDir)
	}
	command, privileged, err := tunnelCommand(tunnel)
	if err != nil {
		return err
	}

	record, running, err := readRunningRecord(tunnel.VMStateDir)
	if err != nil {
		return err
	}
	if running && slices.Equal(record.Command, command) {
		ui.Printf("%s: port forwarding already running (%s)\n", tunnel.VMName, describeForwards(tunnel.Forwards))
		return nil
	}
	// Check sudo and ports before stopping the old tunnel, so a failure leaves it running.
	if privileged {
		if err := ensureSudoWithoutPrompt(); err != nil {
			return fmt.Errorf("VM %s: %w", tunnel.VMName, err)
		}
	}
	if err := checkHostPortsFree(forwardsNotHeldBy(tunnel.Forwards, record, running)); err != nil {
		return fmt.Errorf("VM %s: %w", tunnel.VMName, err)
	}
	if err := Stop(tunnel.VMStateDir); err != nil {
		return err
	}

	if err := launchTunnel(tunnel, command, privileged); err != nil {
		return fmt.Errorf("VM %s: %w", tunnel.VMName, err)
	}
	ui.Printf("%s: forwarding %s\n", tunnel.VMName, describeForwards(tunnel.Forwards))
	return nil
}

// tunnelCommand returns the argv for the tunnel. Host ports below the
// unprivileged port start need root to bind, so then ssh runs via 'sudo -n'
// (ensureSudoWithoutPrompt asks for a password first if needed).
func tunnelCommand(tunnel Tunnel) ([]string, bool, error) {
	command := append([]string{"ssh"}, ssh.TunnelArgs(tunnel.KeyPath, tunnel.User, tunnel.GuestIP, tunnel.Forwards)...)
	lowestUnprivilegedPort := unprivilegedPortStart()
	for _, forward := range tunnel.Forwards {
		if forward.HostPort < lowestUnprivilegedPort {
			return append([]string{"sudo", "-n"}, command...), true, nil
		}
	}
	return command, false, nil
}

// launchTunnel starts the tunnel detached from the terminal, records it, and
// waits until all ports accept connections. A tunnel that exits or never gets
// ready is reported with its ssh output and not left behind.
func launchTunnel(tunnel Tunnel, command []string, privileged bool) error {
	logPath := filepath.Join(tunnel.VMStateDir, tunnelLogFile)
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("creating tunnel log %s: %w", logPath, err)
	}
	defer closeLogFile(logFile)

	tunnelProcess := exec.Command(command[0], command[1:]...)
	tunnelProcess.Stdout = logFile
	tunnelProcess.Stderr = logFile
	tunnelProcess.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := tunnelProcess.Start(); err != nil {
		return fmt.Errorf("starting ssh tunnel: %w — check that ssh is installed", err)
	}

	record := tunnelRecord{PID: tunnelProcess.Process.Pid, Command: command, Privileged: privileged}
	if err := writeRecord(tunnel.VMStateDir, record); err != nil {
		return errors.Join(err, stopProcess(record))
	}

	exited := make(chan error, 1)
	go func() { exited <- tunnelProcess.Wait() }()
	if err := waitUntilListening(tunnel.Forwards, exited); err != nil {
		stopErr := stopProcess(record)
		removeErr := removeRecord(tunnel.VMStateDir)
		return errors.Join(fmt.Errorf("%w%s", err, logExcerpt(logPath)), stopErr, removeErr)
	}
	return nil
}

// closeLogFile closes the parent's handle on the tunnel log; the tunnel keeps its own.
func closeLogFile(logFile *os.File) {
	if err := logFile.Close(); err != nil {
		ui.Printf("Warning: closing %s: %v\n", logFile.Name(), err)
	}
}

// waitUntilListening polls until every forward accepts connections, the
// tunnel exits, or tunnelReadyTimeout passes.
func waitUntilListening(forwards []ssh.LocalForward, exited <-chan error) error {
	deadline := time.Now().Add(tunnelReadyTimeout)
	for time.Now().Before(deadline) {
		select {
		case exitErr := <-exited:
			return fmt.Errorf("ssh tunnel exited (%v) — is the VM running and reachable over SSH?", exitErr)
		case <-time.After(tunnelPollInterval):
		}
		if allListening(forwards) {
			return nil
		}
	}
	return fmt.Errorf("forwarded ports did not open within %s", tunnelReadyTimeout)
}

// forwardsNotHeldBy returns the forwards whose host port the running tunnel
// doesn't already listen on; those it holds are busy only because of it.
func forwardsNotHeldBy(forwards []ssh.LocalForward, record tunnelRecord, running bool) []ssh.LocalForward {
	if !running {
		return forwards
	}
	var notHeld []ssh.LocalForward
	for _, forward := range forwards {
		listenPrefix := forward.ListenSpec() + ":"
		held := slices.ContainsFunc(record.Command, func(argument string) bool { return strings.HasPrefix(argument, listenPrefix) })
		if !held {
			notHeld = append(notHeld, forward)
		}
	}
	return notHeld
}

// allListening reports whether every forward's host port accepts connections.
func allListening(forwards []ssh.LocalForward) bool {
	for _, forward := range forwards {
		if !portAcceptsConnections(forward) {
			return false
		}
	}
	return true
}

// checkHostPortsFree fails if another program already listens on a host port,
// which ssh would otherwise only report as a generic forwarding failure.
func checkHostPortsFree(forwards []ssh.LocalForward) error {
	for _, forward := range forwards {
		if portAcceptsConnections(forward) {
			return fmt.Errorf("host port %s is already in use by another program — choose another 'host' port in the Herdfile or stop that program",
				net.JoinHostPort(forward.HostIP, strconv.Itoa(forward.HostPort)))
		}
	}
	return nil
}

// portAcceptsConnections dials the forward's host port. A wildcard bind
// address (0.0.0.0, ::) is checked through the matching loopback address.
func portAcceptsConnections(forward ssh.LocalForward) bool {
	address := net.JoinHostPort(dialHostFor(forward.HostIP), strconv.Itoa(forward.HostPort))
	connection, err := net.DialTimeout("tcp", address, portCheckDialTimeout)
	if err != nil {
		return false
	}
	if closeErr := connection.Close(); closeErr != nil {
		ui.Printf("Warning: closing port check connection to %s: %v\n", address, closeErr)
	}
	return true
}

// dialHostFor returns the address to dial to reach a port bound on hostIP:
// the loopback address of the same family for 0.0.0.0 and ::, else hostIP.
func dialHostFor(hostIP string) string {
	parsedIP := net.ParseIP(hostIP)
	if parsedIP == nil || !parsedIP.IsUnspecified() {
		return hostIP
	}
	if parsedIP.To4() != nil {
		return "127.0.0.1"
	}
	return "::1"
}

// Stop ends the VM's tunnel if one is running and removes its record. A
// recorded PID whose command line no longer matches belongs to another
// process and is left alone.
func Stop(vmStateDir string) error {
	record, running, err := readRunningRecord(vmStateDir)
	if err != nil {
		return err
	}
	if running {
		if err := stopProcess(record); err != nil {
			return err
		}
	}
	return removeRecord(vmStateDir)
}

// stopProcess sends SIGTERM to the tunnel (via sudo for privileged ones) and
// waits for it to exit.
func stopProcess(record tunnelRecord) error {
	if record.Privileged {
		if output, err := exec.Command("sudo", "kill", "-TERM", strconv.Itoa(record.PID)).CombinedOutput(); err != nil {
			return fmt.Errorf("stopping port-forward tunnel (pid %d) with sudo: %w: %s", record.PID, err, strings.TrimSpace(string(output)))
		}
	} else if err := syscall.Kill(record.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("stopping port-forward tunnel (pid %d): %w", record.PID, err)
	}

	deadline := time.Now().Add(tunnelStopTimeout)
	for time.Now().Before(deadline) {
		if !processRunsCommand(record.PID, record.Command) {
			return nil
		}
		time.Sleep(tunnelPollInterval)
	}
	return fmt.Errorf("port-forward tunnel (pid %d) did not exit within %s — stop it with 'kill %d'", record.PID, tunnelStopTimeout, record.PID)
}

// readRunningRecord loads the VM's tunnel record and reports whether its
// process is still the recorded tunnel. A missing record is not an error.
func readRunningRecord(vmStateDir string) (tunnelRecord, bool, error) {
	recordPath := filepath.Join(vmStateDir, tunnelRecordFile)
	content, err := os.ReadFile(recordPath)
	if errors.Is(err, os.ErrNotExist) {
		return tunnelRecord{}, false, nil
	}
	if err != nil {
		return tunnelRecord{}, false, fmt.Errorf("reading %s: %w", recordPath, err)
	}
	var record tunnelRecord
	if err := json.Unmarshal(content, &record); err != nil {
		return tunnelRecord{}, false, fmt.Errorf("parsing %s: %w — delete it and stop any leftover 'ssh -N' tunnel for this VM", recordPath, err)
	}
	return record, processRunsCommand(record.PID, record.Command), nil
}

// processRunsCommand reports whether pid is alive and runs exactly command,
// read from /proc/<pid>/cmdline, so a reused PID is never mistaken for ours.
func processRunsCommand(pid int, command []string) bool {
	if pid <= 0 {
		return false
	}
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	return slices.Equal(strings.Split(strings.TrimSuffix(string(cmdline), "\x00"), "\x00"), command)
}

// writeRecord saves the tunnel record as JSON in the VM's state directory.
func writeRecord(vmStateDir string, record tunnelRecord) error {
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding tunnel record: %w", err)
	}
	recordPath := filepath.Join(vmStateDir, tunnelRecordFile)
	if err := os.WriteFile(recordPath, content, recordFilePermissions); err != nil {
		return fmt.Errorf("writing %s: %w", recordPath, err)
	}
	return nil
}

// removeRecord deletes the VM's tunnel record; a missing record is fine.
func removeRecord(vmStateDir string) error {
	recordPath := filepath.Join(vmStateDir, tunnelRecordFile)
	if err := os.Remove(recordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", recordPath, err)
	}
	return nil
}

// ensureSudoWithoutPrompt makes sure the background 'sudo -n ssh' for a
// privileged port won't need a password: if 'sudo -n true' fails, it runs
// 'sudo true' in the foreground so the user can enter it once.
func ensureSudoWithoutPrompt() error {
	if exec.Command("sudo", "-n", "true").Run() == nil {
		return nil
	}
	sudoPrompt := exec.Command("sudo", "true")
	sudoPrompt.Stdin = os.Stdin
	sudoPrompt.Stdout = os.Stdout
	sudoPrompt.Stderr = os.Stderr
	if err := sudoPrompt.Run(); err != nil {
		return fmt.Errorf("host ports below %d need sudo to bind, and sudo failed: %w — use a host port of %d or higher, or check your sudo access",
			unprivilegedPortStart(), err, unprivilegedPortStart())
	}
	return nil
}

// unprivilegedPortStart returns the lowest port a normal user may bind on
// this host (net.ipv4.ip_unprivileged_port_start), or the Linux default.
func unprivilegedPortStart() int {
	content, err := os.ReadFile(unprivilegedPortStartPath)
	if err != nil {
		return defaultUnprivilegedPortStart
	}
	portStart, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return defaultUnprivilegedPortStart
	}
	return portStart
}

// describeForwards renders forwards as "127.0.0.1:8080 → 80, …" for messages.
func describeForwards(forwards []ssh.LocalForward) string {
	descriptions := make([]string, len(forwards))
	for index, forward := range forwards {
		descriptions[index] = fmt.Sprintf("%s → %d", net.JoinHostPort(forward.HostIP, strconv.Itoa(forward.HostPort)), forward.GuestPort)
	}
	return strings.Join(descriptions, ", ")
}

// logExcerpt returns the tunnel log's content for an error message, or "".
func logExcerpt(logPath string) string {
	content, err := os.ReadFile(logPath)
	if err != nil || len(strings.TrimSpace(string(content))) == 0 {
		return ""
	}
	return ": " + strings.TrimSpace(string(content))
}
