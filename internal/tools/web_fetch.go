package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	defaultFetchTimeout = 30 * time.Second
	maxFetchBytes       = 5 << 20 // 5MB raw HTML limit
	maxOutputChars      = 50000   // truncate extracted text at 50K chars
)

// WebFetchTool fetches a URL and returns the content as plain text.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string     { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "Fetch a web page by URL and return its content as plain text (HTML stripped)"
}
func (t *WebFetchTool) Category() ToolCategory { return CategoryReadOnly }

func (t *WebFetchTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch",
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	rawURL := stringArg(args, "url")
	if rawURL == "" {
		return &ToolResult{Error: "url is required"}, nil
	}

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	ctx, cancel := context.WithTimeout(ctx, defaultFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("invalid URL: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SynapseRouter/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("fetch failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ToolResult{Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("reading response: %v", err)}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	var text string
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		text = htmlToText(string(body))
	} else {
		text = string(body)
	}

	if len(text) > maxOutputChars {
		text = text[:maxOutputChars] + "\n\n[content truncated at 50,000 characters]"
	}

	if strings.TrimSpace(text) == "" {
		return &ToolResult{Output: "(page returned no extractable text)"}, nil
	}

	return &ToolResult{Output: text}, nil
}

// Regex patterns for HTML-to-text conversion (compiled once).
var (
	reScript     = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle      = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reNav        = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	reFooter     = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	reComment    = regexp.MustCompile(`(?s)<!--.*?-->`)
	reBlockBreak = regexp.MustCompile(`(?i)</?(p|div|br|h[1-6]|li|tr|blockquote|pre|hr|section|article|header|main)[^>]*>`)
	reAllTags    = regexp.MustCompile(`<[^>]*>`)
	reMultiNL    = regexp.MustCompile(`\n{3,}`)
	reMultiSpace = regexp.MustCompile(`[ \t]+`)
)

// htmlToText converts HTML to readable plain text by removing tags and normalizing whitespace.
func htmlToText(html string) string {
	// Remove non-content elements
	s := reScript.ReplaceAllString(html, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reNav.ReplaceAllString(s, "")
	s = reFooter.ReplaceAllString(s, "")
	s = reComment.ReplaceAllString(s, "")

	// Convert block-level elements to newlines
	s = reBlockBreak.ReplaceAllString(s, "\n")

	// Strip remaining tags
	s = reAllTags.ReplaceAllString(s, "")

	// Decode common HTML entities
	for entity, replacement := range htmlEntityMap {
		s = strings.ReplaceAll(s, entity, replacement)
	}

	// Normalize whitespace
	s = reMultiSpace.ReplaceAllString(s, " ")
	// Trim each line
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	s = strings.Join(lines, "\n")
	s = reMultiNL.ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}
