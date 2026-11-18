// Package tooling checks the developer tooling and docs in the repository
// (Claude Code skills, the image URL guide docs/images.md and its links), not
// vmherd behaviour. It needs no libvirt and runs in CI.
package tooling

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// skillsDirectoryRelativePath is where Claude Code looks for project skills.
const skillsDirectoryRelativePath = ".claude/skills"

// skillFileName is the file Claude Code loads from each skill directory.
const skillFileName = "SKILL.md"

// goModFileName marks the repository root when walking up from this package.
const goModFileName = "go.mod"

// frontmatterDelimiter opens and closes the YAML frontmatter of a SKILL.md.
const frontmatterDelimiter = "---"

// skillFrontmatter holds the frontmatter fields vmherd requires of a skill.
// Claude Code itself defaults name to the directory and only recommends a
// description; vmherd requires both, with name equal to the directory.
type skillFrontmatter struct {
	Name        string `yaml:"name"`        // must equal the skill's directory name
	Description string `yaml:"description"` // shown in the skill list; must not be empty
}

// findRepositoryRoot walks up from the test's working directory (the package
// directory) to the first directory containing go.mod.
func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	currentDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(currentDirectory, goModFileName)); statErr == nil {
			return currentDirectory
		}
		parentDirectory := filepath.Dir(currentDirectory)
		if parentDirectory == currentDirectory {
			t.Fatalf("no %s found above the test directory: run the test inside the vmherd repository", goModFileName)
		}
		currentDirectory = parentDirectory
	}
}

// isFrontmatterDelimiter reports whether a line is '---', ignoring trailing
// whitespace only: an indented '---' is content (e.g. inside a block scalar).
func isFrontmatterDelimiter(contentLine string) bool {
	return strings.TrimRight(contentLine, " \t") == frontmatterDelimiter
}

// extractFrontmatter returns the YAML between the opening and closing '---'
// lines at the start of a SKILL.md, or an error saying what is missing.
func extractFrontmatter(skillFileContent []byte) ([]byte, error) {
	normalizedContent := bytes.ReplaceAll(skillFileContent, []byte("\r\n"), []byte("\n"))
	contentLines := strings.Split(string(normalizedContent), "\n")
	if !isFrontmatterDelimiter(contentLines[0]) {
		return nil, errors.New("no frontmatter: the file must start with a '---' line")
	}
	for lineIndex := 1; lineIndex < len(contentLines); lineIndex++ {
		if isFrontmatterDelimiter(contentLines[lineIndex]) {
			return []byte(strings.Join(contentLines[1:lineIndex], "\n")), nil
		}
	}
	return nil, errors.New("frontmatter is not closed: add a '---' line after it")
}

// checkSkillEntry returns every layout problem of one entry in .claude/skills/,
// each naming the entry and the fix. Symlinks are followed (os.Stat).
func checkSkillEntry(skillsDirectory, entryName string) []string {
	entryPath := filepath.Join(skillsDirectory, entryName)
	skillName := strings.TrimSuffix(entryName, filepath.Ext(entryName))
	entryInfo, err := os.Stat(entryPath)
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot read the entry (%v): fix it (a broken link?) or remove it", entryName, err)}
	}
	if !entryInfo.IsDir() {
		return []string{fmt.Sprintf("%s: a skill must be a directory %s/%s/ containing %s",
			entryName, skillsDirectoryRelativePath, skillName, skillFileName)}
	}
	skillFileContent, err := os.ReadFile(filepath.Join(entryPath, skillFileName))
	if err != nil {
		return []string{fmt.Sprintf("%s: cannot read %s (%v): add %s/%s/%s",
			entryName, skillFileName, err, skillsDirectoryRelativePath, entryName, skillFileName)}
	}
	frontmatterYAML, err := extractFrontmatter(skillFileContent)
	if err != nil {
		return []string{fmt.Sprintf("%s/%s: %v", entryName, skillFileName, err)}
	}
	var frontmatter skillFrontmatter
	if err := yaml.Unmarshal(frontmatterYAML, &frontmatter); err != nil {
		return []string{fmt.Sprintf("%s/%s: frontmatter is not valid YAML (%v): fix its syntax", entryName, skillFileName, err)}
	}
	var entryProblems []string
	if frontmatter.Name != entryName {
		entryProblems = append(entryProblems, fmt.Sprintf("%s/%s: frontmatter name is %q: set 'name: %s' to match the directory",
			entryName, skillFileName, frontmatter.Name, entryName))
	}
	if strings.TrimSpace(frontmatter.Description) == "" {
		entryProblems = append(entryProblems, fmt.Sprintf("%s/%s: frontmatter has no description: add 'description: <what the skill does>'",
			entryName, skillFileName))
	}
	return entryProblems
}

