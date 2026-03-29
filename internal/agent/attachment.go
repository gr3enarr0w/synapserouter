package agent

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxAttachmentSize is the maximum file size to include inline.
// Files larger than this are truncated with a hint to use file_read.
const maxAttachmentSize = 10 * 1024 // 10KB

// Attachment represents a file attached to a user message.
type Attachment struct {
	Path     string // absolute path to the file
	Content  string // file content (possibly truncated)
	MimeType string // detected MIME type
	Truncated bool  // true if content was truncated
	Error    string // non-empty if file could not be read
}

// atReferencePattern matches @filename patterns. Supports:
//   - @filename.ext (simple filename in current dir)
//   - @path/to/file.ext (relative path)
//   - @./path/to/file.ext (explicit relative)
//   - @~/path/to/file.ext (home-relative)
//
// Does NOT match @ in email addresses (user@domain.com) because
// the character before @ must be whitespace or start-of-string,
// and the path must contain a dot in the filename portion or a slash.
var atReferencePattern = regexp.MustCompile(`(?:^|(?:\s))@((?:[~.]?/)?[^\s@]+\.[^\s@]+)`)

// absolutePathPattern matches absolute paths like /path/to/file.ext
// that appear as standalone tokens in the message. Must contain at least
// one slash after the root and end with a filename (contains a dot).
var absolutePathPattern = regexp.MustCompile(`(?:^|\s)(/(?:[^\s]+/)*[^\s/]+\.[^\s]+)`)

// ParseAttachments scans a user message for file references (@file or absolute paths),
// reads each referenced file, and returns a cleaned message plus attachment metadata.
// The cleaned message has @references removed but absolute paths are preserved
// (since they may be intentional in the message text).
func ParseAttachments(message string, workDir string) (string, []Attachment) {
	seen := make(map[string]bool)
	var attachments []Attachment

	// Handle @dir/ references — expand directories into individual file attachments
	message, dirAttachments := expandDirReferences(message, workDir)
	for _, att := range dirAttachments {
		if !seen[att.Path] {
			seen[att.Path] = true
			attachments = append(attachments, att)
		}
	}

	// Find @references
	cleanedMessage := message
	atMatches := atReferencePattern.FindAllStringSubmatchIndex(message, -1)
	// Process in reverse order so index positions remain valid after replacement
	for i := len(atMatches) - 1; i >= 0; i-- {
		match := atMatches[i]
		// match[2]:match[3] is the capture group (the path without @)
		refPath := message[match[2]:match[3]]
		absPath := resolveAttachmentPath(refPath, workDir)

		if seen[absPath] {
			// Still remove the duplicate reference from the message
			atStart := match[2] - 1 // include the @ character
			// Find the @ character position: it's just before the capture group
			// but we need to account for possible leading whitespace in the match
			fullMatch := message[match[0]:match[1]]
			atIdx := strings.Index(fullMatch, "@")
			if atIdx >= 0 {
				atStart = match[0] + atIdx
			}
			cleanedMessage = cleanedMessage[:atStart] + cleanedMessage[match[3]:]
			continue
		}
		seen[absPath] = true

		att := readAttachment(absPath)
		attachments = append(attachments, att)

		// Remove the @reference from the message
		atStart := match[0]
		fullMatch := message[match[0]:match[1]]
		atIdx := strings.Index(fullMatch, "@")
		if atIdx >= 0 {
			atStart = match[0] + atIdx
		}
		cleanedMessage = cleanedMessage[:atStart] + cleanedMessage[match[3]:]
	}

	// Find absolute paths (these are NOT removed from the message)
	absMatches := absolutePathPattern.FindAllStringSubmatch(message, -1)
	for _, m := range absMatches {
		absPath := filepath.Clean(m[1])
		if seen[absPath] {
			continue
		}
		// Only attach if the file actually exists
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		seen[absPath] = true
		attachments = append(attachments, readAttachment(absPath))
	}

	// Clean up extra whitespace from removals
	cleanedMessage = strings.TrimSpace(cleanedMessage)
	// Collapse multiple spaces into one
	for strings.Contains(cleanedMessage, "  ") {
		cleanedMessage = strings.ReplaceAll(cleanedMessage, "  ", " ")
	}

	return cleanedMessage, attachments
}

// resolveAttachmentPath resolves a file reference to an absolute path.
func resolveAttachmentPath(ref string, workDir string) string {
	if strings.HasPrefix(ref, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Clean(filepath.Join(home, ref[2:]))
		}
	}
	if filepath.IsAbs(ref) {
		return filepath.Clean(ref)
	}
	return filepath.Clean(filepath.Join(workDir, ref))
}

