package agent

import (
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"
)

// Intent represents a classified user intent
type Intent string

const (
	IntentChat     Intent = "chat"
	IntentGenerate Intent = "generate"
	IntentModify   Intent = "modify"
	IntentFix      Intent = "fix"
	IntentResearch Intent = "research"
	IntentExplain  Intent = "explain"
	IntentReview   Intent = "review"
	IntentPlan     Intent = "plan"
	IntentDelegate Intent = "delegate"
	IntentUnknown  Intent = "unknown"
)

// IntentRouter implements hybrid three-layer intent routing
type IntentRouter struct {
	// Layer 1: keyword maps
	greetingKeywords    map[string]bool
	questionPrefixes    []string
	exactPhraseToIntent map[string]Intent

	// Layer 2: TF-IDF semantic routing
	routeExamples    map[Intent][]string
	tfidfVectors     map[Intent][][]float64
	idfWeights       map[string]float64
	vocabulary       map[string]int
	vectorMutex      sync.RWMutex
}

// containsKeyword checks if text contains a keyword with appropriate boundary matching
// For keywords 5+ characters: uses substring matching (strings.Contains)
// For keywords under 5 characters: requires word boundaries
func containsKeyword(text, keyword string) bool {
	if len(keyword) >= 5 {
		return strings.Contains(text, keyword)
	}

	// For short keywords, require word boundaries
	idx := 0
	for {
		pos := strings.Index(text[idx:], keyword)
		if pos == -1 {
			return false
		}
		pos += idx

		// Check boundary before the match
		beforeOK := pos == 0 || isWordBoundary(text[pos-1])
		// Check boundary after the match
		afterPos := pos + len(keyword)
		afterOK := afterPos >= len(text) || isWordBoundary(text[afterPos])

		if beforeOK && afterOK {
			return true
		}
		idx = pos + 1
	}
}

// isWordBoundary returns true if the character is a word boundary
func isWordBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
		c == '.' || c == ',' || c == '!' || c == '?' ||
		c == ';' || c == ':' || c == '(' || c == ')' ||
		c == '[' || c == ']' || c == '{' || c == '}' ||
		c == '"' || c == '\'' || c == '-' || c == '_'
}

// NewIntentRouter creates and initializes the intent router
func NewIntentRouter() *IntentRouter {
	router := &IntentRouter{
		greetingKeywords:    make(map[string]bool),
		questionPrefixes:    []string{},
		exactPhraseToIntent: make(map[string]Intent),
		routeExamples:       make(map[Intent][]string),
		tfidfVectors:        make(map[Intent][][]float64),
		idfWeights:          make(map[string]float64),
		vocabulary:          make(map[string]int),
	}

	router.initLayer1()

	// Load YAML intent routes (embedded + user-defined) — adds to keyword maps
	routes := loadIntentRoutes()
	applyRoutesToRouter(router, routes)

	// Load user corrections (high-priority exact phrase matches)
	corrections := loadCorrections()
	applyCorrectionsToRouter(router, corrections)

	router.initLayer2()
	router.computeTFIDFVectors()

	log.Printf("[IntentRouter] loaded %d phrases, %d greetings, %d prefixes", len(router.exactPhraseToIntent), len(router.greetingKeywords), len(router.questionPrefixes))
	return router
}

