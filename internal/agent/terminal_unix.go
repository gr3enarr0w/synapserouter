//go:build !windows

package agent

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// OnResize calls the callback when the terminal is resized (Unix-only).
// Returns a stop function to cancel the listener.
func OnResize(callback func(width, height int)) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ch:
				w, h, err := term.GetSize(int(os.Stdout.Fd())) //nolint:G115 // os.Stdout.Fd() always fits in int on supported platforms
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
