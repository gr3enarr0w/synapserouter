package router

import (
	"log"
	"os"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/orchestration"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

// ConversationSignals holds detected patterns from analyzing conversation messages.
type ConversationSignals struct {
	DetectedLanguages []string
	HasCodeChanges    bool
	HasErrors         bool
	ErrorLoopDetected bool
	RepeatedErrors    []string
	SecuritySurface   bool
	ResearchNeeded    bool
	ToolCallFailures  int
	ValidationErrors  bool
}

// SkillInjection represents a single piece of skill context to inject.
type SkillInjection struct {
	Source    string   // "skill-match", "error-loop", "post-change", "research-hint", "validation-error"
	SkillName string
	Content   string
	MCPTools  []string
}

// PreprocessResult holds the full result of conversation preprocessing.
type PreprocessResult struct {
	Signals    ConversationSignals
	Injections []SkillInjection
	Skipped    bool
}

const maxInjectionTokens = 500

// PreprocessRequest analyzes conversation messages and returns skill context injections.
func (r *Router) PreprocessRequest(req *providers.ChatRequest) *PreprocessResult {
	if req.SkipSkillPreprocess || req.SkipMemory {
		return &PreprocessResult{Skipped: true}
	}
	if !skillPreprocessEnabled() {
		return &PreprocessResult{Skipped: true}
	}
	if len(req.Messages) == 0 {
		return &PreprocessResult{Skipped: true}
	}

	signals := analyzeConversation(req.Messages)
	injections := buildInjections(signals, orchestration.DefaultSkills())

	if len(injections) > 0 {
		injectSkillContext(req, injections)
		log.Printf("[Router] Skill preprocessor: %d injections (%v languages, errorLoop=%v, codeChanges=%v, security=%v, research=%v, validation=%v)",
			len(injections), signals.DetectedLanguages, signals.ErrorLoopDetected,
			signals.HasCodeChanges, signals.SecuritySurface, signals.ResearchNeeded, signals.ValidationErrors)
	}

	return &PreprocessResult{
		Signals:    signals,
		Injections: injections,
	}
}

func skillPreprocessEnabled() bool {
	v := os.Getenv("SKILL_PREPROCESS")
	if v == "" {
		return true // default on
	}
	return strings.EqualFold(v, "true") || v == "1"
}

// analyzeConversation scans messages for signals that indicate skill context would help.
func analyzeConversation(messages []providers.Message) ConversationSignals {
	var signals ConversationSignals

	signals.DetectedLanguages = detectLanguages(messages)
	signals.HasCodeChanges = detectCodeChanges(messages)
	signals.SecuritySurface = detectSecuritySurface(messages)
	signals.ResearchNeeded = detectResearchNeeds(messages)

	loopDetected, repeatedErrors, isValidation := detectErrorLoop(messages)
	signals.ErrorLoopDetected = loopDetected
	signals.RepeatedErrors = repeatedErrors
	signals.HasErrors = len(repeatedErrors) > 0 || loopDetected
	signals.ValidationErrors = isValidation

	// Count tool call failures in assistant messages
	for _, msg := range messages {
		if msg.Role == "tool" && containsErrorIndicator(msg.Content) {
			signals.ToolCallFailures++
		}
	}

	return signals
}

// buildInjections converts detected signals into skill context injections.
func buildInjections(signals ConversationSignals, registry []orchestration.Skill) []SkillInjection {
	var injections []SkillInjection
	budgetUsed := 0

	// Error loop / validation error — highest priority
	if signals.ErrorLoopDetected {
		if signals.ValidationErrors {
			inj := SkillInjection{
				Source:  "validation-error",
				Content: "[API Docs] A validation error indicates incorrect API parameters. Look up the API documentation (context7.query-docs, WebFetch) for correct parameter limits and constraints BEFORE trying different values.",
			}
			budgetUsed += estimateInjectionTokens(inj.Content)
			injections = append(injections, inj)
		} else {
			inj := SkillInjection{
				Source:  "error-loop",
				Content: "[Error Recovery] The same error has appeared multiple times. STOP retrying the same approach. (1) Research the error using documentation tools, (2) Try a fundamentally different approach, (3) Check for missing dependencies or config issues.",
			}
			budgetUsed += estimateInjectionTokens(inj.Content)
			injections = append(injections, inj)
		}
	}

	// Language/domain skill matches — match by language field first, then triggers
	if len(signals.DetectedLanguages) > 0 {
		langSet := make(map[string]bool)
		for _, lang := range signals.DetectedLanguages {
			langSet[strings.ToLower(lang)] = true
		}
		// Match skills by their language field (more precise than trigger matching)
		var matched []orchestration.Skill
		seen := make(map[string]bool)
		for _, skill := range registry {
			if skill.Language != "" && langSet[strings.ToLower(skill.Language)] && !seen[skill.Name] {
				matched = append(matched, skill)
				seen[skill.Name] = true
			}
		}
		// Also try trigger matching for languages without a dedicated skill
		goalText := strings.Join(signals.DetectedLanguages, " ")
		triggerMatched := orchestration.MatchSkills(goalText, registry)
		for _, skill := range triggerMatched {
			if !seen[skill.Name] {
				matched = append(matched, skill)
				seen[skill.Name] = true
			}
		}
		for _, skill := range matched {
			if budgetUsed >= maxInjectionTokens {
				break
			}
			// Only inject analyze-phase skills (patterns, not testing/review)
			if skill.Phase != "analyze" {
				continue
			}
			tools := ""
			if len(skill.MCPTools) > 0 {
				tools = " Available tools: " + strings.Join(skill.MCPTools, ", ") + "."
			}
			content := "[Skill Context] This conversation involves " + skill.Name + ": " + skill.Description + "." + tools
			inj := SkillInjection{
				Source:    "skill-match",
				SkillName: skill.Name,
				Content:   content,
				MCPTools:  skill.MCPTools,
			}
			tokens := estimateInjectionTokens(content)
			if budgetUsed+tokens > maxInjectionTokens {
				break
			}
			budgetUsed += tokens
			injections = append(injections, inj)
		}
	}

	// Post-change verification
	if signals.HasCodeChanges && budgetUsed < maxInjectionTokens {
		inj := SkillInjection{
			Source:  "post-change",
			Content: "[Verification] Code changes were made. Verify: go vet ./... → go test -race ./... → go build -o synroute .",
		}
		budgetUsed += estimateInjectionTokens(inj.Content)
		injections = append(injections, inj)
	}

	// Security surface
	if signals.SecuritySurface && budgetUsed < maxInjectionTokens {
		inj := SkillInjection{
			Source:  "security",
			Content: "[Security] Auth/credentials touched. Verify: no secrets in source, proper token handling, input validation.",
		}
		budgetUsed += estimateInjectionTokens(inj.Content)
		injections = append(injections, inj)
	}

	// Research needs
	if signals.ResearchNeeded && budgetUsed < maxInjectionTokens {
		inj := SkillInjection{
			Source:  "research-hint",
			Content: "[Research Available] Use context7.query-docs to look up API documentation before writing code that calls external APIs.",
		}
		_ = estimateInjectionTokens(inj.Content) // budgetUsed not needed after last injection
		injections = append(injections, inj)
	}

	return injections
}

// injectSkillContext prepends or merges injections into the system message.
func injectSkillContext(req *providers.ChatRequest, injections []SkillInjection) {
	if len(injections) == 0 {
		return
	}

	// Build injection text with budget enforcement
	var parts []string
	budgetUsed := 0
	for _, inj := range injections {
		tokens := estimateInjectionTokens(inj.Content)
		if budgetUsed+tokens > maxInjectionTokens {
			break
		}
		parts = append(parts, inj.Content)
		budgetUsed += tokens
	}

	injectionText := strings.Join(parts, "\n")

	// Merge into existing system message or prepend a new one
	if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
		req.Messages[0].Content = injectionText + "\n\n" + req.Messages[0].Content
	} else {
		systemMsg := providers.Message{
			Role:    "system",
			Content: injectionText,
		}
		req.Messages = append([]providers.Message{systemMsg}, req.Messages...)
	}
}

