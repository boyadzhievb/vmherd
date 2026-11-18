// Command image-urls-check is a release tool, not part of the vmherd binary.
// It checks every image URL in docs/images.md against the vendor and, unless
// --check is given, rewrites stale ones in the working tree for review.
// It never commits, tags or pushes.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/boyadzhievb/vmherd/internal/imageurls"
)

// exitCodeStaleOrFailed is the exit code when any URL is stale or failed.
const exitCodeStaleOrFailed = 1

// main parses the flags, runs the check and exits non-zero on any stale or
// failed URL.
func main() {
	checkOnly := flag.Bool("check", false, "only report stale URLs; never write a file")
	repositoryDirectory := flag.String("repo", ".", "root of the vmherd repository")
	flag.Parse()

	if err := imageurls.NewChecker().Run(*repositoryDirectory, *checkOnly, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "image-urls-check: %v\n", err)
		os.Exit(exitCodeStaleOrFailed)
	}
}