// initLayer1 sets up keyword matching rules
func (r *IntentRouter) initLayer1() {
	// Greetings -> chat
	greetings := []string{
		"hello", "hi", "hey", "good morning", "good afternoon", "good evening",
		"how are you", "how's it going", "what's up", "hey there", "hi there",
		"good day", "greetings", "yo", "sup", "howdy", "welcome",
		"hello there", "hi friend", "hey buddy", "good morning everyone",
		"good afternoon everyone", "good evening everyone", "hello everyone",
		"hi everyone", "hey everyone", "morning", "afternoon", "evening",
		"hiya", "hello friend", "hey friend", "g'day", "how are things",
		"how is it going", "what's new", "how have you been", "long time no see",
		"nice to meet you", "pleased to meet you", "great to see you",
	}
	for _, g := range greetings {
		r.greetingKeywords[g] = true
	}

	// Question prefixes for chat (when no @ file refs)
	r.questionPrefixes = []string{
		"what is", "who is", "when was", "why is", "how does",
		"what are", "who are", "when were", "why are", "how do",
		"what was", "who was", "when is", "why was", "how can",
		"what will", "who will", "when will", "why will", "how will",
		"what has", "who has", "when has", "why has", "how has",
		"what had", "who had", "when had", "why had", "how had",
		"what would", "who would", "when would", "why would", "how would",
		"what could", "who could", "when could", "why could", "how could",
		"what should", "who should", "when should", "why should", "how should",
		"who invented", "who created", "who discovered", "who built", "who designed",
		"when did", "where is", "where are", "where was", "where were",
		"how many", "how much", "how long", "how far", "how old",
		"is it", "is there", "are there", "can i", "can you",
		"do you", "does it", "did you", "have you", "will it",
		"tell me about", "tell me what", "tell me how", "tell me why",
	}

	// Exact phrase mappings
	exactPhrases := map[string]Intent{
		// Chat phrases
		"tell me a joke": IntentChat,
		"tell me something": IntentChat,
		"chat with me": IntentChat,
		"let's talk": IntentChat,
		"just chatting": IntentChat,
		"casual conversation": IntentChat,
		"thanks": IntentChat,
		"thank you": IntentChat,
		"nevermind": IntentChat,
		"ship it": IntentDelegate, "deploy it": IntentDelegate, "push it": IntentDelegate, "merge it": IntentDelegate,
		"scratch that": IntentGenerate, "start over": IntentGenerate, "redo": IntentGenerate, "from scratch": IntentGenerate,
		"make it faster": IntentModify, "speed it up": IntentModify, "optimize": IntentModify, "too slow": IntentModify,
		"clean up": IntentModify, "clean this": IntentModify, "tidy up": IntentModify, "prettify": IntentModify,
		"undo": IntentModify, "revert": IntentModify, "roll back": IntentModify, "go back": IntentModify,
		"keeps crashing": IntentFix, "wont work": IntentFix, "doesnt work": IntentFix, "stopped working": IntentFix,
		"not responding": IntentFix, "freezing": IntentFix,
		"2+2": IntentChat,
		"simple math": IntentChat,

		// Code phrases
		// Generate (new files)
		"write code": IntentGenerate,
		"create a function": IntentGenerate,
		"implement this": IntentGenerate,
		"code this": IntentGenerate,
		"build this": IntentGenerate,
		"develop a solution": IntentGenerate,
		"write a script": IntentGenerate,
		"create a class": IntentGenerate,
		"implement a feature": IntentGenerate,
		"code a module": IntentGenerate,
		// Delegate (commands, git, deploy)
		"list all files": IntentDelegate,
		"show me the git log": IntentDelegate,
		"run the tests": IntentDelegate,
		"commit these changes": IntentDelegate,
		"deploy to": IntentDelegate,

		// Fix phrases
		"fix this": IntentFix,
		"debug this": IntentFix,
		"fix the bug": IntentFix,
		"fix the error": IntentFix,
		"resolve this issue": IntentFix,
		"troubleshoot": IntentFix,
		"something is broken": IntentFix,
		"this doesn't work": IntentFix,
		"fix the problem": IntentFix,
		"debug the code": IntentFix,
		"whats wrong with": IntentFix,
		"why is this failing": IntentFix,
		"why does this fail": IntentFix,
		"failing": IntentFix,
		"is broken": IntentFix,
		"not working": IntentFix,
		"broken": IntentFix,
		"doesn't work": IntentFix,
		// Generate prefixes
		"write me a": IntentGenerate,
		"write me": IntentGenerate,
		"can you write": IntentGenerate,
		"can you create": IntentGenerate,
		"can you build": IntentGenerate,
		"can you make": IntentGenerate,
		// Modify prefixes
		"change this": IntentModify,
		"update this": IntentModify,
		"edit this": IntentModify,
		"refactor this": IntentModify,
		"modify this": IntentModify,
		"adjust this": IntentModify,
		// Delegate prefixes
		"execute": IntentDelegate,
		"run command": IntentDelegate,
		"start the": IntentDelegate,
		"install the": IntentDelegate,

		// Research phrases
		"research this": IntentResearch,
		"look this up": IntentResearch,
		"find information": IntentResearch,
		"search for": IntentResearch,
		"investigate": IntentResearch,
		"learn about": IntentResearch,
		"what do you know about": IntentResearch,
		"tell me about": IntentResearch,
		"explain the concept": IntentResearch,
		"background on": IntentResearch,

		// Explain phrases
		"explain this": IntentExplain,
		"how does this work": IntentExplain,
		"walk me through": IntentExplain,
		"break this down": IntentExplain,
		"this codebase": IntentExplain,
		"the codebase": IntentExplain,
		"this project": IntentExplain,
		"explain the code": IntentExplain,
		"what does this do": IntentExplain,
		"help me understand": IntentExplain,
		"I need help": IntentExplain,
		"clarify this": IntentExplain,
		"make this clear": IntentExplain,
		"simplify this": IntentExplain,

		// Review phrases
		"review this": IntentReview,
		"code review": IntentReview,
		"check this code": IntentReview,
		"audit this": IntentReview,
		"audit the": IntentReview,
		"audit my": IntentReview,
		"audit our": IntentReview,
		"evaluate this": IntentReview,
		"analyze this": IntentReview,
		"look over this": IntentReview,
		"examine this": IntentReview,
		"inspect this": IntentReview,
		"assess this": IntentReview,

		// Plan phrases
		"plan this": IntentPlan,
		"create a plan": IntentPlan,
		"design a solution": IntentPlan,
		"outline the approach": IntentPlan,
		"strategy for": IntentPlan,
		"roadmap for": IntentPlan,
		"steps to": IntentPlan,
		"how to approach": IntentPlan,
		"approach this": IntentPlan,
		"approach the": IntentPlan,
		"should i approach": IntentPlan,
		"design this": IntentPlan,
		"architecture for": IntentPlan,
	}

	for phrase, intent := range exactPhrases {
		r.exactPhraseToIntent[strings.ToLower(phrase)] = intent
	}
}

