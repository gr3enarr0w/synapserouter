package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type VectorMemory struct {
	db       *sql.DB
	embedder EmbeddingProvider
}

type MemoryEntry struct {
	ID        int64
	Content   string
	Embedding []byte
	Timestamp time.Time
	SessionID string
	Role      string
	Metadata  map[string]interface{}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func NewVectorMemory(db *sql.DB) *VectorMemory {
	return NewVectorMemoryWithEmbedder(db, nil)
}

func NewVectorMemoryWithEmbedder(db *sql.DB, embedder EmbeddingProvider) *VectorMemory {
	if embedder == nil {
		// Try OpenAI embeddings if API key is available
		if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
			embedder = NewOpenAIEmbedding(apiKey)
			log.Println("[Memory] Using OpenAI embeddings for semantic search")
		} else {
			// Fallback to local hash-based embeddings
			embedder = NewLocalHashEmbedding(384)
			log.Println("[Memory] Using local hash embeddings (set OPENAI_API_KEY for better semantic search)")
		}
	}

	return &VectorMemory{
		db:       db,
		embedder: embedder,
	}
}

// Store saves a message to memory for later SQLite-backed retrieval.
func (vm *VectorMemory) Store(content, role, sessionID string, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	// Generate embedding for the content
	var embeddingBytes []byte
	if vm.embedder != nil && content != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		embedding, err := vm.embedder.Embed(ctx, content)
		if err != nil {
			log.Printf("[Memory] Failed to generate embedding: %v (storing without embedding)", err)
		} else {
			embeddingBytes = EncodeEmbedding(embedding)
		}
	}

	_, err = vm.db.Exec(`
		INSERT INTO memory (content, embedding, timestamp, session_id, role, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
	`, content, embeddingBytes, time.Now(), sessionID, role, string(metadataJSON))

	if err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	log.Printf("[Memory] Stored message (role=%s, session=%s, len=%d, embedded=%v)",
		role, sessionID, len(content), embeddingBytes != nil)
	return nil
}

// StoreMessages stores multiple messages
func (vm *VectorMemory) StoreMessages(messages []Message, sessionID string) error {
	for _, msg := range messages {
		metadata := map[string]interface{}{
			"timestamp": time.Now().Unix(),
		}
		if err := vm.Store(msg.Content, msg.Role, sessionID, metadata); err != nil {
			return err
		}
	}
	return nil
}

