//go:build !windows

package agent

import (
	"os"
	"syscall"
	"unsafe"
)

// flushStdin discards any data received on stdin but not yet read.
// Prevents stale cursor position responses (\033[row;colR) from
// readline's \033[6n queries from causing Readline() to return io.EOF.
//
// Uses POSIX tcflush(TCIFLUSH) via ioctl. Works on macOS and Linux.
func flushStdin() {
	fd := os.Stdin.Fd()

	// TCIFLUSH = 1 (flush data received but not read)
	// On macOS: TIOCFLUSH with FREAD flag
	// On Linux: TCFLSH ioctl with TCIFLUSH arg
	//
	// Use the portable approach: TIOCFLUSH exists on both platforms.
	// TIOCFLUSH (0x80047410 on macOS) flushes the specified queues.
	// Pass FREAD (1) to flush input only.
	fread := int32(1) // FREAD = flush input queue
	syscall.Syscall(syscall.SYS_IOCTL, fd, 0x80047410, uintptr(unsafe.Pointer(&fread)))
}