// listSkillEntryNames returns the sorted names of the entries in a skills
// directory that the layout test checks. Hidden entries are skipped: Claude
// Code ignores them, and Finder creates .DS_Store.
func listSkillEntryNames(skillsDirectory string) ([]string, error) {
	directoryEntries, err := os.ReadDir(skillsDirectory)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", skillsDirectory, err)
	}
	entryNames := make([]string, 0, len(directoryEntries))
	for _, directoryEntry := range directoryEntries {
		if strings.HasPrefix(directoryEntry.Name(), ".") {
			continue
		}
		entryNames = append(entryNames, directoryEntry.Name())
	}
	sort.Strings(entryNames)
	return entryNames, nil
}

// TestProjectSkillsUseSkillLayout checks every entry in .claude/skills/ is a
// <name>/SKILL.md that Claude Code loads, and reports all problems in one run.
func TestProjectSkillsUseSkillLayout(t *testing.T) {
	skillsDirectory := filepath.Join(findRepositoryRoot(t), skillsDirectoryRelativePath)
	entryNames, err := listSkillEntryNames(skillsDirectory)
	if err != nil {
		t.Fatalf("listing the project skills: %v", err)
	}
	if len(entryNames) == 0 {
		t.Fatalf("%s is empty: the project skills are missing", skillsDirectoryRelativePath)
	}
	for _, entryName := range entryNames {
		for _, entryProblem := range checkSkillEntry(skillsDirectory, entryName) {
			t.Error(entryProblem)
		}
	}
}

// TestListSkillEntryNamesSkipsHiddenEntries checks hidden entries such as
// .DS_Store are not checked, while a flat .md file and a skill directory are.
func TestListSkillEntryNamesSkipsHiddenEntries(t *testing.T) {
	skillsDirectory := t.TempDir()
	for _, flatFileName := range []string{".DS_Store", "commit.md"} {
		if err := os.WriteFile(filepath.Join(skillsDirectory, flatFileName), nil, 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", flatFileName, err)
		}
	}
	if err := os.Mkdir(filepath.Join(skillsDirectory, "test"), 0o700); err != nil {
		t.Fatalf("creating fixture skill directory: %v", err)
	}
	entryNames, err := listSkillEntryNames(skillsDirectory)
	if err != nil {
		t.Fatalf("listing fixture skills: %v", err)
	}
	expectedEntryNames := []string{"commit.md", "test"}
	if strings.Join(entryNames, ",") != strings.Join(expectedEntryNames, ",") {
		t.Fatalf("expected entries %q, got %q", expectedEntryNames, entryNames)
	}
}

// skillFixtureKind says what createSkillFixture puts at a fixture entry.
type skillFixtureKind string

const (
	// skillFixtureDirectory is a directory, with a SKILL.md when content is set.
	skillFixtureDirectory skillFixtureKind = "directory"
	// skillFixtureFlatFile is a file in place of a skill directory.
	skillFixtureFlatFile skillFixtureKind = "flat file"
	// skillFixtureDanglingSymlink is a symlink whose target does not exist.
	skillFixtureDanglingSymlink skillFixtureKind = "dangling symlink"
	// skillFixtureSymlinkToDirectory is a symlink to a skill directory elsewhere,
	// as .claude/skills/openspec-* link into .agents/skills/.
	skillFixtureSymlinkToDirectory skillFixtureKind = "symlink to directory"
)

// createSkillFixture creates one entry of the given kind in skillsDirectory,
// writing skillContent as its SKILL.md (or as the flat file) when not empty.
func createSkillFixture(t *testing.T, skillsDirectory, entryName string, fixtureKind skillFixtureKind, skillContent string) {
	t.Helper()
	entryPath := filepath.Join(skillsDirectory, entryName)
	writeFixtureSkillDirectory := func(directoryPath string) {
		if err := os.Mkdir(directoryPath, 0o700); err != nil {
			t.Fatalf("creating fixture %s: %v", directoryPath, err)
		}
		if skillContent == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(directoryPath, skillFileName), []byte(skillContent), 0o600); err != nil {
			t.Fatalf("writing fixture SKILL.md: %v", err)
		}
	}
	switch fixtureKind {
	case skillFixtureDirectory:
		writeFixtureSkillDirectory(entryPath)
	case skillFixtureFlatFile:
		if err := os.WriteFile(entryPath, []byte(skillContent), 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", entryPath, err)
		}
	case skillFixtureDanglingSymlink:
		if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), entryPath); err != nil {
			t.Fatalf("creating fixture link %s: %v", entryPath, err)
		}
	case skillFixtureSymlinkToDirectory:
		targetDirectory := filepath.Join(t.TempDir(), "shared-"+entryName)
		writeFixtureSkillDirectory(targetDirectory)
		if err := os.Symlink(targetDirectory, entryPath); err != nil {
			t.Fatalf("creating fixture link %s: %v", entryPath, err)
		}
	default:
		t.Fatalf("unknown fixture kind %q", fixtureKind)
	}
}

