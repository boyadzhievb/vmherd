package imageurls

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// resultLabelWidth is the width of the OK/STALE/FAILED column; the detail
// lines of a stale or failed URL are indented by the same width.
const resultLabelWidth = 8

// imageNameColumnWidth is the width of the image name column.
const imageNameColumnWidth = 16

// detailLabelWidth is the width of the "old:", "new:", ... labels of a
// stale URL's detail lines.
const detailLabelWidth = 9

// Run checks every URL in repositoryDirectory's image guide and prints one
// result per image to output. Unless checkOnly, it rewrites each stale URL
// with its replacement. It returns an error if any URL was stale or failed.
func (checker *Checker) Run(repositoryDirectory string, checkOnly bool, output io.Writer) error {
	guideContent, readError := os.ReadFile(filepath.Join(repositoryDirectory, GuideRelativePath))
	if readError != nil {
		return fmt.Errorf("reading the image guide %s: %w — run the checker from the vmherd repository or pass --repo", GuideRelativePath, readError)
	}
	guideImages, parseError := ParseGuideTable(string(guideContent))
	if parseError != nil {
		return fmt.Errorf("reading the image table: %w", parseError)
	}

	var printErrors []error
	staleCount, failedCount := 0, 0
	for _, guideImage := range guideImages {
		checkResult := checker.CheckImage(guideImage)
		var rewrittenFiles []string
		if checkResult.Status == StatusStale && !checkOnly {
			var rewriteError error
			rewrittenFiles, rewriteError = RewriteURL(repositoryDirectory, guideImage.URL, checkResult.ReplacementURL)
			if rewriteError != nil {
				checkResult = failedResult(guideImage, fmt.Errorf("%s: rewriting %s: %w", guideImage.Name, guideImage.URL, rewriteError))
			}
		}
		switch checkResult.Status {
		case StatusStale:
			staleCount++
		case StatusFailed:
			failedCount++
		}
		printErrors = append(printErrors, printCheckResult(output, checkResult, checkOnly, rewrittenFiles))
	}

	printErrors = append(printErrors, printRunSummary(output, len(guideImages), staleCount, failedCount, checkOnly))
	if printError := errors.Join(printErrors...); printError != nil {
		return fmt.Errorf("writing the check report: %w", printError)
	}
	if staleCount+failedCount > 0 {
		return fmt.Errorf("%d of %d image URLs are stale or failed", staleCount+failedCount, len(guideImages))
	}
	return nil
}

// printCheckResult writes one image's result: an OK line, or a STALE or
// FAILED line with the details below it.
func printCheckResult(output io.Writer, checkResult CheckResult, checkOnly bool, rewrittenFiles []string) error {
	var resultText strings.Builder
	writeResultLine := func(resultLabel string) {
		fmt.Fprintf(&resultText, "%-*s%-*s %s\n", resultLabelWidth, resultLabel, imageNameColumnWidth, checkResult.Image.Name, checkResult.Image.URL)
	}
	writeDetailLine := func(detailLabel, detailText string) {
		fmt.Fprintf(&resultText, "%*s%-*s %s\n", resultLabelWidth, "", detailLabelWidth, detailLabel, detailText)
	}
	switch checkResult.Status {
	case StatusCurrent:
		writeResultLine("OK")
	case StatusStale:
		writeResultLine("STALE")
		writeDetailLine("reason:", checkResult.StaleReason)
		writeDetailLine("old:", checkResult.Image.URL)
		writeDetailLine("new:", checkResult.ReplacementURL)
		writeDetailLine(checkResult.Checksum.Algorithm+":", checkResult.Checksum.Hash)
		writeDetailLine("listing:", checkResult.ListingURL)
		writeDetailLine("checksum:", checkResult.ChecksumURL)
		if checkOnly {
			writeDetailLine("--check:", "no file written")
		} else {
			writeDetailLine("rewrote:", strings.Join(rewrittenFiles, ", "))
		}
	case StatusFailed:
		writeResultLine("FAILED")
		fmt.Fprintf(&resultText, "%*s%v\n", resultLabelWidth, "", checkResult.Err)
	}
	_, writeError := io.WriteString(output, resultText.String())
	return writeError
}

// printRunSummary writes what the maintainer does next.
func printRunSummary(output io.Writer, imageCount, staleCount, failedCount int, checkOnly bool) error {
	var summaryText strings.Builder
	if staleCount+failedCount == 0 {
		fmt.Fprintf(&summaryText, "All %d image URLs are current.\n", imageCount)
	}
	if staleCount > 0 && checkOnly {
		summaryText.WriteString("Stale image URLs found: run 'go run ./cmd/image-urls-check' without --check to rewrite them, " +
			"then review 'git diff', commit, and release again.\n")
	}
	if staleCount > 0 && !checkOnly {
		summaryText.WriteString("Image URLs were rewritten: review 'git diff --stat' and 'git diff', commit, and run the release again.\n")
	}
	if failedCount > 0 {
		fmt.Fprintf(&summaryText, "%d image URLs failed: open the directory named above, pick the current file, "+
			"and update its URL in %s (and in internal/config/images.go or test/e2e/e2e_test.go if they hold it).\n",
			failedCount, GuideRelativePath)
	}
	_, writeError := io.WriteString(output, summaryText.String())
	return writeError
}
