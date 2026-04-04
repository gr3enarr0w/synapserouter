package tools

import (
	"log"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/security"
)

// SearchResultWithScore extends SearchResult with a quality score
type SearchResultWithScore struct {
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	Snippet      string  `json:"snippet"`
	QualityScore float64 `json:"quality_score"` // 0-1 score
}

// ResultFilter filters search results for sensitive data and quality
type ResultFilter struct {
	redactor         *security.Redactor
	internalURLRegex *regexp.Regexp
	credentialRegex  *regexp.Regexp
	goodDomains      map[string]float64 // domain -> base score
}

// NewResultFilter creates a new ResultFilter with default patterns
func NewResultFilter(redactor *security.Redactor) *ResultFilter {
	// Internal/private URL patterns
	internalPatterns := []string{
		`\.internal\b`,
		`\.local\b`,
		`\.corp\b`,
		`\.private\b`,
		`\.lan\b`,
		`localhost`,
		`127\.0\.0\.1`,
		`::1`,
	}

	// Credential patterns
	credentialPatterns := []string{
		// Database connection strings
		`(?i)(postgres|postgresql|mysql|mongodb|redis|amqp|sqlserver)://[^\s"']+`,
		// AWS keys
		`(?i)(AKIA|ABIA|ACCA|ASIA)[A-Z0-9]{16}`,
		// Generic API keys
		`(?i)api[_-]?key\s*[=:]\s*['"]?[a-zA-Z0-9]{20,}['"]?`,
		// Password in URL
		`://[^:]+:[^@]+@`,
		// .env style variables
		`(?i)(SECRET|PASSWORD|TOKEN|KEY|CREDENTIAL)[_\s]*[=:]\s*['"]?[^\s'"]{8,}['"]?`,
		// Private keys
		`-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`,
		// JWT tokens
		`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`,
	}

	// Combine patterns
	internalRegex := regexp.MustCompile(strings.Join(internalPatterns, "|"))
	credentialRegex := regexp.MustCompile(strings.Join(credentialPatterns, "|"))

	// Known good domains with base quality scores
	goodDomains := map[string]float64{
		"github.com":          0.95,
		"stackoverflow.com":   0.95,
		"docs.google.com":     0.90,
		"medium.com":          0.85,
		"dev.to":              0.85,
		"reddit.com":          0.75,
		"youtube.com":         0.80,
		"twitter.com":         0.70,
		"linkedin.com":        0.75,
		"microsoft.com":       0.90,
		"apple.com":           0.90,
		"amazon.com":          0.85,
		"cloudflare.com":      0.90,
		"aws.amazon.com":      0.95,
		"azure.microsoft.com": 0.95,
		"google.com":          0.90,
		"mozilla.org":         0.95,
		"apache.org":          0.95,
		"python.org":          0.95,
		"golang.org":          0.95,
		"rust-lang.org":       0.95,
		"kubernetes.io":       0.95,
		"docker.com":          0.90,
		"npmjs.com":           0.90,
		"pypi.org":            0.95,
		"rubygems.org":        0.95,
		"oracle.com":          0.85,
		"redhat.com":          0.90,
		"ubuntu.com":          0.90,
		"debian.org":          0.90,
		"nginx.com":           0.90,
		"postgresql.org":      0.95,
		"mysql.com":           0.90,
		"mongodb.com":         0.90,
		"elastic.co":          0.90,
		"grafana.com":         0.85,
		"prometheus.io":       0.90,
		"terraform.io":        0.90,
		"ansible.com":         0.85,
		"jenkins.io":          0.85,
		"gitlab.com":          0.90,
		"bitbucket.org":       0.85,
		"atlassian.com":       0.85,
		"jira.com":            0.80,
		"confluence.com":      0.80,
		"notion.so":           0.80,
		"slack.com":           0.80,
		"discord.com":         0.75,
		"telegram.org":        0.75,
		"whatsapp.com":        0.75,
		"zoom.us":             0.80,
		"meet.google.com":     0.85,
		"teams.microsoft.com": 0.85,
		"webex.com":           0.80,
		"dropbox.com":         0.85,
		"box.com":             0.85,
		"icloud.com":          0.85,
		"protonmail.com":      0.85,
		"fastmail.com":        0.85,
		"mailgun.com":         0.85,
		"sendgrid.com":        0.85,
		"twilio.com":          0.90,
		"stripe.com":          0.90,
		"paypal.com":          0.85,
		"square.com":          0.85,
		"shopify.com":         0.85,
		"wordpress.com":       0.80,
		"wix.com":             0.75,
		"squarespace.com":     0.80,
		"godaddy.com":         0.75,
		"namecheap.com":       0.80,
		"akamai.com":          0.85,
		"fastly.com":          0.85,
	}

	if redactor == nil {
		redactor = security.NewRedactor()
	}

	return &ResultFilter{
		redactor:         redactor,
		internalURLRegex: internalRegex,
		credentialRegex:  credentialRegex,
		goodDomains:      goodDomains,
	}
}