// detectErrorLoop scans the last 10 messages for repeated error patterns.
// Returns whether a loop was detected, the repeated errors, and whether they are validation errors.
func detectErrorLoop(messages []providers.Message) (loopDetected bool, repeatedErrors []string, isValidation bool) {
	start := 0
	if len(messages) > 10 {
		start = len(messages) - 10
	}
	window := messages[start:]

	// Collect error fingerprints
	fingerprints := make(map[string]int)
	var allErrors []string
	for _, msg := range window {
		if msg.Role == "system" {
			continue
		}
		errors := extractErrors(msg.Content)
		allErrors = append(allErrors, errors...)
		for _, e := range errors {
			fp := errorFingerprint(e)
			fingerprints[fp]++
		}
	}

	// Check for retry language
	hasRetryLanguage := false
	for _, msg := range window {
		if msg.Role == "assistant" {
			lower := strings.ToLower(msg.Content)
			for _, phrase := range retryPhrases {
				if strings.Contains(lower, phrase) {
					hasRetryLanguage = true
					break
				}
			}
		}
		if hasRetryLanguage {
			break
		}
	}

	// Loop detected if any fingerprint appears 2+ times
	for fp, count := range fingerprints {
		if count >= 2 {
			loopDetected = true
			repeatedErrors = append(repeatedErrors, fp)
		}
	}

	// Also detect loop via retry language + any errors
	if !loopDetected && hasRetryLanguage && len(allErrors) >= 2 {
		loopDetected = true
		if len(allErrors) > 0 {
			repeatedErrors = append(repeatedErrors, allErrors[0])
		}
	}

	// Classify validation errors
	if loopDetected {
		for _, e := range allErrors {
			if classifyError(e) == "validation" {
				isValidation = true
				break
			}
		}
	}

	return
}

