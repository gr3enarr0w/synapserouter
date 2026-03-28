package agent

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
func NewTerminal(fd int) *Terminal {
	return &Terminal{
		fd:   fd,
		file: os.NewFile(uintptr(fd), "/dev/stdin"),
	}
}

// MakeRaw puts the terminal into raw mode and returns a restore function.
// Always call restore() when done (defer it).
func (t *Terminal) MakeRaw() (restore func(), err error) {
	oldState, err := term.MakeRaw(t.fd)
	if err != nil {
		return nil, fmt.Errorf("make raw: %w", err)
	}
	return func() { term.Restore(t.fd, oldState) }, nil
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

// OnResize calls the callback when the terminal is resized.
// Returns a stop function to cancel the listener.
func OnResize(callback func(width, height int)) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ch:
				w, h, err := term.GetSize(int(os.Stdout.Fd()))
				if err == nil {
					callback(w, h)
				}
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(ch)
		close(done)
	}
}
