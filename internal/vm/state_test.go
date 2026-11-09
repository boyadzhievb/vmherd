package vm

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestParseDomainStateKnownStates(t *testing.T) {
	knownStates := []VMState{
		StateRunning, StateIdle, StatePaused, StateShuttingDown, StateStopped,
		StateCrashed, StateDying, StatePMSuspended,
	}
	for _, wantState := range knownStates {
		// virsh domstate ends its output with a blank line.
		gotState, err := parseDomainState("vmherd-test-web", string(wantState)+"\n\n")
		if err != nil {
			t.Errorf("parseDomainState(%q) returned error: %v", wantState, err)
			continue
		}
		if gotState != wantState {
			t.Errorf("parseDomainState(%q) = %q", wantState, gotState)
		}
	}
}

func TestParseDomainStateRejectsUnknownState(t *testing.T) {
	for _, unknownOutput := range []string{"", "no state\n", "blocked\n"} {
		state, err := parseDomainState("vmherd-test-web", unknownOutput)
		if err == nil {
			t.Errorf("parseDomainState(%q) = %q, want an error", unknownOutput, state)
			continue
		}
		if !strings.Contains(err.Error(), "virsh dominfo vmherd-test-web") {
			t.Errorf("error for %q has no recovery hint: %v", unknownOutput, err)
		}
	}
}

func TestParseDomainNames(t *testing.T) {
	listOutput := "vmherd-demo-1a2b3c4d-web\nother-project-db\n\n"
	gotNames := parseDomainNames(listOutput)
	wantNames := []string{"vmherd-demo-1a2b3c4d-web", "other-project-db"}
	if !slices.Equal(gotNames, wantNames) {
		t.Errorf("parseDomainNames() = %v, want %v", gotNames, wantNames)
	}
	if gotNames := parseDomainNames("\n"); len(gotNames) != 0 {
		t.Errorf("parseDomainNames() of empty list = %v, want none", gotNames)
	}
}

func TestIsPoweredOn(t *testing.T) {
	testCases := map[VMState]bool{
		StateRunning:      true,
		StateIdle:         true,
		StatePaused:       true,
		StateShuttingDown: true,
		StateDying:        true,
		StatePMSuspended:  true,
		StateStopped:      false,
		StateCrashed:      false,
		StateNotCreated:   false,
	}
	for state, wantPoweredOn := range testCases {
		if gotPoweredOn := isPoweredOn(state); gotPoweredOn != wantPoweredOn {
			t.Errorf("isPoweredOn(%q) = %v, want %v", state, gotPoweredOn, wantPoweredOn)
		}
	}
}

func TestIsRunning(t *testing.T) {
	testCases := map[VMState]bool{
		StateRunning:      true,
		StateIdle:         true,
		StatePaused:       false,
		StateShuttingDown: false,
		StateStopped:      false,
		StateNotCreated:   false,
	}
	for state, wantRunning := range testCases {
		if gotRunning := state.IsRunning(); gotRunning != wantRunning {
			t.Errorf("%q.IsRunning() = %v, want %v", state, gotRunning, wantRunning)
		}
	}
}

func TestVirshStderrDetail(t *testing.T) {
	failingCommand := exec.Command("sh", "-c", "echo 'error: failed to connect to the hypervisor' >&2; exit 1")
	_, err := failingCommand.Output()
	if got := virshStderrDetail(err); got != ": error: failed to connect to the hypervisor" {
		t.Errorf("virshStderrDetail() = %q", got)
	}
	if got := virshStderrDetail(errors.New("not an exit error")); got != "" {
		t.Errorf("virshStderrDetail() of a non-exit error = %q, want empty", got)
	}
}

func TestEnsureRunningRefusesMissingDomain(t *testing.T) {
	err := EnsureRunning("vmherd-test-web", StateNotCreated)
	if err == nil || !strings.Contains(err.Error(), "vmherd up") {
		t.Errorf("EnsureRunning() of a missing domain = %v, want an error pointing to 'vmherd up'", err)
	}
	if err := EnsureRunning("vmherd-test-web", StateRunning); err != nil {
		t.Errorf("EnsureRunning() of a running domain = %v, want nil", err)
	}
}
