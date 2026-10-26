package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// EnsureKeyPair generates an ed25519 SSH key pair if it doesn't already exist.
// Returns the path to the private key.
func EnsureKeyPair(stateDir, comment string) (string, error) {
	keyPath := filepath.Join(stateDir, "id_ed25519")

	if _, err := os.Stat(keyPath); err == nil {
		return keyPath, nil
	}

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create state directory %s: %w", stateDir, err)
	}

	// Tighten permissions even if the directory already existed with a looser mode.
	if err := os.Chmod(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("cannot set permissions on state directory %s: %w", stateDir, err)
	}

	cmd := exec.Command("ssh-keygen",
		"-t", "ed25519",
		"-f", keyPath,
		"-N", "",
		"-C", comment,
		"-q",
	)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cannot generate SSH key: %w", err)
	}

	// Explicitly set private key permissions to owner-only, in case umask is too permissive.
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return "", fmt.Errorf("cannot set permissions on private key %s: %w", keyPath, err)
	}

	ui.Printf("SSH key generated: %s\n", keyPath)
	return keyPath, nil
}

// ReadPublicKey reads the public key associated with the private key at keyPath.
func ReadPublicKey(keyPath string) (string, error) {
	pubKeyPath := keyPath + ".pub"
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", fmt.Errorf("cannot read SSH public key %s: %w", pubKeyPath, err)
	}
	return string(data), nil
}
