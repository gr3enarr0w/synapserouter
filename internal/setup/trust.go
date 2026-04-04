package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const trustConfigDir = "~/.synroute"

// TrustLevel represents the trust status of a directory
type TrustLevel int

const (
	Untrusted TrustLevel = iota
	ParentTrusted
	Trusted
)

func (t TrustLevel) String() string {
	switch t {
	case Trusted:
		return "trusted"
	case ParentTrusted:
		return "parent_trusted"
	case Untrusted:
		return "untrusted"
	default:
		return "unknown"
	}
}

// trustedDirsPath returns the path to the trusted directories JSON file
func trustedDirsPath() (string, error) {
	return expandTilde(filepath.Join(trustConfigDir, "trusted_dirs.json")), nil
}

// loadTrustedDirs loads the trusted directories from the JSON file
func loadTrustedDirs() (map[string]TrustLevel, error) {
	path, err := trustedDirsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]TrustLevel), nil
		}
		return nil, err
	}

	var trusted map[string]TrustLevel
	if err := json.Unmarshal(data, &trusted); err != nil {
		return nil, err
	}

	return trusted, nil
}

// saveTrustedDirs saves the trusted directories to the JSON file
func saveTrustedDirs(trusted map[string]TrustLevel) error {
	path, err := trustedDirsPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(trusted, "", "  ")
	if err != nil {
		return err
	}

	// Ensure config directory exists
	configDir := filepath.Dir(path)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// CheckTrust checks the trust level of a directory
// Returns the trust level (Trusted, ParentTrusted, or Untrusted)
func CheckTrust(dir string) TrustLevel {
	// Normalize the directory path
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Untrusted
	}

	trusted, err := loadTrustedDirs()
	if err != nil {
		return Untrusted
	}

	// Check if the exact directory is trusted
	if level, exists := trusted[dir]; exists {
		return level
	}

	// Check parent directories
	current := dir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root
			break
		}
		if level, exists := trusted[parent]; exists {
			if level == Trusted {
				return ParentTrusted
			}
		}
		current = parent
	}

	return Untrusted
}

// SaveTrust persists the trust level for a directory
func SaveTrust(dir string, level TrustLevel) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	trusted, err := loadTrustedDirs()
	if err != nil {
		return err
	}

	trusted[dir] = level

	return saveTrustedDirs(trusted)
}

// IsTrusted checks if a directory or any of its parents is trusted
// Returns true if the directory is Trusted or ParentTrusted
func IsTrusted(dir string) bool {
	level := CheckTrust(dir)
	return level == Trusted || level == ParentTrusted
}

// TrustDialog displays the trust dialog and handles user input
// Returns true if the directory should be trusted, false for read-only mode
func TrustDialog(dir string) (bool, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────┐")
	fmt.Println("│  Do you trust the files in this folder?                 │")
	fmt.Println("│                                                         │")
	fmt.Println("│  " + truncatePath(absDir, 54) + "  │")
	fmt.Println("│                                                         │")
	fmt.Println("│  1. Trust this folder                                   │")
	fmt.Println("│  2. Trust parent folder                                 │")
	fmt.Println("│  3. Don't trust (read-only mode)                        │")
	fmt.Println("└─────────────────────────────────────────────────────────┘")
	fmt.Print("\nEnter your choice (1-3): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	choice := strings.TrimSpace(input)

	switch choice {
	case "1":
		if err := SaveTrust(absDir, Trusted); err != nil {
			return false, fmt.Errorf("failed to save trust: %w", err)
		}
		return true, nil
	case "2":
		parentDir := filepath.Dir(absDir)
		if err := SaveTrust(parentDir, Trusted); err != nil {
			return false, fmt.Errorf("failed to save trust: %w", err)
		}
		return true, nil
	case "3":
		return false, nil
	default:
		fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
		return TrustDialog(dir)
	}
}

// truncatePath truncates a path to fit within the specified width
func truncatePath(path string, maxWidth int) string {
	if len(path) <= maxWidth {
		return padRight(path, maxWidth)
	}

	// Show beginning and end of path
	halfWidth := (maxWidth - 3) / 2
	if halfWidth < 5 {
		halfWidth = 5
	}
	start := path[:halfWidth]
	end := path[len(path)-halfWidth:]
	return padRight(start+"..."+end, maxWidth)
}

// padRight pads a string to the specified width
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