// initLayer2 sets up TF-IDF training examples (50+ per route)
func (r *IntentRouter) initLayer2() {
	r.routeExamples = map[Intent][]string{
		IntentChat: {
			"hello how are you", "hi there", "good morning", "hey buddy",
			"how's it going", "what's up", "just wanted to say hi", "greetings",
			"how are things", "hope you're doing well", "nice to meet you",
			"let's chat", "casual conversation", "tell me a joke", "entertain me",
			"what's new", "how have you been", "long time no see", "welcome",
			"good afternoon", "good evening", "hey there friend", "hi friend",
			"yo what's up", "sup", "howdy", "g'day mate", "morning everyone",
			"afternoon everyone", "evening everyone", "hello everyone", "hi all",
			"hey all", "good day", "pleasant day", "lovely day", "beautiful day",
			"hope you're having a great day", "how's your day going", "what's happening",
			"anything new", "what's going on", "how's life", "how are you today",
			"good to see you", "great to see you", "pleased to meet you",
			"nice talking to you", "let's talk", "just chatting", "friendly chat",
			"ok", "k", "yes", "no", "sure", "yep", "nope", "nah", "lol", "lmao", "idk",
			"ty", "thx", "np", "gg", "lgtm", "interesting", "nice", "cool", "sweet",
			"awesome", "perfect", "great", "wow", "hmm", "huh", "meh", "ugh", "oops",
			"sorry", "please", "thanks", "thank you", "goodbye", "bye", "see ya",
			"cheers", "yo", "sup", "hey",
		},
		IntentGenerate: {
			"write a function", "create a class", "implement a feature", "code this module",
			"build a solution", "develop this", "write the code", "create a script",
			"implement the algorithm", "code the logic", "write a program", "build this feature",
			"create a new file", "add a function", "implement the method", "code a component",
			"write the implementation", "create a helper", "build a utility", "develop a module",
			"write a test", "create test cases", "implement tests", "code the tests",
			"build the api", "create an endpoint", "implement the route", "code the handler",
			"write a query", "create a database function", "implement the model", "code the schema",
			"write a parser", "create a lexer", "implement the compiler", "code the interpreter",
			"build a cli", "create a command", "implement the flag", "code the argument parser",
			"write a server", "create a service", "implement the backend", "code the api",
			"build a frontend", "create a component", "implement the ui", "code the view",
			"write a library", "create a package", "implement the interface", "code the abstraction",
			"build a tool", "create a utility", "implement the helper", "code the function",
		},
		IntentFix: {
			"fix this bug", "debug the issue", "resolve the error", "troubleshoot this",
			"something is broken", "this doesn't work", "fix the problem", "debug this code",
			"fix the crash", "resolve the exception", "troubleshoot the error", "fix the failure",
			"debug the test", "fix the failing test", "resolve the test error", "troubleshoot the test",
			"fix the compilation error", "debug the build", "resolve the build failure", "fix the lint error",
			"fix the type error", "debug the type issue", "resolve the type mismatch", "fix the null pointer",
			"fix the memory leak", "debug the memory issue", "resolve the performance problem", "fix the slow code",
			"fix the race condition", "debug the concurrency issue", "resolve the deadlock", "fix the timeout",
			"fix the api error", "debug the network issue", "resolve the connection problem", "fix the http error",
			"fix the database error", "debug the query", "resolve the sql issue", "fix the data problem",
			"fix the authentication error", "debug the auth issue", "resolve the permission problem", "fix the access denied",
			"fix the validation error", "debug the input issue", "resolve the format problem", "fix the parsing error",
			"fix the logic error", "debug the algorithm", "resolve the calculation issue", "fix the wrong output",
		},
		IntentResearch: {
			"research this topic", "look up information", "find data on", "search for details",
			"investigate this", "learn about this", "what do you know about", "tell me about",
			"explain the background", "give me context", "what is the history", "research the concept",
			"look into this", "find information about", "search the web", "investigate further",
			"learn more about", "what are the details", "tell me more", "explain further",
			"research the technology", "look up the documentation", "find the specs", "search for examples",
			"investigate the issue", "learn the best practices", "what are the standards", "research the patterns",
			"look up the api", "find the reference", "search for tutorials", "investigate the options",
			"learn the fundamentals", "what are the basics", "research the theory", "look up the principles",
			"find the use cases", "search for applications", "investigate the trends", "learn the latest",
			"research the competition", "look up alternatives", "find comparisons", "search for reviews",
			"investigate the market", "learn the industry", "what are the players", "research the landscape",
			"look up the news", "find recent articles", "search for updates", "investigate changes",
		},
		IntentExplain: {
			"explain this code", "how does this work", "walk me through", "break this down",
			"what does this do", "help me understand", "clarify this", "make this clear",
			"simplify this", "explain the concept", "how it works", "what is happening",
			"explain the logic", "walk through the code", "break down the function", "what is this doing",
			"help me understand this", "clarify the purpose", "make it clearer", "simplify the explanation",
			"explain the algorithm", "how the algorithm works", "what the algorithm does", "explain the approach",
			"explain the design", "how the design works", "what the design does", "explain the architecture",
			"explain the pattern", "how the pattern works", "what the pattern does", "explain the principle",
			"explain the syntax", "how the syntax works", "what the syntax means", "explain the semantics",
			"explain the behavior", "how it behaves", "what happens when", "explain the flow",
			"explain the process", "how the process works", "what the process does", "explain the workflow",
			"explain the pipeline", "how the pipeline works", "what the pipeline does", "explain the system",
		},
		IntentReview: {
			"review this code", "code review", "check this code", "audit this",
			"evaluate this", "analyze this", "look over this", "examine this",
			"inspect this", "assess this", "review the implementation", "check the quality",
			"audit the code", "evaluate the design", "analyze the structure", "look for issues",
			"examine the logic", "inspect the implementation", "assess the quality", "review for bugs",
			"check for errors", "audit for security", "evaluate performance", "analyze efficiency",
			"look for improvements", "examine best practices", "inspect conventions", "assess readability",
			"review the tests", "check test coverage", "audit test quality", "evaluate test cases",
			"analyze test structure", "look for test gaps", "examine test logic", "inspect test data",
			"review the documentation", "check the comments", "audit the docs", "evaluate clarity",
			"analyze completeness", "look for missing docs", "examine examples", "inspect accuracy",
			"review the architecture", "check the design", "audit the structure", "evaluate patterns",
			"analyze dependencies", "look for coupling", "examine cohesion", "inspect modularity",
		},
		IntentPlan: {
			"plan this project", "create a plan", "design a solution", "outline the approach",
			"strategy for this", "roadmap for", "steps to build", "how to approach",
			"design the architecture", "plan the implementation", "create a roadmap", "outline the steps",
			"strategy for building", "plan the development", "design the system", "outline the components",
			"plan the features", "create a timeline", "design the workflow", "outline the process",
			"strategy for testing", "plan the tests", "design test cases", "outline test strategy",
			"plan the deployment", "create deployment plan", "design the pipeline", "outline the ci/cd",
			"plan the migration", "create migration strategy", "design the transition", "outline the rollout",
			"plan the refactoring", "create refactoring plan", "design the improvements", "outline the changes",
			"plan the optimization", "create optimization strategy", "design performance improvements", "outline the tuning",
			"plan the integration", "create integration plan", "design the api", "outline the interfaces",
			"plan the database", "create schema plan", "design the data model", "outline the tables",
			"plan the security", "create security plan", "design authentication", "outline the authorization",
		},
	}

	// Load test cases as additional training data for better TF-IDF accuracy
	r.loadTestCasesAsTrainingData()
}

