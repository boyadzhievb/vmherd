package tooling

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/imageurls"
)

// imageGuideLinkPrefix starts the link to a file on the vmherd GitHub page;
// the rest of the link is the file's path in the repository.
const imageGuideLinkPrefix = "https://github.com/boyadzhievb/vmherd/blob/main/"

// imageGuideRelativePath is the image URL guide whose table the checker reads.
const imageGuideRelativePath = imageurls.GuideRelativePath

// readmeRelativePath is the README that must reference the image guide.
const readmeRelativePath = "README.md"

// e2eTestRelativePath holds the e2e distro table. It is behind the
// integration build tag, so its URLs are read from the source text.
const e2eTestRelativePath = "test/e2e/e2e_test.go"

// e2eDistroRowPattern matches one {"name", "https://…", bool} row of the e2e
// distro table and captures the image name and URL.
var e2eDistroRowPattern = regexp.MustCompile(`\{"([^"]+)", "(https://[^"]+)", (?:true|false)\}`)

// e2eDistroTableStart opens the e2e distro table in e2e_test.go's source.
const e2eDistroTableStart = "var distroTable = []distroCase{"

// e2eDistroTableBlock returns the e2e distro table's source, from its opening
// line to the closing brace at the start of a line.
func e2eDistroTableBlock(t *testing.T, e2eSource string) string {
	t.Helper()
	tableStart := strings.Index(e2eSource, e2eDistroTableStart)
	if tableStart == -1 {
		t.Fatalf("found no e2e URLs: %s has no line %q", e2eTestRelativePath, e2eDistroTableStart)
	}
	tableText := e2eSource[tableStart:]
	if tableEnd := strings.Index(tableText, "\n}"); tableEnd != -1 {
		tableText = tableText[:tableEnd]
	}
	return tableText
}

// countDistroTableRows counts the lines of the distro table that hold an
// https:// URL, whatever the row's format.
func countDistroTableRows(distroTableText string) int {
	rowCount := 0
	for _, tableLine := range strings.Split(distroTableText, "\n") {
		if strings.Contains(tableLine, "\"https://") {
			rowCount++
		}
	}
	return rowCount
}

// readRepositoryFile returns a file's content by its path from the repository root.
func readRepositoryFile(t *testing.T, relativePath string) string {
	t.Helper()
	fileContent, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), relativePath))
	if err != nil {
		t.Fatalf("reading %s: %v", relativePath, err)
	}
	return string(fileContent)
}

// TestImageURLGuideLinkMatchesFile checks that the link the example Herdfile
// gives for the image guide names a file that exists in the repository.
func TestImageURLGuideLinkMatchesFile(t *testing.T) {
	exampleContent, err := config.ExampleHerdfile("vmherd-web-01234567", "192.168.151")
	if err != nil {
		t.Fatalf("rendering example Herdfile: %v", err)
	}
	linkStart := strings.Index(exampleContent, imageGuideLinkPrefix)
	if linkStart == -1 {
		t.Fatalf("example Herdfile has no link starting with %s", imageGuideLinkPrefix)
	}
	linkedPath := strings.Fields(exampleContent[linkStart+len(imageGuideLinkPrefix):])[0]
	if _, statErr := os.Stat(filepath.Join(findRepositoryRoot(t), linkedPath)); statErr != nil {
		t.Errorf("example Herdfile links %s%s, but the file is missing from the repository: %v", imageGuideLinkPrefix, linkedPath, statErr)
	}
}

// readmeSection returns the README text from a "## " heading to the next one.
func readmeSection(readmeContent, heading string) string {
	sectionStart := strings.Index(readmeContent, "\n"+heading+"\n")
	if sectionStart == -1 {
		return ""
	}
	sectionText := readmeContent[sectionStart+len(heading)+2:]
	if nextHeading := strings.Index(sectionText, "\n## "); nextHeading != -1 {
		sectionText = sectionText[:nextHeading]
	}
	return sectionText
}

// TestReadmeLinksImageGuide checks that the README's image section and the
// images row of its Herdfile reference both link the image guide.
func TestReadmeLinksImageGuide(t *testing.T) {
	readmeContent := readRepositoryFile(t, readmeRelativePath)

	if !strings.Contains(readmeSection(readmeContent, "## Supported Images"), imageGuideRelativePath) {
		t.Errorf("README.md section '## Supported Images' does not link %s", imageGuideRelativePath)
	}

	imagesRowFound := false
	for _, readmeLine := range strings.Split(readmeContent, "\n") {
		if !strings.HasPrefix(readmeLine, "| `images` |") {
			continue
		}
		imagesRowFound = true
		if !strings.Contains(readmeLine, imageGuideRelativePath) {
			t.Errorf("README.md Herdfile reference row for `images` does not link %s: %s", imageGuideRelativePath, readmeLine)
		}
	}
	if !imagesRowFound {
		t.Errorf("README.md has no Herdfile reference row starting with | `images` |")
	}
}

// TestImageURLListsAgree checks that every URL 'vmherd init' writes and every
// URL the e2e distro test boots appears in the image guide's table under the
// same image name, so checking the guide checks everything vmherd ships.
func TestImageURLListsAgree(t *testing.T) {
	guideImages, err := imageurls.ParseGuideTable(readRepositoryFile(t, imageGuideRelativePath))
	if err != nil {
		t.Fatalf("reading the table in %s: %v", imageGuideRelativePath, err)
	}
	guideURLsByName := make(map[string]string, len(guideImages))
	for _, guideImage := range guideImages {
		guideURLsByName[guideImage.Name] = guideImage.URL
	}

	requireInGuide := func(imageName, imageURL, sourceFile string) {
		if guideURL := guideURLsByName[imageName]; guideURL != imageURL {
			t.Errorf("%s from %s (%s) is missing from the table in %s (has %q): add the row or fix the URL",
				imageName, sourceFile, imageURL, imageGuideRelativePath, guideURL)
		}
	}

	for _, builtInImage := range config.BuiltInImages() {
		requireInGuide(builtInImage.Name, builtInImage.URL, "internal/config/images.go")
	}

	distroTableText := e2eDistroTableBlock(t, readRepositoryFile(t, e2eTestRelativePath))
	e2eRows := e2eDistroRowPattern.FindAllStringSubmatch(distroTableText, -1)
	if len(e2eRows) == 0 {
		t.Fatalf("found no e2e URLs in %s: the distro table's row format changed, update e2eDistroRowPattern", e2eTestRelativePath)
	}
	if tableRowCount := countDistroTableRows(distroTableText); tableRowCount != len(e2eRows) {
		t.Fatalf("%s's distro table has %d rows but e2eDistroRowPattern matches %d: a row's format changed, update the pattern",
			e2eTestRelativePath, tableRowCount, len(e2eRows))
	}
	for _, e2eRow := range e2eRows {
		requireInGuide(e2eRow[1], e2eRow[2], e2eTestRelativePath)
	}
}
