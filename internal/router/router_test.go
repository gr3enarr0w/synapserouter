package router

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"
)

type testProvider struct{}

func (p *testProvider) Name() string { return "nanogpt" }

func (p *testProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	return providers.ChatResponse{
		ID:      "resp-1",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "nanogpt-test-model",
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:    "assistant",
					Content: "ok",
				},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (p *testProvider) IsHealthy(ctx context.Context) bool { return true }
func (p *testProvider) MaxContextTokens() int              { return 2000000 }

func TestChatCompletionWithDebugIncludesMemoryCandidates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "router.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := applyRouterTestSchema(db); err != nil {
		t.Fatal(err)
	}

	tracker, err := usage.NewTracker(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Close()

	vm := memory.NewVectorMemory(db)
	r := NewRouter([]providers.Provider{&testProvider{}}, tracker, vm, db)

	req := providers.ChatRequest{
		Model: "gpt-test",
		Messages: []providers.Message{
			{Role: "user", Content: "Need help with go fallback routing"},
		},
	}

	resp, err := r.ChatCompletionWithDebug(context.Background(), req, "session-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if resp.XProxyMetadata == nil {
		t.Fatal("expected debug metadata")
	}
	if resp.XProxyMetadata.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %s", resp.XProxyMetadata.SessionID)
	}
	if resp.XProxyMetadata.MemoryCandidateCount == 0 {
		t.Fatal("expected memory candidates")
	}
}

func TestTraceDecisionSelectsHealthyProvider(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := applyRouterTestSchema(db); err != nil {
		t.Fatal(err)
	}

	tracker, err := usage.NewTracker(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Close()

	vm := memory.NewVectorMemory(db)
	r := NewRouter([]providers.Provider{&testProvider{}}, tracker, vm, db)

	req := providers.ChatRequest{
		Model: "gpt-test",
		Messages: []providers.Message{
			{Role: "user", Content: "show provider trace for fallback"},
		},
	}

	trace, err := r.TraceDecision(context.Background(), req, "session-trace")
	if err != nil {
		t.Fatal(err)
	}
	if trace.SelectedProvider != "nanogpt" {
		t.Fatalf("unexpected selected provider: %s", trace.SelectedProvider)
	}
	if len(trace.Providers) != 1 {
		t.Fatalf("expected 1 provider entry, got %d", len(trace.Providers))
	}
	if !trace.Providers[0].Selected {
		t.Fatal("expected provider to be marked selected")
	}
}

func applyRouterTestSchema(db *sql.DB) error {
	schemaPath := filepath.Join("..", "..", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(sqlBytes))
	return err
}
