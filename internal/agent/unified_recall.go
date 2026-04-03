package agent

import (
	"fmt"
	"log"

	"github.com/gr3enarr0w/synapserouter/internal/memory"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// UnifiedSearcher wraps both ToolOutputStore and VectorMemory to provide
// unified recall across tool outputs and compacted conversation messages.
// It implements tools.ToolOutputSearcher so it can be used as a drop-in
// replacement for the recall tool's searcher.
type UnifiedSearcher struct {
	toolStore    *ToolOutputStore
	vectorMemory *memory.VectorMemory
	sessionIDs   []string // current session first, then parent sessions
}

// NewUnifiedSearcher creates a searcher that queries across multiple sessions.
// sessionIDs should be ordered current-first: [current, parent, grandparent, ...].
func NewUnifiedSearcher(toolStore *ToolOutputStore, vectorMemory *memory.VectorMemory, sessionIDs []string) *UnifiedSearcher {
	return &UnifiedSearcher{
		toolStore:    toolStore,
		vectorMemory: vectorMemory,
		sessionIDs:   sessionIDs,
	}
}

// Search queries ToolOutputStore.SearchMultiSession for all sessionIDs.
// When no toolName filter is set, it also queries VectorMemory for compacted
// conversation messages and merges the results.
func (u *UnifiedSearcher) Search(sessionID, toolName string, limit int) ([]tools.ToolOutputResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Use all known session IDs (current + parents) instead of just the one passed in.
	searchIDs := u.sessionIDs
	if len(searchIDs) == 0 {
		searchIDs = []string{sessionID}
	}

	var results []tools.ToolOutputResult

	// Query tool output store across all sessions.
	if u.toolStore != nil {
		toolResults, err := u.toolStore.SearchMultiSession(searchIDs, toolName, limit)
		if err != nil {
			log.Printf("[UnifiedSearcher] tool store search error: %v", err)
		} else {
			results = append(results, toolResults...)
		}
	}

	// When no tool name filter is set, also query VectorMemory for compacted
	// conversation messages. These are surfaced as synthetic ToolOutputResults
	// so the recall tool can display them uniformly.
	if toolName == "" && u.vectorMemory != nil {
		remaining := limit - len(results)
		if remaining > 0 {
			for _, sid := range searchIDs {
				if remaining <= 0 {
					break
				}
				msgs, err := u.vectorMemory.RetrieveRecentFromSession(sid, remaining)
				if err != nil {
					log.Printf("[UnifiedSearcher] vector memory search error for session %s: %v", sid, err)
					continue
				}
				for _, msg := range msgs {
					if len(results) >= limit {
						break
					}
					results = append(results, tools.ToolOutputResult{
						ID:          -1, // negative ID signals this is a memory entry, not a tool output
						ToolName:    fmt.Sprintf("memory:%s", msg.Role),
						ArgsSummary: fmt.Sprintf("session=%s", sid),
						Summary:     truncateForSummary(msg.Content, 200),
						OutputSize:  len(msg.Content),
					})
					remaining--
				}
			}
		}
	}

	return results, nil
}

// Retrieve delegates to ToolOutputStore.Retrieve. For positive IDs, it tries
// each session ID until the output is found (since the caller may not know
// which session originally stored it).
func (u *UnifiedSearcher) Retrieve(sessionID string, id int64) (string, error) {
	if u.toolStore == nil {
		return "", fmt.Errorf("no tool output store configured")
	}

	// Try the provided sessionID first (most common case).
	output, err := u.toolStore.Retrieve(sessionID, id)
	if err == nil {
		return output, nil
	}

	// Fall back to searching all known session IDs.
	for _, sid := range u.sessionIDs {
		if sid == sessionID {
			continue // already tried
		}
		output, err = u.toolStore.Retrieve(sid, id)
		if err == nil {
			return output, nil
		}
	}

	return "", fmt.Errorf("tool output %d not found in any session", id)
}

// RetrieveRelevant performs semantic search across all known sessions.
// Implements tools.SemanticSearcher.
func (u *UnifiedSearcher) RetrieveRelevant(query, sessionID string, maxTokens int) ([]tools.SemanticResult, error) {
	if u.vectorMemory == nil {
		return nil, fmt.Errorf("no vector memory configured")
	}

	// Search across all known sessions (current + parents).
	searchIDs := u.sessionIDs
	if len(searchIDs) == 0 {
		searchIDs = []string{sessionID}
	}

	var allResults []tools.SemanticResult
	seen := make(map[string]struct{})

	for _, sid := range searchIDs {
		msgs, err := u.vectorMemory.RetrieveRelevant(query, sid, maxTokens)
		if err != nil {
			log.Printf("[UnifiedSearcher] semantic search error for session %s: %v", sid, err)
			continue
		}
		for _, msg := range msgs {
			key := msg.Role + "\x00" + msg.Content
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			allResults = append(allResults, tools.SemanticResult{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	if len(allResults) == 0 {
		return nil, fmt.Errorf("no relevant results found")
	}

	return allResults, nil
}

// truncateForSummary truncates content to maxLen characters for display.
func truncateForSummary(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}
