//go:build integration

package vm

import (
	"testing"
)

const integrationDomain = "vmherd-integration-test"

func TestVMStateReturnsNotCreatedForMissing(t *testing.T) {
	state, err := State("vmherd-nonexistent-domain-xyz")
	if err != nil {
		t.Fatalf("State() of a missing domain returned an error: %v", err)
	}
	if state != StateNotCreated {
		t.Errorf("expected 'not created', got %q", state)
	}
}