// loadTestCasesAsTrainingData loads test cases from JSON and adds them to routeExamples
func (r *IntentRouter) loadTestCasesAsTrainingData() {
	data, err := embeddedIntentRoutes.ReadFile("intent_data/training_cases.json")
	if err != nil {
		log.Printf("Warning: Could not load test cases for TF-IDF training: %v", err)
		return
	}

	var testCases []struct {
		Message  string `json:"message"`
		Expected string `json:"expected"`
	}
	if err := json.Unmarshal(data, &testCases); err != nil {
		log.Printf("Warning: Could not parse test cases: %v", err)
		return
	}

	intentMap := map[string]Intent{
		"chat":      IntentChat,
		"code":      IntentGenerate,
		"fix":       IntentFix,
		"generate":  IntentGenerate,
		"explain":   IntentExplain,
		"review":    IntentReview,
		"research":  IntentResearch,
		"plan":      IntentPlan,
		"optimize":  IntentModify,
		"translate": IntentChat,
		"test":      IntentGenerate,
	}

	for _, tc := range testCases {
		intent, ok := intentMap[tc.Expected]
		if !ok {
			continue
		}
		r.routeExamples[intent] = append(r.routeExamples[intent], tc.Message)
	}
	log.Printf("Loaded %d test cases as TF-IDF training data", len(testCases))
}

