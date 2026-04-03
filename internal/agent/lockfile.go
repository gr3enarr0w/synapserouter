package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const lockFileName = ".synroute.lock"

// AcquireLock creates a lock file with the current PID.
// Returns the lock file path and an error if the lock already exists with an alive PID.
func AcquireLock(workDir string) (string, error) {
	lockPath := filepath.Join(workDir, lockFileName)
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)

	// Check if lock exists
	if _, err := os.Stat(lockPath); err == nil {
		// Lock exists, check if PID is alive
		existingPID, err := os.ReadFile(lockPath)
		if err == nil {
			existingPIDInt, err := strconv.Atoi(string(existingPID))
			if err == nil && isProcessAlive(existingPIDInt) {
				return lockPath, fmt.Errorf("lock held by alive process %d", existingPIDInt)
			}
		}
		// Lock is stale, will overwrite
	}

	// Create lock file with PID
	if err := os.WriteFile(lockPath, []byte(pidStr), 0644); err != nil {
		return "", fmt.Errorf("failed to create lock file: %w", err)
	}

	return lockPath, nil
}

// ReleaseLock removes the lock file.
func ReleaseLock(lockPath string) error {
	if lockPath == "" {
		return nil
	}
	return os.Remove(lockPath)
}

// IsLockHeld checks if a lock file exists and is held by an alive process.
func IsLockHeld(workDir string) (bool, int, error) {
	lockPath := filepath.Join(workDir, lockFileName)
	
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return false, 0, fmt.Errorf("invalid lock file content: %w", err)
	}

	if isProcessAlive(pid) {
		return true, pid, nil
	}

	// Lock exists but process is dead - stale lock
	return false, pid, nil
}

// isProcessAlive checks if a process with the given PID is running.
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix-like systems, FindProcess always succeeds.
	// We need to send signal 0 to check if process exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
