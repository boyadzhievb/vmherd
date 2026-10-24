//go:build integration

package vm

import (
	"testing"
)

const integrationDomain = "vmherd-integration-test"

func TestVMExistsReturnsFalseForMissing(t *testing.T) {
	if Exists("vmherd-nonexistent-domain-xyz") {
		t.Error("expected Exists to return false for a nonexistent domain")
	}
}

func TestVMStateReturnsNotCreatedForMissing(t *testing.T) {
	state := State("vmherd-nonexistent-domain-xyz")
	if state != "not created" {
		t.Errorf("expected 'not created', got %q", state)
	}
}