// readAttachment reads a file and creates an Attachment.
func readAttachment(absPath string) Attachment {
	att := Attachment{Path: absPath}

	info, err := os.Stat(absPath)
	if err != nil {
		att.Error = fmt.Sprintf("cannot read file: %v", err)
		return att
	}

	if info.IsDir() {
		att.Error = "path is a directory — use @dir/ to attach all files"
		return att
	}

	// Read the file (up to maxAttachmentSize + 1 byte to detect truncation)
	f, err := os.Open(absPath)
	if err != nil {
		att.Error = fmt.Sprintf("cannot open file: %v", err)
		return att
	}
	defer f.Close()

	buf := make([]byte, maxAttachmentSize+1)
	n, _ := f.Read(buf)
	buf = buf[:n]

	// Detect MIME type from content
	att.MimeType = detectMimeType(absPath, buf)

	// Check for binary content
	if isBinaryContent(buf) {
		att.Content = "[binary file, use appropriate tool to process]"
		return att
	}

	// Check for truncation
	if info.Size() > maxAttachmentSize {
		att.Truncated = true
		// Ensure we don't cut in the middle of a UTF-8 character
		content := string(buf[:maxAttachmentSize])
		for !utf8.ValidString(content) && len(content) > 0 {
			content = content[:len(content)-1]
		}
		att.Content = content + "\n\n[truncated at 10KB, use file_read for full content]"
	} else {
		att.Content = string(buf)
	}

	return att
}

// detectMimeType returns a MIME type for the file based on extension and content.
func detectMimeType(path string, content []byte) string {
	// Try extension-based detection first for common code files
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".rs":
		return "text/x-rust"
	case ".java":
		return "text/x-java"
	case ".rb":
		return "text/x-ruby"
	case ".c", ".h":
		return "text/x-c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "text/x-c++"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".toml":
		return "text/toml"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".sh", ".bash", ".zsh":
		return "text/x-shellscript"
	case ".sql":
		return "text/x-sql"
	case ".txt":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".proto":
		return "text/x-protobuf"
	case ".dockerfile":
		return "text/x-dockerfile"
	}

	// Dockerfile without extension
	base := filepath.Base(path)
	if strings.EqualFold(base, "Dockerfile") || strings.HasPrefix(base, "Dockerfile.") {
		return "text/x-dockerfile"
	}
	if strings.EqualFold(base, "Makefile") || strings.EqualFold(base, "GNUmakefile") {
		return "text/x-makefile"
	}

	// Fall back to content-based detection
	if len(content) > 0 {
		detected := http.DetectContentType(content)
		return detected
	}

	return "application/octet-stream"
}

// isBinaryContent checks if content appears to be binary by looking for null bytes
// and checking the ratio of non-printable characters.
func isBinaryContent(content []byte) bool {
	if len(content) == 0 {
		return false
	}

	// Check first 512 bytes (or less)
	checkLen := len(content)
	if checkLen > 512 {
		checkLen = 512
	}
	sample := content[:checkLen]

	nonPrintable := 0
	for _, b := range sample {
		if b == 0 {
			return true // null byte is a strong binary indicator
		}
		// Count non-printable, non-whitespace bytes
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}

	// If more than 10% non-printable, treat as binary
	return float64(nonPrintable)/float64(checkLen) > 0.1
}

// maxDirFiles limits how many files are attached from a single @dir/ reference.
const maxDirFiles = 50

// dirRefPattern matches @dir/ references (paths ending with / followed by whitespace or EOL).
// Must not match @~/file.go or @./path/file.ext — only paths where the last char before
// whitespace/EOL is a slash.
var dirRefPattern = regexp.MustCompile(`(?:^|\s)@((?:[~.]?/)?[^\s@]*/)(?:\s|$)`)

// expandDirReferences finds @dir/ references, expands them to individual files,
// and returns the cleaned message plus the file attachments.
func expandDirReferences(message string, workDir string) (string, []Attachment) {
	matches := dirRefPattern.FindAllStringSubmatchIndex(message, -1)
	if len(matches) == 0 {
		return message, nil
	}

	var attachments []Attachment
	cleaned := message

	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		dirRef := message[match[2]:match[3]]
		absDir := resolveAttachmentPath(dirRef, workDir)

		info, err := os.Stat(absDir)
		if err != nil || !info.IsDir() {
			continue
		}

		// Walk directory and collect files (non-recursive, skip hidden)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}

		count := 0
		for _, entry := range entries {
			if count >= maxDirFiles {
				break
			}
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			filePath := filepath.Join(absDir, entry.Name())
			attachments = append(attachments, readAttachment(filePath))
			count++
		}

		// Remove the @dir/ reference from the message
		fullMatch := message[match[0]:match[1]]
		atIdx := strings.Index(fullMatch, "@")
		if atIdx >= 0 {
			removeStart := match[0] + atIdx
			cleaned = cleaned[:removeStart] + cleaned[match[3]:]
		}
	}

	return cleaned, attachments
}

// FormatAttachments formats a list of attachments for injection into the conversation.
func FormatAttachments(attachments []Attachment) string {
	if len(attachments) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n--- Attached Files ---\n")
	for _, att := range attachments {
		b.WriteString(fmt.Sprintf("\n### %s", att.Path))
		if att.MimeType != "" {
			b.WriteString(fmt.Sprintf(" (%s)", att.MimeType))
		}
		b.WriteString("\n")

		if att.Error != "" {
			b.WriteString(fmt.Sprintf("[error: %s]\n", att.Error))
			continue
		}

		b.WriteString("```\n")
		b.WriteString(att.Content)
		if !strings.HasSuffix(att.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
	b.WriteString("--- End Attached Files ---")

	return b.String()
}