// TestCheckSkillEntryReportsEachProblem checks the layout check itself on
// fixture skills, so a check that always passes cannot hide a broken skill.
func TestCheckSkillEntryReportsEachProblem(t *testing.T) {
	validSkillContent := "---\nname: test\ndescription: Run tests\n---\n# Body\n"
	testCases := []struct {
		caseName         string           // describes the fixture
		entryName        string           // entry created in the fixture skills directory
		fixtureKind      skillFixtureKind // what the entry is
		skillContent     string           // SKILL.md (or flat file) content; empty means none
		expectedProblems []string         // one substring per expected problem, in order
	}{
		{caseName: "valid skill", entryName: "test", fixtureKind: skillFixtureDirectory,
			skillContent: validSkillContent},
		{caseName: "flat file", entryName: "commit.md", fixtureKind: skillFixtureFlatFile, skillContent: "# Commit\n",
			expectedProblems: []string{"commit.md: a skill must be a directory .claude/skills/commit/ containing SKILL.md"}},
		{caseName: "no SKILL.md", entryName: "test", fixtureKind: skillFixtureDirectory,
			expectedProblems: []string{"test: cannot read SKILL.md"}},
		{caseName: "dangling symlink", entryName: "test", fixtureKind: skillFixtureDanglingSymlink,
			expectedProblems: []string{"test: cannot read the entry"}},
		{caseName: "symlink to valid skill directory", entryName: "test", fixtureKind: skillFixtureSymlinkToDirectory,
			skillContent: validSkillContent},
		{caseName: "CRLF line endings", entryName: "test", fixtureKind: skillFixtureDirectory,
			skillContent: "---\r\nname: test\r\ndescription: Run tests\r\n---\r\n# Body\r\n"},
		{caseName: "no frontmatter", entryName: "test", fixtureKind: skillFixtureDirectory, skillContent: "# Run tests\n",
			expectedProblems: []string{"test/SKILL.md: no frontmatter"}},
		{caseName: "leading space before opening delimiter", entryName: "test", fixtureKind: skillFixtureDirectory,
			skillContent:     " ---\nname: test\ndescription: Run tests\n---\n",
			expectedProblems: []string{"test/SKILL.md: no frontmatter"}},
		{caseName: "indented delimiter inside a block scalar", entryName: "test", fixtureKind: skillFixtureDirectory,
			skillContent: "---\nname: test\ndescription: |\n  ---\n  Run tests\n---\n"},
		{caseName: "unclosed frontmatter", entryName: "test", fixtureKind: skillFixtureDirectory, skillContent: "---\nname: test\n",
			expectedProblems: []string{"test/SKILL.md: frontmatter is not closed"}},
		{caseName: "invalid YAML", entryName: "test", fixtureKind: skillFixtureDirectory, skillContent: "---\nname: [test\n---\n",
			expectedProblems: []string{"test/SKILL.md: frontmatter is not valid YAML"}},
		{caseName: "name mismatch", entryName: "test", fixtureKind: skillFixtureDirectory,
			skillContent:     "---\nname: tests\ndescription: Run tests\n---\n",
			expectedProblems: []string{"set 'name: test'"}},
		{caseName: "no description", entryName: "test", fixtureKind: skillFixtureDirectory, skillContent: "---\nname: test\n---\n",
			expectedProblems: []string{"test/SKILL.md: frontmatter has no description"}},
		{caseName: "empty frontmatter", entryName: "test", fixtureKind: skillFixtureDirectory, skillContent: "---\n---\n",
			expectedProblems: []string{"set 'name: test'", "test/SKILL.md: frontmatter has no description"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			skillsDirectory := t.TempDir()
			createSkillFixture(t, skillsDirectory, testCase.entryName, testCase.fixtureKind, testCase.skillContent)
			entryProblems := checkSkillEntry(skillsDirectory, testCase.entryName)
			if len(entryProblems) != len(testCase.expectedProblems) {
				t.Fatalf("expected %d problems %q, got %d: %q",
					len(testCase.expectedProblems), testCase.expectedProblems, len(entryProblems), entryProblems)
			}
			for problemIndex, expectedProblem := range testCase.expectedProblems {
				if !strings.Contains(entryProblems[problemIndex], expectedProblem) {
					t.Errorf("problem %d: expected it to contain %q, got %q", problemIndex, expectedProblem, entryProblems[problemIndex])
				}
			}
		})
	}
}
