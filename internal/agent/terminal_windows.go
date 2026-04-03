//go:build windows

package agent

// OnResize is a no-op on Windows as SIGWINCH is not supported.
// Returns a no-op stop function.
func OnResize(callback func(width, height int)) func() {
	return func() {}
}
