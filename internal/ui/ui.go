package ui

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

var quiet atomic.Bool

// SetQuiet enables or disables quiet mode.
func SetQuiet(q bool) {
	quiet.Store(q)
}

// IsQuiet returns whether quiet mode is enabled.
func IsQuiet() bool {
	return quiet.Load()
}

// Printf prints a formatted message unless quiet mode is on.
func Printf(format string, args ...any) {
	if !quiet.Load() {
		fmt.Printf(format, args...)
	}
}

// Println prints a message unless quiet mode is on.
func Println(args ...any) {
	if !quiet.Load() {
		fmt.Println(args...)
	}
}

// Stdout returns os.Stdout in normal mode, or a discard writer in quiet mode.
func Stdout() io.Writer {
	if quiet.Load() {
		return io.Discard
	}
	return os.Stdout
}

// Stderr returns os.Stderr in normal mode, or a discard writer in quiet mode.
func Stderr() io.Writer {
	if quiet.Load() {
		return io.Discard
	}
	return os.Stderr
}
