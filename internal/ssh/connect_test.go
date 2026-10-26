package ssh

import (
	"fmt"
	"testing"
	"time"
)

// TestSSHBaseArgs verifies that sshBaseArgs produces the correct SSH arguments
// with proper count, key path placement, user@ip format, and timeout value.
func TestSSHBaseArgs(t *testing.T) {
	testKeyPath := "/home/vmherd/.ssh/id_ed25519"
	testUser := "vmherd"
	testIP := "192.168.122.10"

	args := sshBaseArgs(testKeyPath, testUser, testIP)

	// Expected layout: 4x("-o", value) + "-i" + keyPath + user@ip = 11 elements.
	const expectedArgCount = 11
	if len(args) != expectedArgCount {
		t.Fatalf("sshBaseArgs returned %d args, want %d: %v", len(args), expectedArgCount, args)
	}

	// Verify ConnectTimeout uses the package constant.
	expectedTimeoutArg := fmt.Sprintf("ConnectTimeout=%d", sshConnectTimeout)
	if args[1] != expectedTimeoutArg {
		t.Errorf("args[1] = %q, want %q", args[1], expectedTimeoutArg)
	}

	// Verify the "-i" flag is at index 8, with the key path at index 9.
	const keyFlagIndex = 8
	const keyPathIndex = 9
	if args[keyFlagIndex] != "-i" {
		t.Errorf("args[%d] = %q, want %q", keyFlagIndex, args[keyFlagIndex], "-i")
	}
	if args[keyPathIndex] != testKeyPath {
		t.Errorf("args[%d] = %q, want key path %q", keyPathIndex, args[keyPathIndex], testKeyPath)
	}

	// Verify user@ip format in the last element.
	expectedUserHost := fmt.Sprintf("%s@%s", testUser, testIP)
	lastArg := args[len(args)-1]
	if lastArg != expectedUserHost {
		t.Errorf("last arg = %q, want %q", lastArg, expectedUserHost)
	}
}

// TestExponentialBackoffConstants verifies that the retry interval constants
// form a valid exponential backoff configuration.
func TestExponentialBackoffConstants(t *testing.T) {
	if initialRetryInterval <= 0 {
		t.Errorf("initialRetryInterval = %s, must be positive", initialRetryInterval)
	}
	if maxRetryInterval < initialRetryInterval {
		t.Errorf("maxRetryInterval (%s) must be >= initialRetryInterval (%s)",
			maxRetryInterval, initialRetryInterval)
	}
	if defaultWaitTimeout < maxRetryInterval {
		t.Errorf("defaultWaitTimeout (%s) must be >= maxRetryInterval (%s)",
			defaultWaitTimeout, maxRetryInterval)
	}

	// Simulate the backoff sequence and ensure it converges to the cap.
	currentInterval := initialRetryInterval
	for i := 0; i < 10; i++ {
		currentInterval *= 2
		if currentInterval > maxRetryInterval {
			currentInterval = maxRetryInterval
		}
	}
	if currentInterval != maxRetryInterval {
		t.Errorf("backoff did not converge to cap: got %s, want %s",
			currentInterval, maxRetryInterval)
	}
}

// TestBackoffSequenceValues verifies the exact sequence of intervals
// produced by the exponential backoff logic.
func TestBackoffSequenceValues(t *testing.T) {
	expectedSequence := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		16 * time.Second, // capped
	}

	currentInterval := initialRetryInterval
	for i, expected := range expectedSequence {
		if currentInterval != expected {
			t.Errorf("step %d: interval = %s, want %s", i, currentInterval, expected)
		}
		currentInterval *= 2
		if currentInterval > maxRetryInterval {
			currentInterval = maxRetryInterval
		}
	}
}
