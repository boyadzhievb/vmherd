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

// CheckSharedFolderHostPathsExist checks that every share's resolved host path
// of the given VMs exists and is a directory. Loading accepts missing folders,
// so up and reload call this before changing anything. Every problem, in
// Herdfile order, is returned at once.
func CheckSharedFolderHostPathsExist(vms []VM) error {
	var problems []error
	for _, vmConfig := range vms {
		for _, sharedFolder := range vmConfig.SharedFolders {
			if err := checkSharedFolderHostPathExists(vmConfig.Name, sharedFolder); err != nil {
				problems = append(problems, err)
			}
		}
	}
	return errors.Join(problems...)
}

// checkSharedFolderHostPathExists reports why one share's host path can't be
// shared: it is missing (or behind a dangling symlink), not a directory, or
// can't be inspected.
func checkSharedFolderHostPathExists(vmName string, sharedFolder SharedFolder) error {
	resolvedHostPath := sharedFolder.ResolvedHost
	fileInfo, err := os.Stat(resolvedHostPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return missingHostPathError(vmName, sharedFolder)
	case err != nil:
		return fmt.Errorf("VM %q: shared folder host path %q (%s) cannot be checked: %w — "+
			"fix its permissions or remove the share from the Herdfile", vmName, sharedFolder.Host, resolvedHostPath, err)
	case !fileInfo.IsDir():
		return fmt.Errorf("VM %q: shared folder host path %q (%s) is not a directory — 9p shares a folder; "+
			"point 'host' at a folder or remove the share", vmName, sharedFolder.Host, resolvedHostPath)
	}
	return nil
}

// missingHostPathError explains a share host path that doesn't exist: a
// dangling symlink on the path needs its target, anything else 'mkdir -p'.
func missingHostPathError(vmName string, sharedFolder SharedFolder) error {
	resolvedHostPath := sharedFolder.ResolvedHost
	linkPath, linkTarget, isDangling := findDanglingSymlink(resolvedHostPath)
	if !isDangling {
		return fmt.Errorf("VM %q: shared folder host path %q (%s) does not exist — create it with mkdir -p %s "+
			"or remove the share from the Herdfile", vmName, sharedFolder.Host, resolvedHostPath, shellQuote(resolvedHostPath))
	}
	if linkPath == resolvedHostPath {
		return fmt.Errorf("VM %q: shared folder host path %q (%s) is a symlink to missing %s — "+
			"create the target or fix the link", vmName, sharedFolder.Host, resolvedHostPath, linkTarget)
	}
	return fmt.Errorf("VM %q: shared folder host path %q (%s) does not exist: %s is a symlink to missing %s — "+
		"create the target or fix the link", vmName, sharedFolder.Host, resolvedHostPath, linkPath, linkTarget)
}

// findDanglingSymlink walks from missingPath up to the deepest element that
// exists (os.Lstat) and reports it when it is a symlink whose target is
// missing. A relative link target is returned as an absolute path. If the
// link can't be read, it reports no dangling link (the plain mkdir hint).
func findDanglingSymlink(missingPath string) (linkPath, linkTarget string, isDangling bool) {
	candidatePath := filepath.Clean(missingPath)
	for {
		fileInfo, err := os.Lstat(candidatePath)
		if err == nil {
			if fileInfo.Mode()&fs.ModeSymlink == 0 {
				return "", "", false
			}
			if _, statErr := os.Stat(candidatePath); !errors.Is(statErr, fs.ErrNotExist) {
				return "", "", false
			}
			target, readErr := os.Readlink(candidatePath)
			if readErr != nil {
				return "", "", false
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(candidatePath), target)
			}
			return candidatePath, target, true
		}
		parentPath := filepath.Dir(candidatePath)
		if parentPath == candidatePath {
			return "", "", false
		}
		candidatePath = parentPath
	}
}

// shellQuote single-quotes a path for a shell command shown to the user,
// ending the quote around each embedded single quote: close, escaped quote, reopen.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
