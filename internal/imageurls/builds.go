package imageurls

import (
	"errors"
	"net/url"
	"sort"
	"strings"
)

// errNoNewerBuild means no listed file has the old file's name pattern with a
// higher build number.
var errNoNewerBuild = errors.New("no newer build of the same file is listed")

// errAmbiguousBuild means several listed files could be the newer build, so
// the checker won't guess.
var errAmbiguousBuild = errors.New("several files could be the newer build, so none is picked")

// fixedNameWords are architecture and format words whose digits are part of
// the name, never a build number (x86_64 is not build 86.64).
var fixedNameWords = []string{"x86_64", "amd64", "aarch64", "arm64", "qcow2"}

// maxBuildNumberDigits is the longest digit run taken as a build number; a
// date-time stamp has 14 digits, so longer runs are not a build.
const maxBuildNumberDigits = 14

// nameToken is one run of a file name: all digits, or no digits.
type nameToken struct {
	text          string // the run's characters
	isDigits      bool   // true for a run of decimal digits
	startOffset   int    // byte offset of the run in the file name
	isBuildNumber bool   // set on the old name's tokens: a digit run that may grow
}

// selectNewerBuild returns the listed file that is oldFilename in a newer
// build: same text except the build numbers, which are higher (numerically,
// left to right). See markBuildNumbers. A tie, or several newer builds while
// the old file is still listed, is errAmbiguousBuild.
func selectNewerBuild(oldFilename, directoryURL string, listedFilenames []string) (string, error) {
	namePattern := markBuildNumbers(oldFilename, directoryURL)
	oldStillListed := false
	var newerFilenames []string
	newerTokensByFilename := make(map[string][]nameToken)
	for _, listedFilename := range listedFilenames {
		if listedFilename == oldFilename {
			oldStillListed = true
			continue
		}
		listedTokens := splitDigitRuns(listedFilename)
		if matchesNamePattern(namePattern, listedTokens) && compareBuildNumbers(namePattern, listedTokens, namePattern) > 0 {
			newerFilenames = append(newerFilenames, listedFilename)
			newerTokensByFilename[listedFilename] = listedTokens
		}
	}

	if len(newerFilenames) == 0 {
		return "", errNoNewerBuild
	}
	sort.SliceStable(newerFilenames, func(firstIndex, secondIndex int) bool {
		return compareBuildNumbers(namePattern, newerTokensByFilename[newerFilenames[firstIndex]], newerTokensByFilename[newerFilenames[secondIndex]]) > 0
	})
	if len(newerFilenames) > 1 {
		topBuildTied := compareBuildNumbers(namePattern, newerTokensByFilename[newerFilenames[0]], newerTokensByFilename[newerFilenames[1]]) == 0
		if topBuildTied || oldStillListed {
			return "", errAmbiguousBuild
		}
	}
	return newerFilenames[0], nil
}

// markBuildNumbers splits oldFilename and marks its build numbers: digit runs
// that are not inside a fixedNameWords word and don't equal a path segment of
// directoryURL (the release, e.g. 45 in /releases/45/).
func markBuildNumbers(oldFilename, directoryURL string) []nameToken {
	fixedRanges := fixedWordRanges(strings.ToLower(oldFilename))
	directorySegments := make(map[string]bool)
	if parsedDirectoryURL, parseError := url.Parse(directoryURL); parseError == nil {
		for _, pathSegment := range strings.Split(parsedDirectoryURL.Path, "/") {
			directorySegments[pathSegment] = true
		}
	}

	nameTokens := splitDigitRuns(oldFilename)
	for tokenIndex, nameToken := range nameTokens {
		if !nameToken.isDigits || directorySegments[nameToken.text] {
			continue
		}
		tokenEnd := nameToken.startOffset + len(nameToken.text)
		insideFixedWord := false
		for _, fixedRange := range fixedRanges {
			insideFixedWord = insideFixedWord || (nameToken.startOffset >= fixedRange[0] && tokenEnd <= fixedRange[1])
		}
		nameTokens[tokenIndex].isBuildNumber = !insideFixedWord
	}
	return nameTokens
}

// fixedWordRanges returns the [start, end) byte ranges of every fixedNameWords
// occurrence in a lower-cased file name.
func fixedWordRanges(lowerFilename string) [][2]int {
	var fixedRanges [][2]int
	for _, fixedWord := range fixedNameWords {
		for searchOffset := 0; ; {
			wordIndex := strings.Index(lowerFilename[searchOffset:], fixedWord)
			if wordIndex == -1 {
				break
			}
			wordStart := searchOffset + wordIndex
			fixedRanges = append(fixedRanges, [2]int{wordStart, wordStart + len(fixedWord)})
			searchOffset = wordStart + len(fixedWord)
		}
	}
	return fixedRanges
}

// splitDigitRuns splits a file name into alternating digit and non-digit runs.
func splitDigitRuns(filename string) []nameToken {
	var nameTokens []nameToken
	for byteOffset, nameCharacter := range filename {
		characterIsDigit := nameCharacter >= '0' && nameCharacter <= '9'
		lastIndex := len(nameTokens) - 1
		if lastIndex >= 0 && nameTokens[lastIndex].isDigits == characterIsDigit {
			nameTokens[lastIndex].text += string(nameCharacter)
			continue
		}
		nameTokens = append(nameTokens, nameToken{text: string(nameCharacter), isDigits: characterIsDigit, startOffset: byteOffset})
	}
	return nameTokens
}

// matchesNamePattern reports whether a candidate has the old name's runs:
// identical text everywhere except build numbers, which must be digit runs
// of at most maxBuildNumberDigits.
func matchesNamePattern(namePattern, candidateTokens []nameToken) bool {
	if len(namePattern) != len(candidateTokens) {
		return false
	}
	for tokenIndex, patternToken := range namePattern {
		candidateToken := candidateTokens[tokenIndex]
		if patternToken.isDigits != candidateToken.isDigits {
			return false
		}
		if !patternToken.isBuildNumber && patternToken.text != candidateToken.text {
			return false
		}
		if patternToken.isBuildNumber && len(candidateToken.text) > maxBuildNumberDigits {
			return false
		}
	}
	return true
}

// compareBuildNumbers compares two names that match namePattern by their
// build numbers, numerically, left to right: -1, 0 or 1.
func compareBuildNumbers(namePattern, firstTokens, secondTokens []nameToken) int {
	for tokenIndex, patternToken := range namePattern {
		if !patternToken.isBuildNumber {
			continue
		}
		if comparison := compareDecimalNumbers(firstTokens[tokenIndex].text, secondTokens[tokenIndex].text); comparison != 0 {
			return comparison
		}
	}
	return 0
}

// compareDecimalNumbers compares two digit strings as numbers of any length,
// ignoring leading zeros: -1, 0 or 1.
func compareDecimalNumbers(firstDigits, secondDigits string) int {
	firstDigits = strings.TrimLeft(firstDigits, "0")
	secondDigits = strings.TrimLeft(secondDigits, "0")
	if len(firstDigits) != len(secondDigits) {
		if len(firstDigits) < len(secondDigits) {
			return -1
		}
		return 1
	}
	return strings.Compare(firstDigits, secondDigits)
}
