package agent

import (
	"os/exec"
	"strings"
)

// extractPDFText extracts text from a PDF file using pdftotext.
// Returns empty string and error if pdftotext is not installed.
// Install: brew install poppler (macOS) or apt install poppler-utils (Linux).
func extractPDFText(path string) (string, error) {
	cmd := exec.Command("pdftotext", "-layout", path, "-")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
