package main

import (
	"strings"
	"testing"
)

// TestMissingNinePSummary checks that the 9p failure names the cause and the
// recovery, alone or under a failed provisioning step.
func TestMissingNinePSummary(t *testing.T) {
	ninePSummary := missingNinePSummary("alma")
	for _, fragment := range []string{"no 9p support", "Rocky and AlmaLinux", "shared_folders: []", "vmherd reload alma"} {
		if !strings.Contains(ninePSummary, fragment) {
			t.Errorf("missingNinePSummary() does not mention %q: %s", fragment, ninePSummary)
		}
	}
	if got := joinFailureSummaries("", ninePSummary); got != ninePSummary {
		t.Errorf("joinFailureSummaries without a step failure = %q, want the 9p summary alone", got)
	}
	stepLine := "vmherd: provisioning step 1 of 1 (exit 1) failed; later steps were skipped"
	if got := joinFailureSummaries(stepLine, ninePSummary); !strings.HasPrefix(got, stepLine+"\n  also: ") {
		t.Errorf("joinFailureSummaries with a step failure = %q, want the step first", got)
	}
}