var retryPhrases = []string{
	"let me try again",
	"that didn't work",
	"same error",
	"try a different",
	"let me adjust",
	"let me fix",
	"try with",
	"try changing",
}

// extractErrors finds error-like lines in message content.
func extractErrors(content string) []string {
	var errors []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if containsErrorIndicator(line) && len(line) > 5 {
			errors = append(errors, line)
		}
	}
	return errors
}

var errorPrefixes = []string{"error:", "fail:", "panic:", "fatal:", "error :", "failed:"}
var errorContains = []string{"error:", "fail ", "panic:", "fatal:", "traceback", "exception:", "400 bad request", "500 internal", "404 not found", "429 too many"}

func containsErrorIndicator(text string) bool {
	lower := strings.ToLower(text)
	for _, prefix := range errorPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, substr := range errorContains {
		if strings.Contains(lower, substr) {
			return true
		}
	}
	return false
}

// errorFingerprint normalizes an error for deduplication.
func errorFingerprint(errText string) string {
	// Strip line numbers, addresses, timestamps
	fp := strings.TrimSpace(errText)
	// Take first 80 chars
	if len(fp) > 80 {
		fp = fp[:80]
	}
	return strings.ToLower(fp)
}

// classifyError categorizes an error string.
func classifyError(errText string) string {
	lower := strings.ToLower(errText)

	// Validation errors
	for _, pattern := range validationPatterns {
		if strings.Contains(lower, pattern) {
			return "validation"
		}
	}

	// Auth errors
	for _, pattern := range authPatterns {
		if strings.Contains(lower, pattern) {
			return "auth"
		}
	}

	// Rate limit
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") || strings.Contains(lower, "too many requests") {
		return "rate-limit"
	}

	// Not found
	if strings.Contains(lower, "not found") || strings.Contains(lower, "404") {
		return "not-found"
	}

	return "unknown"
}

var validationPatterns = []string{
	"must be less than",
	"must be greater than",
	"must be at least",
	"must be at most",
	"invalid value",
	"validation failed",
	"validation error",
	"invalid parameter",
	"invalid argument",
	"out of range",
	"must be between",
	"less than or equal to",
	"greater than or equal to",
	"expected type",
	"type mismatch",
}

