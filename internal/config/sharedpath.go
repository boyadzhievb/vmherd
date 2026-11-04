package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// forbiddenPathPrefixes lists system directories that shared folder host paths
// must not resolve into, not even with allow_outside_home. Read-only.
var forbiddenPathPrefixes = []string{
	"/etc", "/root", "/usr", "/bin", "/sbin", "/lib", "/lib64",
	"/sys", "/proc", "/boot", "/dev", "/run", "/var/lib",
}

// forbiddenExactPaths lists directories that must never be shared as a whole,
// although paths below them may be. $HOME is added at check time. Read-only.
var forbiddenExactPaths = []string{"/", "/home", "/Users"}

// resolveSymlinksAllowMissing resolves symlinks in an absolute path. When the
// path does not exist, it resolves the deepest existing ancestor and appends
// the missing remainder, so a deleted share folder does not break validation.
func resolveSymlinksAllowMissing(absolutePath string) (string, error) {
	existingPath := filepath.Clean(absolutePath)
	missingSuffix := ""
	for {
		resolvedPath, err := filepath.EvalSymlinks(existingPath)
		if err == nil {
			return filepath.Join(resolvedPath, missingSuffix), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("resolving symlinks in %s: %w", existingPath, err)
		}
		parentPath := filepath.Dir(existingPath)
		if parentPath == existingPath {
			return filepath.Clean(absolutePath), nil
		}
		missingSuffix = filepath.Join(filepath.Base(existingPath), missingSuffix)
		existingPath = parentPath
	}
}

// isPathWithin reports whether candidatePath equals baseDir or lies below it.
func isPathWithin(candidatePath, baseDir string) bool {
	if candidatePath == baseDir {
		return true
	}
	return strings.HasPrefix(candidatePath, strings.TrimSuffix(baseDir, "/")+"/")
}

// withResolvedForms returns each path plus its symlink-resolved form when that
// differs, so policy lists also match hosts where system directories are
// symlinks (macOS: /etc -> /private/etc).
func withResolvedForms(paths []string) ([]string, error) {
	expandedPaths := make([]string, 0, 2*len(paths))
	for _, listedPath := range paths {
		resolvedPath, err := resolveSymlinksAllowMissing(listedPath)
		if err != nil {
			return nil, err
		}
		expandedPaths = append(expandedPaths, listedPath)
		if resolvedPath != listedPath {
			expandedPaths = append(expandedPaths, resolvedPath)
		}
	}
	return expandedPaths, nil
}

// checkForbiddenHostPath rejects candidatePath if it is a directory that must
// not be shared as a whole (exactPaths) or lies in a system directory (prefixes).
func checkForbiddenHostPath(candidatePath string, exactPaths, prefixes []string) error {
	for _, exactPath := range exactPaths {
		if candidatePath == exactPath {
			return fmt.Errorf("resolves to %q, which must not be shared as a whole: share a subdirectory instead", candidatePath)
		}
	}
	for _, prefix := range prefixes {
		if isPathWithin(candidatePath, prefix) {
			return fmt.Errorf("resolves to forbidden system directory %q: choose a path outside system directories", candidatePath)
		}
	}
	return nil
}

// resolvedHomeDir returns $HOME with symlinks resolved, or "" when $HOME is
// unset (then no path counts as inside $HOME, which only narrows the allowlist).
func resolvedHomeDir() (string, error) {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return "", nil
	}
	return resolveSymlinksAllowMissing(homeDir)
}

// resolveSharedFolderHostPath turns a Herdfile host path into an absolute,
// symlink-free path and checks it against the shared-folder policy: never
// "/", $HOME itself or system directories; otherwise only inside the project
// directory or $HOME unless allowOutsideHome is set.
func resolveSharedFolderHostPath(hostPath, projectDir string, allowOutsideHome bool) (string, error) {
	absoluteHostPath := hostPath
	if !filepath.IsAbs(absoluteHostPath) {
		absoluteHostPath = filepath.Join(projectDir, hostPath)
	}
	resolvedHostPath, err := resolveSymlinksAllowMissing(absoluteHostPath)
	if err != nil {
		return "", err
	}
	resolvedProjectDir, err := resolveSymlinksAllowMissing(projectDir)
	if err != nil {
		return "", err
	}
	homeDir, err := resolvedHomeDir()
	if err != nil {
		return "", err
	}

	exactPathsToCheck := forbiddenExactPaths
	if homeDir != "" {
		exactPathsToCheck = append([]string{homeDir}, forbiddenExactPaths...)
	}
	exactPaths, err := withResolvedForms(exactPathsToCheck)
	if err != nil {
		return "", err
	}
	prefixes, err := withResolvedForms(forbiddenPathPrefixes)
	if err != nil {
		return "", err
	}
	for _, candidatePath := range []string{filepath.Clean(absoluteHostPath), resolvedHostPath} {
		if err := checkForbiddenHostPath(candidatePath, exactPaths, prefixes); err != nil {
			return "", err
		}
	}
	insideProject := isPathWithin(resolvedHostPath, resolvedProjectDir)
	insideHome := homeDir != "" && isPathWithin(resolvedHostPath, homeDir)
	if !insideProject && !insideHome && !allowOutsideHome {
		return "", fmt.Errorf("resolves to %q, which is outside the project directory and $HOME: "+
			"set 'allow_outside_home: true' on this shared_folders entry to allow it", resolvedHostPath)
	}
	return resolvedHostPath, nil
}