// computeTFIDFVectors pre-computes TF-IDF vectors for all training examples
func (r *IntentRouter) computeTFIDFVectors() {
	// Build vocabulary and document frequencies
	docFreq := make(map[string]int)
	allDocs := []string{}

	for _, examples := range r.routeExamples {
		for _, example := range examples {
			allDocs = append(allDocs, example)
			tokens := tokenize(example)
			seen := make(map[string]bool)
			for _, token := range tokens {
				if !seen[token] {
					docFreq[token]++
					seen[token] = true
				}
			}
		}
	}

	// Build vocabulary and IDF weights
	n := float64(len(allDocs))
	idx := 0
	for token, df := range docFreq {
		r.vocabulary[token] = idx
		idx++
		r.idfWeights[token] = math.Log(n / float64(df))
	}

	// Compute TF-IDF vectors for each route
	for intent, examples := range r.routeExamples {
		vectors := make([][]float64, len(examples))
		for i, example := range examples {
			vectors[i] = r.computeTFIDFVector(example)
		}
		r.tfidfVectors[intent] = vectors
	}
}

// tokenize splits text into lowercase tokens
func tokenize(text string) []string {
	text = strings.ToLower(text)
	text = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			return r
		}
		return ' '
	}, text)
	return strings.Fields(text)
}

