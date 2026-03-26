# Synapse Router Architecture

## Core Design Philosophy

**The primary goal of Synapse Router is to enable UNIFIED CONTEXT across all LLM providers and tools.**

Unlike traditional LLM proxies that simply route requests to different APIs, Synapse Router maintains a **shared memory system** that preserves conversational context across provider switches, tool calls, and orchestration tasks.

## Key Innovation: Unified Context Through Vector Memory

### The Problem
When routing between different LLM providers (Claude → OpenAI → Gemini), traditional approaches lose context:
- Each provider only sees the current request
- Previous conversations are forgotten
- Tool results aren't shared across providers
- Provider switches break multi-turn conversations

### The Solution: Vector Memory as Universal Context Store

```
User Request
     ↓
[Vector Memory] ← Stores ALL interactions
     ↓
[Router] → Retrieves relevant context
     ↓
Provider A (Claude) ← Gets full context
     ↓
[Vector Memory] ← Stores response
     ↓
[Router] → Next request goes to Provider B
     ↓
Provider B (OpenAI) ← Gets SAME context
```

### How It Works

1. **Every interaction is stored in vector memory**
   - User messages
   - Assistant responses
   - Tool calls and results
   - Orchestration task outputs

2. **Context is retrieved for every request**
   - Relevant messages are fetched based on query
   - Session history is maintained
   - Token limits are respected

3. **All providers share the same context**
   - Provider A's response informs Provider B's request
   - Tool results are visible to all subsequent calls
   - Orchestration tasks can reference any prior interaction

## Architecture Layers

### 1. Provider Layer (`internal/providers/`)
Multiple LLM provider implementations:
- Anthropic (Claude)
- OpenAI (Codex)
- Google (Gemini)
- Qwen
- Ollama Cloud
- NanoGPT (fallback)

**Note**: Each provider has model-specific context limits. The vector memory system AUGMENTS these limits by providing selective context.

### 2. Memory Layer (`internal/memory/`)
Unified context storage:
- **Vector Memory**: Stores all interactions with semantic search capability
- **Session Management**: Tracks conversation flows
- **Token Estimation**: Manages context window limits
- **Relevance Scoring**: Retrieves most relevant prior context

**Current Implementation**: Lexical search (keyword matching)
**Planned**: Real vector embeddings for semantic similarity

### 3. Router Layer (`internal/router/`)
Intelligent request routing:
- **Provider Selection**: Choose best available provider
- **Circuit Breaker**: Detect and avoid failing providers
- **Fallback Chain**: Automatic failover on errors
- **Context Injection**: Augment requests with memory

### 4. Orchestration Layer (`internal/orchestration/`)
Multi-step task execution:
- **Tasks**: Complex workflows across multiple LLM calls
- **Agents**: Specialized workers with role-based capabilities
- **Swarms**: Parallel agent coordination
- **Workflows**: Template-based execution patterns

All orchestration operations **share the same vector memory**, enabling:
- Task results inform subsequent tasks
- Agent collaboration through shared context
- Workflow steps build on previous outputs

## Context Flow Example

### Without Shared Context (Traditional Proxy)
```
User: "Analyze this data: [100 rows]"
  → Provider A (Claude): Analyzes data

User: "Now create a visualization"
  → Provider B (OpenAI): ❌ Doesn't know what data to visualize
```

### With Shared Context (Synapse Router)
```
User: "Analyze this data: [100 rows]"
  → Provider A (Claude): Analyzes data
  → [Vector Memory]: Stores data + analysis

User: "Now create a visualization"
  → [Vector Memory]: Retrieves data + analysis
  → Provider B (OpenAI): ✅ Creates visualization from stored context
```

## Memory System Details

### Storage Schema
```sql
CREATE TABLE memory (
    id INTEGER PRIMARY KEY,
    content TEXT,           -- Message content
    embedding BLOB,         -- Vector embedding (TODO: implement)
    timestamp DATETIME,     -- When stored
    session_id TEXT,        -- Conversation context
    role TEXT,              -- user/assistant/system/tool
    metadata TEXT           -- Additional context (JSON)
);
```

### Retrieval Strategies

1. **Recent Retrieval** (`RetrieveRecent`)
   - Get N most recent messages in session
   - Used when no specific query

