package ui

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestSetQuietAndIsQuiet(t *testing.T) {
	SetQuiet(false)
	if IsQuiet() {
		t.Error("expected quiet to be false")
	}

	SetQuiet(true)
	if !IsQuiet() {
		t.Error("expected quiet to be true")
	}
	SetQuiet(false)
}

func TestStdoutNormalMode(t *testing.T) {
	SetQuiet(false)
	defer SetQuiet(false)

	writer := Stdout()
	if writer != os.Stdout {
		t.Error("expected os.Stdout in normal mode")
	}
}

func TestStdoutQuietMode(t *testing.T) {
	SetQuiet(true)
	defer SetQuiet(false)

	writer := Stdout()
	if writer != io.Discard {
		t.Error("expected io.Discard in quiet mode")
	}
}

func TestStderrNormalMode(t *testing.T) {
	SetQuiet(false)
	defer SetQuiet(false)

	writer := Stderr()
	if writer != os.Stderr {
		t.Error("expected os.Stderr in normal mode")
	}
}

func TestStderrQuietMode(t *testing.T) {
	SetQuiet(true)
	defer SetQuiet(false)

	writer := Stderr()
	if writer != io.Discard {
		t.Error("expected io.Discard in quiet mode")
	}
}

func TestPrintfNormalMode(t *testing.T) {
	SetQuiet(false)
	defer SetQuiet(false)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Printf("hello %s", "world")

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if buf.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", buf.String())
	}
}

func TestPrintfQuietMode(t *testing.T) {
	SetQuiet(true)
	defer SetQuiet(false)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Printf("should not appear")

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if buf.String() != "" {
		t.Errorf("expected empty output in quiet mode, got %q", buf.String())
	}
}

func TestPrintlnNormalMode(t *testing.T) {
	SetQuiet(false)
	defer SetQuiet(false)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Println("hello")

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if buf.String() != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", buf.String())
	}
}

func TestPrintlnQuietMode(t *testing.T) {
	SetQuiet(true)
	defer SetQuiet(false)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	Println("should not appear")

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	if buf.String() != "" {
		t.Errorf("expected empty output in quiet mode, got %q", buf.String())
	}
}
