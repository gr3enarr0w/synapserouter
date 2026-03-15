package agent

import (
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

const maxConversationMessages = 200

// Conversation manages message history for an agent session.
type Conversation struct {
	messages []providers.Message
}

// NewConversation creates an empty conversation.
func NewConversation() *Conversation {
	return &Conversation{}
}

// Add appends a message to the conversation history.
func (c *Conversation) Add(msg providers.Message) {
	c.messages = append(c.messages, msg)
	c.trim()
}

// Messages returns the full message history.
func (c *Conversation) Messages() []providers.Message {
	return c.messages
}

// Clear resets the conversation history.
func (c *Conversation) Clear() {
	c.messages = nil
}

// trim drops old messages when the conversation exceeds the max,
// respecting tool-call boundaries so assistant messages with ToolCalls
// are never separated from their corresponding tool result messages.
func (c *Conversation) trim() {
	if len(c.messages) <= maxConversationMessages {
		return
	}

	keepFrom := len(c.messages) - maxConversationMessages
	if keepFrom < 0 {
		keepFrom = 0
	}

	// Advance keepFrom past any orphaned tool messages at the cut point.
	// If we'd start in the middle of a tool-call sequence, move forward
	// until we find a clean boundary (a user or assistant-without-tool-calls message).
	for keepFrom < len(c.messages) {
		msg := c.messages[keepFrom]
		if msg.Role == "tool" {
			// This is a tool result without its preceding assistant tool call — skip it
			keepFrom++
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// This assistant message has tool calls — we need to include all
			// the tool results that follow. But if we're cutting here, we'd
			// lose the context. Skip this whole group.
			keepFrom++
			// Skip the corresponding tool results
			for keepFrom < len(c.messages) && c.messages[keepFrom].Role == "tool" {
				keepFrom++
			}
			continue
		}
		break // Clean boundary — user message or plain assistant message
	}

	c.messages = c.messages[keepFrom:]
}
