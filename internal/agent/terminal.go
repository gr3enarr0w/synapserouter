package agent

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// KeyEvent represents a decoded keypress.
type KeyEvent struct {
	Rune rune
	Ctrl bool
}

// Terminal wraps raw terminal mode utilities.
type Terminal struct {
	fd   int
	file *os.File // reused for ReadKey to avoid fd leaks
}

// NewTerminal creates a terminal wrapper for the given file descriptor.
// Uses os.Stdin directly instead of os.NewFile to avoid fd ownership —
// os.NewFile creates a file that closes the fd on GC, which would kill stdin.
func NewTerminal(fd int) *Terminal {
	return &Terminal{
		fd:   fd,
		file: os.Stdin, // reuse existing file, don't create new one that owns the fd
	}
}

// MakeRaw puts the terminal into raw mode and returns a restore function.
// Always call restore() when done (defer it).
func (t *Terminal) MakeRaw() (restore func(), err error) {
	oldState, err := term.MakeRaw(t.fd)
	if err != nil {
		return nil, fmt.Errorf("make raw: %w", err)
	}
	return func() { _ = term.Restore(t.fd, oldState) }, nil
}

// GetSize returns the terminal width and height.
func (t *Terminal) GetSize() (width, height int, err error) {
	return term.GetSize(t.fd)
}

// ReadKey reads a single keypress from the terminal in raw mode.
// Decodes Ctrl-key combinations.
func (t *Terminal) ReadKey() (KeyEvent, error) {
	buf := make([]byte, 8)
	n, err := t.file.Read(buf)
	if err != nil {
		return KeyEvent{}, err
	}
	if n == 0 {
		return KeyEvent{}, fmt.Errorf("no input")
	}

	b := buf[0]

	// Ctrl-key: bytes 1-26 map to Ctrl-A through Ctrl-Z
	if b >= 1 && b <= 26 {
		return KeyEvent{Rune: rune('a' + b - 1), Ctrl: true}, nil
	}

	// Ctrl-/ sends 0x1F
	if b == 0x1F {
		return KeyEvent{Rune: '/', Ctrl: true}, nil
	}

	// Regular character
	return KeyEvent{Rune: rune(b)}, nil
}