// FilterResult filters a single search result
// Returns the filtered result (with PII redacted) and a boolean indicating if it should be excluded
func (rf *ResultFilter) FilterResult(result SearchResult, query string) (*SearchResultWithScore, bool) {
	// Check for internal/private URLs
	if rf.isInternalURL(result.URL) {
		return nil, true
	}

	// Check for credential patterns in URL, title, or snippet
	if rf.containsCredentials(result.URL) || rf.containsCredentials(result.Title) || rf.containsCredentials(result.Snippet) {
		return nil, true
	}

	// Redact PII from title and snippet
	redactedTitle := rf.redactor.Redact(result.Title).Text
	redactedSnippet := rf.redactor.Redact(result.Snippet).Text

	// Calculate quality score
	qualityScore := rf.calculateQualityScore(result, query)

	return &SearchResultWithScore{
		Title:        redactedTitle,
		URL:          result.URL,
		Snippet:      redactedSnippet,
		QualityScore: qualityScore,
	}, false
}

// Filter filters a slice of search results
// Returns filtered results and count of excluded results
func (rf *ResultFilter) Filter(results []SearchResult, query string) ([]SearchResultWithScore, int) {
	var filtered []SearchResultWithScore
	excludedCount := 0

	for _, result := range results {
		scoredResult, excluded := rf.FilterResult(result, query)
		if excluded {
			excludedCount++
		} else if scoredResult != nil {
			filtered = append(filtered, *scoredResult)
		}
	}

	if excludedCount > 0 {
		log.Printf("[Search] filtered %d results containing sensitive data", excludedCount)
	}

	return filtered, excludedCount
}

// isInternalURL checks if a URL points to an internal/private resource
func (rf *ResultFilter) isInternalURL(rawURL string) bool {
	// Quick regex check first
	if rf.internalURLRegex.MatchString(rawURL) {
		return true
	}

	// Parse URL and check IP addresses
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := parsed.Hostname()
	if host == "" {
		return false
	}

	// Check if it's an IP address
	ip := net.ParseIP(host)
	if ip != nil {
		return rf.isPrivateIP(ip)
	}

	return false
}