// RetrieveRecent gets the most recent N messages, optionally filtering out an identical query
func (vm *VectorMemory) RetrieveRecent(sessionID string, limit int, skipContent string) ([]Message, error) {
	rows, err := vm.db.Query(`
		SELECT role, content
		FROM memory
		WHERE session_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, sessionID, limit*2) // Get more so we can filter
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	skipContent = strings.TrimSpace(skipContent)
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			continue
		}
		if skipContent != "" && strings.TrimSpace(msg.Content) == skipContent {
			continue
		}
		messages = append(messages, msg)
		if len(messages) >= limit {
			break
		}
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// RetrieveRelevant gets relevant context using a hybrid approach:
// 1. Always includes the most recent N messages for immediate continuity.
// 2. Supplement with semantic search results (vector or lexical).
func (vm *VectorMemory) RetrieveRelevant(query, sessionID string, maxTokens int) ([]Message, error) {
	// Cap memory injection to prevent context overflow.
	// Memory should never dominate the context window — 8K max, default 4K.
	if maxTokens <= 0 || maxTokens > 8192 {
		maxTokens = 4096
	}

	// 1. Always get most recent context (crucial for handoffs and direct follow-ups)
	recent, err := vm.RetrieveRecent(sessionID, 4, query)
	if err != nil {
		log.Printf("[Memory] Warning: failed to retrieve recent context: %v", err)
	}

	recentTokens := 0
	seen := make(map[string]struct{})
	for _, msg := range recent {
		seen[msg.Role+"\x00"+msg.Content] = struct{}{}
		recentTokens += EstimateTokens(msg.Content)
	}

	// 2. Get semantic context (clamp budget to avoid negative limits)
	semanticBudget := maxTokens - recentTokens
	if semanticBudget < 0 {
		semanticBudget = 0
	}
	var semantic []Message
	if semanticBudget > 0 && vm.embedder != nil {
		semantic, _ = vm.retrieveByVectorSimilarity(query, sessionID, semanticBudget)
	}
	if len(semantic) == 0 && semanticBudget > 0 {
		semantic, _ = vm.retrieveByLexicalScore(query, sessionID, semanticBudget)
	}

	// 3. Combine and deduplicate
	results := make([]Message, 0, len(recent)+len(semantic))
	results = append(results, recent...)

	for _, msg := range semantic {
		key := msg.Role + "\x00" + msg.Content
		if _, exists := seen[key]; !exists {
			results = append(results, msg)
			seen[key] = struct{}{}
		}
	}

	// Sort final results chronologically? 
	// Usually, we want history in order. Semantic search might return jumbled times.
	// But RetrieveRecent is already chronological. Semantic items are added after.
	
	log.Printf("[Memory] Hybrid search found %d messages (%d recent, %d semantic)", 
		len(results), len(recent), len(results)-len(recent))

	return results, nil
}

// retrieveByVectorSimilarity uses cosine similarity between embeddings
func (vm *VectorMemory) retrieveByVectorSimilarity(query, sessionID string, maxTokens int) ([]Message, error) {
	// Generate query embedding
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queryEmbedding, err := vm.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// Fetch messages with embeddings
	rows, err := vm.db.Query(`
		SELECT role, content, embedding, timestamp
		FROM memory
		WHERE session_id = ? AND embedding IS NOT NULL
		ORDER BY timestamp DESC
		LIMIT 200
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query memory: %w", err)
	}
	defer rows.Close()

	type scoredMessage struct {
		Message
		timestamp time.Time
		score     float32
	}

	var scored []scoredMessage
	for rows.Next() {
		var msg scoredMessage
		var embeddingBytes []byte
		if err := rows.Scan(&msg.Role, &msg.Content, &embeddingBytes, &msg.timestamp); err != nil {
			continue
		}

		if len(embeddingBytes) == 0 {
			continue
		}

		embedding, err := DecodeEmbedding(embeddingBytes)
		if err != nil {
			continue
		}

		similarity := CosineSimilarity(queryEmbedding, embedding)
		msg.score = similarity
		scored = append(scored, msg)
	}

	if len(scored) == 0 {
		return nil, fmt.Errorf("no messages with embeddings found")
	}

	// Sort by similarity score (higher is better)
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].timestamp.After(scored[j].timestamp)
		}
		return scored[i].score > scored[j].score
	})

	// Collect top results within token limit
	totalTokens := 0
	results := make([]Message, 0, len(scored))
	seen := make(map[string]struct{})

	for _, item := range scored {
		if _, exists := seen[item.Role+"\x00"+item.Content]; exists {
			continue
		}

		// Skip if it's identical to the query (to avoid echoing)
		if strings.TrimSpace(item.Content) == strings.TrimSpace(query) {
			continue
		}

		msgTokens := EstimateTokens(item.Content)
		if maxTokens > 0 && totalTokens+msgTokens > maxTokens {
			continue
		}

		results = append(results, item.Message)
		seen[item.Role+"\x00"+item.Content] = struct{}{}
		totalTokens += msgTokens
	}

	log.Printf("[Memory] Vector search found %d relevant messages (~%d tokens, limit %d)",
		len(results), totalTokens, maxTokens)

	return results, nil
}