var authPatterns = []string{
	"unauthorized",
	"forbidden",
	"401",
	"403",
	"invalid token",
	"expired token",
	"authentication failed",
}

// detectCodeChanges scans the last 5 assistant messages for code blocks or modification language.
func detectCodeChanges(messages []providers.Message) bool {
	count := 0
	for i := len(messages) - 1; i >= 0 && count < 5; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		count++

		// Check for code blocks
		if strings.Contains(msg.Content, "```") {
			return true
		}

		// Check for tool calls
		if len(msg.ToolCalls) > 0 {
			return true
		}

		// Check for modification language
		lower := strings.ToLower(msg.Content)
		for _, phrase := range codeChangePhrases {
			if strings.Contains(lower, phrase) {
				return true
			}
		}
	}
	return false
}

var codeChangePhrases = []string{
	"i've updated",
	"i've modified",
	"i've changed",
	"i've created",
	"i've added",
	"i've fixed",
	"created the file",
	"updated the file",
	"modified the file",
	"here's the updated",
	"here is the updated",
}

// detectLanguages scans messages for programming language indicators.
func detectLanguages(messages []providers.Message) []string {
	seen := make(map[string]bool)
	var detected []string

	for _, msg := range messages {
		content := strings.ToLower(msg.Content)
		words := tokenizeWords(content)

		for lang, indicators := range languageIndicators {
			if seen[lang] {
				continue
			}
			for _, ind := range indicators {
				matched := false
				if strings.ContainsAny(ind, ".-_/") || strings.Contains(ind, " ") {
					matched = strings.Contains(content, ind)
				} else if ambiguousLangWords[ind] {
					for _, w := range words {
						if w == ind {
							matched = true
							break
						}
					}
				} else {
					matched = strings.Contains(content, ind)
				}
				if matched {
					seen[lang] = true
					detected = append(detected, lang)
					break
				}
			}
		}
	}

	return detected
}

var ambiguousLangWords = map[string]bool{
	"go": true, "rust": true,
}

var languageIndicators = map[string][]string{
	"go":         {"go", "golang", ".go", "func ", "package ", "go test", "go build", "go vet"},
	"python":     {"python", ".py", "def ", "import ", "pip ", "pytest"},
	"sql":        {"select ", "create table", ".sql", "insert into"},
	"docker":     {"docker", "dockerfile", "compose"},
	"javascript": {"javascript", ".js", "node", "npm", "const ", "let "},
	"typescript": {"typescript", ".ts", ".tsx"},
	"rust":       {".rs", "cargo", "fn ", "impl "},
	"java":       {"java", ".java", "public class"},
}

// detectSecuritySurface checks if auth/credential topics are present.
func detectSecuritySurface(messages []providers.Message) bool {
	securityTriggers := []string{
		"auth", "credential", "token", "oauth", "secret", "password",
		"api key", "apikey", "secure", "security", "vulnerability",
		"permission", "encrypt", "decrypt",
	}
	for _, msg := range messages {
		lower := strings.ToLower(msg.Content)
		for _, trigger := range securityTriggers {
			if strings.Contains(lower, trigger) {
				return true
			}
		}
	}
	return false
}

// detectResearchNeeds checks for question patterns about APIs/libraries.
func detectResearchNeeds(messages []providers.Message) bool {
	// Only check user messages
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		lower := strings.ToLower(msg.Content)
		for _, pattern := range researchPatterns {
			if strings.Contains(lower, pattern) {
				return true
			}
		}
	}
	return false
}

var researchPatterns = []string{
	"how does",
	"how do i",
	"how to",
	"what is the",
	"what are the",
	"documentation for",
	"docs for",
	"api for",
	"library for",
	"look up",
	"find out",
}

// estimateInjectionTokens roughly estimates token count (~4 chars per token).
func estimateInjectionTokens(text string) int {
	return len(text) / 4
}