// isPrivateIP checks if an IP address is in a private range
func (rf *ResultFilter) isPrivateIP(ip net.IP) bool {
	// RFC 1918 private networks
	privateNetworks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}

	for _, cidr := range privateNetworks {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// containsCredentials checks if text contains credential patterns
func (rf *ResultFilter) containsCredentials(text string) bool {
	return rf.credentialRegex.MatchString(text)
}

// calculateQualityScore calculates a quality score (0-1) for a search result
func (rf *ResultFilter) calculateQualityScore(result SearchResult, query string) float64 {
	score := 0.5 // Base score

	// Domain authority (0.0 - 0.3)
	domainScore := rf.getDomainScore(result.URL)
	score += domainScore * 0.3

	// Query relevance (0.0 - 0.2)
	relevanceScore := rf.getRelevanceScore(result, query)
	score += relevanceScore * 0.2

	// Recency bonus if date is available in snippet (0.0 - 0.2)
	recencyScore := rf.getRecencyScore(result.Snippet)
	score += recencyScore * 0.2

	// URL structure quality (0.0 - 0.1)
	urlScore := rf.getURLScore(result.URL)
	score += urlScore * 0.1

	// Clamp to 0-1
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// getDomainScore returns a score based on known good domains
func (rf *ResultFilter) getDomainScore(rawURL string) float64 {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0.0
	}

	host := strings.ToLower(parsed.Hostname())

	// Remove www. prefix
	host = strings.TrimPrefix(host, "www.")

	// Check for exact match
	if score, ok := rf.goodDomains[host]; ok {
		return score
	}

	// Check for domain suffix match
	for domain, score := range rf.goodDomains {
		if strings.HasSuffix(host, "."+domain) {
			return score * 0.9 // Slightly lower for subdomains
		}
	}

	// Check TLD for general quality signals
	if strings.HasSuffix(host, ".edu") || strings.HasSuffix(host, ".gov") {
		return 0.85
	}
	if strings.HasSuffix(host, ".org") {
		return 0.75
	}

	// Unknown domain - neutral score
	return 0.5
}

// getRelevanceScore calculates how relevant the result is to the query
func (rf *ResultFilter) getRelevanceScore(result SearchResult, query string) float64 {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(result.Title)
	snippetLower := strings.ToLower(result.Snippet)
	urlLower := strings.ToLower(result.URL)

	// Extract query terms
	queryTerms := strings.Fields(queryLower)
	if len(queryTerms) == 0 {
		return 0.5
	}

	matchCount := 0
	totalTerms := len(queryTerms)

	for _, term := range queryTerms {
		if len(term) < 3 {
			totalTerms--
			continue
		}
		if strings.Contains(titleLower, term) {
			matchCount += 2 // Title matches are more important
		}
		if strings.Contains(snippetLower, term) {
			matchCount++
		}
		if strings.Contains(urlLower, term) {
			matchCount++
		}
	}

	if totalTerms == 0 {
		return 0.5
	}

	// Normalize to 0-1
	relevance := float64(matchCount) / float64(totalTerms*3) // Max 3 matches per term
	if relevance > 1.0 {
		relevance = 1.0
	}

	return relevance
}

// getRecencyScore checks for date patterns in snippet and scores recency
func (rf *ResultFilter) getRecencyScore(snippet string) float64 {
	// Look for year patterns
	yearPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\b(2025|2026)\b`),
		regexp.MustCompile(`\b(2024)\b`),
		regexp.MustCompile(`\b(2023)\b`),
		regexp.MustCompile(`\b(2022)\b`),
		regexp.MustCompile(`\b(2021)\b`),
	}

	for i, pattern := range yearPatterns {
		if pattern.MatchString(snippet) {
			// More recent years get higher scores
			score := 1.0 - (float64(i) * 0.2)
			if score < 0.0 {
				score = 0.0
			}
			return score
		}
	}

	// Check for relative time patterns
	relativePatterns := []string{
		"today", "yesterday", "this week", "this month",
		"last week", "last month", "recent", "new",
		"updated", "published", "posted",
	}

	snippetLower := strings.ToLower(snippet)
	for _, pattern := range relativePatterns {
		if strings.Contains(snippetLower, pattern) {
			return 0.8
		}
	}

	// No date information - neutral score
	return 0.5
}

// getURLScore evaluates URL structure quality
func (rf *ResultFilter) getURLScore(rawURL string) float64 {
	score := 0.5

	// Prefer HTTPS
	if strings.HasPrefix(rawURL, "https://") {
		score += 0.3
	}

	// Prefer shorter, cleaner URLs
	parsed, err := url.Parse(rawURL)
	if err == nil {
		path := parsed.Path
		// Penalize very long paths
		if len(path) > 200 {
			score -= 0.2
		}
		// Penalize URLs with many query parameters
		if len(parsed.RawQuery) > 100 {
			score -= 0.1
		}
		// Bonus for documentation-like paths
		if strings.Contains(path, "/docs/") || strings.Contains(path, "/doc/") ||
			strings.Contains(path, "/guide/") || strings.Contains(path, "/tutorial/") {
			score += 0.2
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}
