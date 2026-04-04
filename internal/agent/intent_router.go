package agent

import (
	"math"
	"strings"
	"sync"
)

// Intent represents a classified user intent
type Intent string

const (
	IntentChat     Intent = "chat"
	IntentCode     Intent = "code"
	IntentFix      Intent = "fix"
	IntentResearch Intent = "research"
	IntentExplain  Intent = "explain"
	IntentReview   Intent = "review"
	IntentPlan     Intent = "plan"
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
	router.initLayer2()
	router.computeTFIDFVectors()

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
		"2+2": IntentChat,
		"simple math": IntentChat,

		// Code phrases
		"write code": IntentCode,
		"create a function": IntentCode,
		"implement this": IntentCode,
		"code this": IntentCode,
		"build this": IntentCode,
		"develop a solution": IntentCode,
		"write a script": IntentCode,
		"create a class": IntentCode,
		"implement a feature": IntentCode,
		"code a module": IntentCode,
		"list all files": IntentCode,
		"show me the git log": IntentCode,
		"run the tests": IntentCode,
		"commit these changes": IntentCode,
		"deploy to": IntentCode,

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
		"write me a": IntentCode,
		"write me": IntentCode,
		"can you write": IntentCode,
		"can you create": IntentCode,
		"can you build": IntentCode,
		"can you make": IntentCode,

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
		},
		IntentCode: {
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
	// Check stripped message first for code override, then original
	for phrase, intent := range r.exactPhraseToIntent {
		if strings.Contains(strippedMessage, phrase) {
			return intent
		}
		if strings.Contains(messageLower, phrase) {
			return intent
		}
	}

	// Check greetings (only if no code/research/fix intent found)
	for greeting := range r.greetingKeywords {
		if messageLower == greeting || strings.HasPrefix(messageLower, greeting+" ") || strings.HasSuffix(messageLower, " "+greeting) {
			return IntentChat
		}
	}

	// Check question prefixes (only if no @ file refs, and no exact phrase matched)
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
	case IntentChat:
		return nil // no tools for chat
	case IntentExplain, IntentReview:
		return filterToolNames(allTools, "file_read", "glob", "grep", "recall")
	case IntentResearch:
		return filterToolNames(allTools, "web_search", "web_fetch", "recall")
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