2. **Relevant Retrieval** (`RetrieveRelevant`)
   - Score messages by keyword overlap
   - Rank by relevance + recency
   - Respect token limits

3. **Session History** (`GetSessionHistory`)
   - Full conversation for a session
   - Used for session resume/fork

### Token Management
- **Estimation**: ~4 characters per token
- **Trimming**: Keep most recent messages that fit
- **Prioritization**: Recent + relevant > old + irrelevant

## Provider Context Limits

**Important**: Provider context limits are MODEL-SPECIFIC:

- **Claude Sonnet 4.5**: 200K tokens
- **GPT-4 Turbo**: 128K tokens
- **Gemini 1.5 Pro**: 2M tokens
- **NanoGPT**: Varies by model

The vector memory system works WITHIN these limits by:
1. Selecting most relevant context from full history
2. Trimming to fit the target model's window
3. Preserving continuity across provider switches

## Orchestration Integration

Orchestration tasks leverage shared context:

```go
task := CreateTask("Research and summarize AI trends")
  ↓
Step 1 (researcher agent):
  - Searches web for AI trends
  - Results stored in vector memory
  ↓
Step 2 (analyst agent):
  - Retrieves research results from memory
  - Analyzes trends
  - Stores analysis in memory
  ↓
Step 3 (writer agent):
  - Retrieves both research + analysis
  - Writes comprehensive summary
```

Each step can use a DIFFERENT provider, but all share context.

## Routing Intelligence

### Provider Selection Logic
1. Check circuit breaker state
2. Check usage quota remaining
3. Check provider health
4. Select first available in priority order
5. On failure, try next in chain

### Context Augmentation
For every request:
1. Extract session ID
2. Query vector memory for relevant context
3. Prepend context to message history
4. Trim to fit target model's limit
5. Forward to selected provider

### Response Processing
After every response:
1. Store assistant message in vector memory
2. Track token usage
3. Update provider statistics
4. Return response + metadata

## Future Enhancements

### High Priority: Real Vector Embeddings
Replace lexical search with semantic similarity:
- Generate embeddings for all stored messages
- Use cosine similarity for retrieval
- Support cross-lingual context matching
- Better relevance scoring

**Implementation Options**:
1. Local embedding model (sentence-transformers)
2. OpenAI embeddings API
3. Dedicated embedding service

### Medium Priority: Context Compression
Intelligently summarize old context:
- Compress old messages while preserving meaning
- Hierarchical summarization for long sessions
- Configurable compression ratios

### Advanced: Multi-Modal Context
Extend beyond text:
- Store image descriptions/embeddings
- Audio transcription context
- File content indexing
- Code snippet tracking

## Performance Characteristics

### Memory Overhead
- Per message: ~1KB (text only, no embedding yet)
- Per session (100 messages): ~100KB
- 10K sessions: ~1GB database

### Latency Impact
- Memory retrieval: <10ms (SQLite)
- Context scoring: <50ms (lexical)
- Total overhead: ~60ms per request

With embeddings:
- Embedding generation: ~100ms per message
- Vector search: ~50ms per query
- Total: ~150ms overhead

### Scalability
Current (SQLite):
- Single instance only
- ~10K sessions max
- ~1M messages max

Future (PostgreSQL + pgvector):
- Distributed deployment
- Unlimited sessions
- Billions of messages

## Monitoring and Debugging

### Key Metrics
- **Context Hit Rate**: % of requests using memory context
- **Average Context Size**: Tokens per augmented request
- **Memory Growth**: New entries per hour
- **Retrieval Latency**: Time to fetch context

### Debug Endpoints
- `GET /v1/memory/session/{session_id}` - View session history
- `GET /v1/audit/session/{session_id}` - See provider decisions
- `POST /v1/debug/trace` - Enable request tracing

## Summary

**Synapse Router is not just a proxy - it's a context continuity layer.**

The vector memory system enables:
- ✅ Seamless provider switching without context loss
- ✅ Multi-turn conversations across different LLMs
- ✅ Tool results accessible to all providers
- ✅ Orchestration tasks building on prior work
- ✅ Session resume and fork capabilities

**The goal**: Make the underlying provider transparent to the user, while maintaining a unified, persistent conversational context.
