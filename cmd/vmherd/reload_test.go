package main

import (
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
)

// unreachableImageURL refuses connections immediately (port 1 on loopback), so
// the image check fails without touching the network.
const unreachableImageURL = "https://127.0.0.1:1/noble-server-cloudimg-amd64.img"

// TestCheckReloadPrerequisitesStopsWhenImageUnavailable checks that reload's
// pre-check fails, and says nothing was changed, when an image can't be
// downloaded or isn't defined — the cases that used to leave no VM behind.
func TestCheckReloadPrerequisitesStopsWhenImageUnavailable(t *testing.T) {
	testCases := []struct {
		name   string            // what makes the image unavailable
		images map[string]string // the Herdfile images section
	}{
		{name: "vendor unreachable", images: map[string]string{"ubuntu:noble": unreachableImageURL}},
		{name: "image not defined", images: map[string]string{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			vmsToReload := []config.VM{{Name: "ctrl", Image: "ubuntu:noble"}}

			resolvedImages, err := checkReloadPrerequisites(vmsToReload, testCase.images)
			if err == nil {
				t.Fatalf("expected an error, got resolved images %v", resolvedImages)
			}
			if !strings.Contains(err.Error(), "reload stopped before changing any VM") {
				t.Errorf("error should say no VM was changed, got: %v", err)
			}
		})
	}
}
