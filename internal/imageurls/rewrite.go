package imageurls

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// rewriteTargetPaths returns the files, relative to the repository, that may
// hold a shipped or documented image URL, in the order they are rewritten.
func rewriteTargetPaths() []string {
	return []string{"internal/config/images.go", GuideRelativePath, "test/e2e/e2e_test.go"}
}

// urlEndDelimiters are the characters that may follow a URL in the target
// files (Go string, Markdown table, link or code); anything else means a
// longer URL that only starts with the old one.
const urlEndDelimiters = "\" \t\r\n|)>`"

// RewriteURL replaces oldURL with newURL, as literal text ending at a
// delimiter (see urlEndDelimiters), in every target file under
// repositoryDirectory that holds it, and returns the files changed.
// The guide must hold oldURL; if it doesn't, no file is written.
func RewriteURL(repositoryDirectory, oldURL, newURL string) ([]string, error) {
	pendingContents := make(map[string]string)
	var pendingPaths []string
	for _, relativePath := range rewriteTargetPaths() {
		fileContent, readError := os.ReadFile(filepath.Join(repositoryDirectory, relativePath))
		if errors.Is(readError, os.ErrNotExist) && relativePath != GuideRelativePath {
			continue
		}
		if readError != nil {
			return nil, fmt.Errorf("reading %s to replace %s: %w", relativePath, oldURL, readError)
		}
		rewrittenContent, replacedCount := replaceDelimitedURL(string(fileContent), oldURL, newURL)
		if replacedCount == 0 {
			if relativePath == GuideRelativePath {
				return nil, fmt.Errorf("%s does not contain %s, so nothing was rewritten: run the checker again on a clean tree", GuideRelativePath, oldURL)
			}
			continue
		}
		pendingContents[relativePath] = rewrittenContent
		pendingPaths = append(pendingPaths, relativePath)
	}

	for writtenCount, relativePath := range pendingPaths {
		if writeError := writeFileAtomically(filepath.Join(repositoryDirectory, relativePath), pendingContents[relativePath]); writeError != nil {
			alreadyRewritten := "none"
			if writtenCount > 0 {
				alreadyRewritten = strings.Join(pendingPaths[:writtenCount], ", ")
			}
			return pendingPaths[:writtenCount], fmt.Errorf("writing %s with %s: %w — already rewritten: %s; check 'git diff' and restore them",
				relativePath, newURL, writeError, alreadyRewritten)
		}
	}
	return pendingPaths, nil
}

// replaceDelimitedURL replaces each oldURL in content that is followed by one
// of urlEndDelimiters or the end of content, and returns how many it replaced.
func replaceDelimitedURL(content, oldURL, newURL string) (string, int) {
	var rewrittenContent strings.Builder
	replacedCount := 0
	for {
		matchIndex := strings.Index(content, oldURL)
		if matchIndex == -1 {
			rewrittenContent.WriteString(content)
			return rewrittenContent.String(), replacedCount
		}
		matchEnd := matchIndex + len(oldURL)
		rewrittenContent.WriteString(content[:matchIndex])
		if matchEnd == len(content) || strings.IndexByte(urlEndDelimiters, content[matchEnd]) != -1 {
			rewrittenContent.WriteString(newURL)
			replacedCount++
		} else {
			rewrittenContent.WriteString(oldURL)
		}
		content = content[matchEnd:]
	}
}

// writeFileAtomically replaces a file's content by writing a temporary file
// in the same directory, with the old file's mode, and renaming it into place.
func writeFileAtomically(filePath, fileContent string) error {
	fileInfo, statError := os.Stat(filePath)
	if statError != nil {
		return fmt.Errorf("reading the mode of %s: %w", filePath, statError)
	}
	temporaryFile, createError := os.CreateTemp(filepath.Dir(filePath), "."+filepath.Base(filePath)+".tmp-*")
	if createError != nil {
		return fmt.Errorf("creating a temporary file next to %s: %w", filePath, createError)
	}
	temporaryPath := temporaryFile.Name()

	_, writeError := temporaryFile.WriteString(fileContent)
	replaceError := errors.Join(writeError, temporaryFile.Close())
	if replaceError == nil {
		replaceError = os.Chmod(temporaryPath, fileInfo.Mode().Perm())
	}
	if replaceError == nil {
		replaceError = os.Rename(temporaryPath, filePath)
	}
	if replaceError != nil {
		return errors.Join(replaceError, removeTemporaryFile(temporaryPath))
	}
	return nil
}

// removeTemporaryFile deletes a temporary file left by a failed write.
func removeTemporaryFile(temporaryPath string) error {
	if removeError := os.Remove(temporaryPath); removeError != nil {
		return fmt.Errorf("removing temporary file %s: %w", temporaryPath, removeError)
	}
	return nil
}