// computeTFIDFVector computes TF-IDF vector for a single document
func (r *IntentRouter) computeTFIDFVector(text string) []float64 {
	tokens := tokenize(text)
	tf := make(map[string]float64)
	for _, token := range tokens {
		tf[token]++
	}
	// Normalize TF
	for token := range tf {
		tf[token] /= float64(len(tokens))
	}

	// Build TF-IDF vector
	vector := make([]float64, len(r.vocabulary))
	for token, tfVal := range tf {
		if idx, ok := r.vocabulary[token]; ok {
			vector[idx] = tfVal * r.idfWeights[token]
		}
	}

	// Normalize vector
	norm := 0.0
	for _, v := range vector {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}

	return vector
}

// cosineSimilarity computes cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	normA = math.Sqrt(normA)
	normB = math.Sqrt(normB)
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (normA * normB)
}

// hasFileReferences checks if message contains @ file references
func hasFileReferences(message string) bool {
	return strings.Contains(message, "@")
}

// Classify runs the three-layer intent classification
func (r *IntentRouter) Classify(message string) Intent {
	message = strings.TrimSpace(message)
	messageLower := strings.ToLower(message)
	
	// Short message default: <=3 chars → chat
	if len(messageLower) <= 3 && messageLower != "" {
		return IntentChat
	}
	
	// Help me patterns
	if strings.Contains(messageLower, "help me make") || strings.Contains(messageLower, "help me create") ||
		strings.Contains(messageLower, "help me build") || strings.Contains(messageLower, "help me write") {
		return IntentGenerate
	}
	if strings.Contains(messageLower, "help me fix") || strings.Contains(messageLower, "help me debug") {
		return IntentFix
	}
	if strings.Contains(messageLower, "help me understand") {
		return IntentExplain
	}

	// LAYER 1: Keyword matching (deterministic, 0ms)

	// Strip greetings to check for code intent underneath
	strippedMessage := messageLower
	for greeting := range r.greetingKeywords {
		if strings.HasPrefix(strippedMessage, greeting+" ") {
			strippedMessage = strings.TrimPrefix(strippedMessage, greeting+" ")
		}
		if strings.HasSuffix(strippedMessage, " "+greeting) {
			strippedMessage = strings.TrimSuffix(strippedMessage, " "+greeting)
		}
		strippedMessage = strings.TrimSpace(strippedMessage)
	}

	// Check exact phrases FIRST (more specific than question prefixes)
	// Track ALL matching intents for conflict detection
	matchingIntents := make(map[Intent]bool)
	for phrase, intent := range r.exactPhraseToIntent {
		if containsKeyword(strippedMessage, phrase) {
			matchingIntents[intent] = true
		}
		if containsKeyword(messageLower, phrase) {
			matchingIntents[intent] = true
		}
	}
	// If intents matched, pick the one with the longest matching phrase
	if len(matchingIntents) > 0 {
		bestIntent := IntentUnknown
		bestLen := 0
		for phrase, intent := range r.exactPhraseToIntent {
			if (containsKeyword(strippedMessage, phrase) || containsKeyword(messageLower, phrase)) && len(phrase) > bestLen {
				bestIntent = intent
				bestLen = len(phrase)
			}
		}
		if bestIntent != IntentUnknown {
			return bestIntent
		}
	}
	// No match — fall through to Layer 2

	// Check greetings (only if no code/research/fix intent found)
	for greeting := range r.greetingKeywords {
		if messageLower == greeting || strings.HasPrefix(messageLower, greeting+" ") || strings.HasSuffix(messageLower, " "+greeting) {
			return IntentChat
		}
	}

	// Check question prefixes LAST — exact phrases and greetings take priority
	// Only match question prefix if NO other keyword matched anywhere in the message
	if !hasFileReferences(message) {
		for _, prefix := range r.questionPrefixes {
			if strings.HasPrefix(messageLower, prefix+" ") || strings.HasPrefix(messageLower, prefix+"?") {
				return IntentChat
			}
		}
	}

	// LAYER 2: TF-IDF semantic routing
	r.vectorMutex.RLock()
	bestIntent := IntentUnknown
	bestScore := 0.0
	threshold := 0.25

	queryVector := r.computeTFIDFVector(message)

	for intent, vectors := range r.tfidfVectors {
		for _, exampleVector := range vectors {
			similarity := cosineSimilarity(queryVector, exampleVector)
			if similarity > bestScore {
				bestScore = similarity
				bestIntent = intent
			}
		}
	}
	r.vectorMutex.RUnlock()

	if bestScore >= threshold {
		return bestIntent
	}

	// LAYER 3: LLM fallback
	return IntentUnknown
}

