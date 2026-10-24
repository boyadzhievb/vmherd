package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPublicKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	pubKeyPath := keyPath + ".pub"

	pubContent := "ssh-ed25519 AAAA testcomment\n"
	if err := os.WriteFile(pubKeyPath, []byte(pubContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadPublicKey(keyPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != pubContent {
		t.Errorf("expected %q, got %q", pubContent, result)
	}
}

func TestReadPublicKeyMissingFile(t *testing.T) {
	_, err := ReadPublicKey("/nonexistent/path/id_ed25519")
	if err == nil {
		t.Fatal("expected error for missing public key file")
	}
}

func TestEnsureKeyPairCreatesNewKey(t *testing.T) {
	dir := t.TempDir()

	keyPath, err := EnsureKeyPair(dir, "test-comment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("private key not created: %v", err)
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Errorf("public key not created: %v", err)
	}

	pubKey, err := ReadPublicKey(keyPath)
	if err != nil {
		t.Fatalf("cannot read generated public key: %v", err)
	}
	if !strings.HasPrefix(pubKey, "ssh-ed25519 ") {
		t.Errorf("expected ed25519 public key, got: %s", pubKey)
	}
}

func TestEnsureKeyPairIdempotent(t *testing.T) {
	dir := t.TempDir()

	firstPath, err := EnsureKeyPair(dir, "test-comment")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	firstPub, err := ReadPublicKey(firstPath)
	if err != nil {
		t.Fatal(err)
	}

	secondPath, err := EnsureKeyPair(dir, "test-comment")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	secondPub, err := ReadPublicKey(secondPath)
	if err != nil {
		t.Fatal(err)
	}

	if firstPath != secondPath {
		t.Errorf("paths differ: %s vs %s", firstPath, secondPath)
	}
	if firstPub != secondPub {
		t.Errorf("key changed between calls — EnsureKeyPair should be idempotent")
	}
}