// retrieveByLexicalScore is the fallback lexical search
func (vm *VectorMemory) retrieveByLexicalScore(query, sessionID string, maxTokens int) ([]Message, error) {
	queryTerms := extractSearchTerms(query)
	if len(queryTerms) == 0 {
		return vm.RetrieveRecent(sessionID, 20, query)
	}

	rows, err := vm.db.Query(`
		SELECT role, content, timestamp
		FROM memory
		WHERE session_id = ?
		ORDER BY timestamp DESC
		LIMIT 200
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}
	defer rows.Close()

	type scoredMessage struct {
		Message
		timestamp time.Time
		score     float64
	}

	var scored []scoredMessage
	for rows.Next() {
		var msg scoredMessage
		if err := rows.Scan(&msg.Role, &msg.Content, &msg.timestamp); err != nil {
			continue
		}
		msg.score = lexicalScore(msg.Content, queryTerms)
		if msg.score > 0 {
			scored = append(scored, msg)
		}
	}

	if len(scored) == 0 {
		return vm.RetrieveRecent(sessionID, 20, query)
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].timestamp.After(scored[j].timestamp)
		}
		return scored[i].score > scored[j].score
	})

	totalTokens := 0
	results := make([]Message, 0, len(scored))
	seen := make(map[string]struct{})
	for _, item := range scored {
		if _, exists := seen[item.Role+"\x00"+item.Content]; exists {
			continue
		}

		// Skip if it's identical to the query (to avoid echoing)
		if strings.TrimSpace(item.Content) == strings.TrimSpace(query) {
			continue
		}

		msgTokens := EstimateTokens(item.Content)
		if maxTokens > 0 && totalTokens+msgTokens > maxTokens {
			continue
		}

		results = append(results, item.Message)
		seen[item.Role+"\x00"+item.Content] = struct{}{}
		totalTokens += msgTokens
	}

	if len(results) == 0 {
		return vm.RetrieveRecent(sessionID, 20, query)
	}

	log.Printf("[Memory] Lexical search found %d relevant messages (~%d tokens, limit %d)",
		len(results), totalTokens, maxTokens)

	return results, nil
}

// EstimateTokens roughly estimates token count (1 token ≈ 4 characters)
func EstimateTokens(text string) int {
	return len(text) / 4
}

// EstimateMessagesTokens estimates total tokens in message list
func EstimateMessagesTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg.Content)
	}
	return total
}

// TrimToTokenLimit trims messages to fit within token limit
func (vm *VectorMemory) TrimToTokenLimit(messages []Message, maxTokens int) []Message {
	total := 0
	var result []Message

	// Keep most recent messages that fit
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i].Content)
		if total+msgTokens > maxTokens {
			break
		}
		result = append([]Message{messages[i]}, result...)
		total += msgTokens
	}

	log.Printf("[Memory] Trimmed to %d messages (~%d tokens, limit %d)",
		len(result), total, maxTokens)
	return result
}

// RetrieveRecentFromSession gets the most recent N messages from a specific session,
// returned in chronological order. Used for cross-session memory carry-over.
func (vm *VectorMemory) RetrieveRecentFromSession(sessionID string, limit int) ([]Message, error) {
	rows, err := vm.db.Query(`
		SELECT role, content
		FROM memory
		WHERE session_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// GetSessionHistory gets full conversation history for a session
func (vm *VectorMemory) GetSessionHistory(sessionID string) ([]Message, error) {
	rows, err := vm.db.Query(`
		SELECT role, content
		FROM memory
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// Cleanup removes old memories
func (vm *VectorMemory) Cleanup(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)

	result, err := vm.db.Exec(`
		DELETE FROM memory
		WHERE timestamp < ?
	`, cutoff)

	if err != nil {
		return err
	}

	count, _ := result.RowsAffected()
	log.Printf("[Memory] Cleaned up %d old entries (older than %v)", count, olderThan)
	return nil
}

func extractSearchTerms(query string) []string {
	terms := strings.Fields(strings.ToLower(query))
	filtered := make([]string, 0, len(terms))
	for _, term := range terms {
		normalized := normalizeSearchTerm(term)
		if len(normalized) < 2 {
			continue
		}
		filtered = append(filtered, normalized)
	}

	return filtered
}

func normalizeSearchTerm(term string) string {
	var builder strings.Builder
	for _, r := range term {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func lexicalScore(content string, queryTerms []string) float64 {
	contentTerms := strings.Fields(strings.ToLower(content))
	if len(contentTerms) == 0 {
		return 0
	}

	normalizedTerms := make([]string, 0, len(contentTerms))
	termCounts := make(map[string]int)
	for _, term := range contentTerms {
		normalized := normalizeSearchTerm(term)
		if normalized == "" {
			continue
		}
		normalizedTerms = append(normalizedTerms, normalized)
		termCounts[normalized]++
	}

	if len(normalizedTerms) == 0 {
		return 0
	}

	score := 0.0
	normalizedContent := " " + strings.Join(normalizedTerms, " ") + " "
	for _, term := range queryTerms {
		if termCounts[term] > 0 {
			score += 1 + float64(termCounts[term]-1)*0.25
		}
		if strings.Contains(normalizedContent, " "+term+" ") {
			score += 0.5
		}
	}

	return score
}