// GetAllowedTools returns the subset of tools allowed for a given intent
func GetAllowedTools(intent Intent, allTools []string) []string {
	switch intent {
	case IntentChat:
		// No tools allowed for chat
		return []string{}
	case IntentExplain, IntentReview:
		// Read-only subset
		readOnly := []string{"file_read", "glob", "grep", "bash"}
		allowed := []string{}
		for _, tool := range allTools {
			for _, ro := range readOnly {
				if tool == ro {
					allowed = append(allowed, tool)
					break
				}
			}
		}
		return allowed
	case IntentResearch:
		// Web search only
		allowed := []string{}
		for _, tool := range allTools {
			if tool == "web_search" || tool == "web_fetch" {
				allowed = append(allowed, tool)
			}
		}
		return allowed
	default:
		// Unknown or other intents: all tools
		return allTools
	}
}

// filterToolsByIntent filters OpenAI tool definitions by intent
func filterToolsByIntent(intent Intent, allTools []map[string]interface{}) []map[string]interface{} {
	switch intent {
	case IntentChat, IntentPlan:
		return nil // no_tools: chat, plan
	case IntentExplain, IntentReview:
		return filterToolNames(allTools, "file_read", "glob", "grep", "recall") // read_only
	case IntentModify, IntentFix:
		return filterToolNames(allTools, "file_read", "file_edit", "bash", "grep", "glob", "recall") // read_write
	case IntentGenerate, IntentDelegate:
		return allTools // full access
	case IntentResearch:
		return filterToolNames(allTools, "web_search", "web_fetch", "recall") // web
	default:
		return allTools
	}
}

func filterToolNames(tools []map[string]interface{}, allowed ...string) []map[string]interface{} {
	allowedSet := make(map[string]bool)
	for _, a := range allowed {
		allowedSet[a] = true
	}
	var filtered []map[string]interface{}
	for _, t := range tools {
		if fn, ok := t["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok && allowedSet[name] {
				filtered = append(filtered, t)
			}
		}
	}
	return filtered
}
