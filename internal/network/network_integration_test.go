//go:build integration

package network

import (
	"testing"
)

func TestNetworkExistsReturnsFalseForMissing(t *testing.T) {
	if exists("vmherd-nonexistent-net-xyz") {
		t.Error("expected exists to return false for a nonexistent network")
	}
}

func TestNetworkIsActiveReturnsFalseForMissing(t *testing.T) {
	if isActive("vmherd-nonexistent-net-xyz") {
		t.Error("expected isActive to return false for a nonexistent network")
	}
}

func TestNetworkDestroyNoopForMissing(t *testing.T) {
	err := Destroy("vmherd-nonexistent-net-xyz")
	if err != nil {
		t.Errorf("Destroy of nonexistent network should be a no-op, got: %v", err)
	}
}
