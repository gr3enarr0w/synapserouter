package agent

import (
	"bufio"
	"context"
	"io"
	"sync"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

// StreamCallback is called for each line of streaming output.
type StreamCallback func(line string)

// TokenCallback is called for each streamed token chunk from the LLM.
type TokenCallback func(token string)

// StreamingChatExecutor extends ChatExecutor with streaming support.
type StreamingChatExecutor interface {
	ChatExecutor
	ChatCompletionStream(ctx context.Context, req providers.ChatRequest, sessionID string, onToken TokenCallback) (providers.ChatResponse, error)
}

// StreamingTokenCollector collects tokens and emits them via a TokenCallback,
// also accumulating the full response for the agent loop.
type StreamingTokenCollector struct {
	mu       sync.Mutex
	tokens   []string
	callback TokenCallback
}

// NewStreamingTokenCollector creates a collector that forwards tokens to the callback.
func NewStreamingTokenCollector(callback TokenCallback) *StreamingTokenCollector {
	return &StreamingTokenCollector{callback: callback}
}

// OnToken handles an incoming token from the LLM stream.
func (c *StreamingTokenCollector) OnToken(token string) {
	c.mu.Lock()
	c.tokens = append(c.tokens, token)
	c.mu.Unlock()

	if c.callback != nil {
		c.callback(token)
	}
}

// FullText returns the accumulated tokens as a single string.
func (c *StreamingTokenCollector) FullText() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := 0
	for _, t := range c.tokens {
		total += len(t)
	}
	buf := make([]byte, 0, total)
	for _, t := range c.tokens {
		buf = append(buf, t...)
	}
	return string(buf)
}

// StreamWriter wraps an io.Writer and calls a callback for each line written.
type StreamWriter struct {
	mu       sync.Mutex
	buf      []byte
	callback StreamCallback
	writer   io.Writer // optional passthrough writer
}

// NewStreamWriter creates a writer that calls the callback for each complete line.
// If writer is non-nil, output is also passed through to it.
func NewStreamWriter(callback StreamCallback, writer io.Writer) *StreamWriter {
	return &StreamWriter{
		callback: callback,
		writer:   writer,
	}
}

// Write implements io.Writer, buffering and emitting complete lines.
func (sw *StreamWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Pass through to underlying writer
	if sw.writer != nil {
		if _, err := sw.writer.Write(p); err != nil {
			return 0, err
		}
	}

	sw.buf = append(sw.buf, p...)

	// Emit complete lines
	for {
		idx := -1
		for i, b := range sw.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}

		line := string(sw.buf[:idx])
		sw.buf = sw.buf[idx+1:]

		if sw.callback != nil {
			sw.callback(line)
		}
	}

	return len(p), nil
}

// Flush emits any remaining buffered content.
func (sw *StreamWriter) Flush() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.buf) > 0 && sw.callback != nil {
		sw.callback(string(sw.buf))
		sw.buf = nil
	}
}

// LineScanner reads from a reader and calls the callback for each line.
// Blocks until the reader is exhausted or context is cancelled.
func LineScanner(r io.Reader, callback StreamCallback) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		callback(scanner.Text())
	}
}
