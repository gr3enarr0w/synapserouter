package memory

import (
	"context"
	"testing"
)

func TestLocalHashEmbedding_Deterministic(t *testing.T) {
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	v1, err := e.Embed(ctx, "go test -race ./...")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := e.Embed(ctx, "go test -race ./...")
	if err != nil {
		t.Fatal(err)
	}

	sim := CosineSimilarity(v1, v2)
	if sim < 0.999 {
		t.Errorf("same input should produce identical vectors, got similarity %f", sim)
	}
}

func TestLocalHashEmbedding_SimilarTextsSimilarVectors(t *testing.T) {
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	// Similar commands should be more similar to each other than to unrelated text
	v1, _ := e.Embed(ctx, "go test ./...")
	v2, _ := e.Embed(ctx, "go test -race ./...")
	v3, _ := e.Embed(ctx, "npm install express react")

	simSimilar := CosineSimilarity(v1, v2)
	simDifferent := CosineSimilarity(v1, v3)

	if simSimilar <= simDifferent {
		t.Errorf("'go test' should be more similar to 'go test -race' (%.3f) than to 'npm install' (%.3f)",
			simSimilar, simDifferent)
	}
	t.Logf("go_test vs go_test_race: %.3f | go_test vs npm_install: %.3f", simSimilar, simDifferent)
}

func TestLocalHashEmbedding_DissimilarTextsDistantVectors(t *testing.T) {
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	v1, _ := e.Embed(ctx, "Python flask API with PostgreSQL database")
	v2, _ := e.Embed(ctx, "Rust cargo build release binary")

	sim := CosineSimilarity(v1, v2)
	if sim > 0.5 {
		t.Errorf("dissimilar texts should have low similarity, got %.3f", sim)
	}
	t.Logf("python_flask vs rust_cargo: %.3f", sim)
}

func TestLocalHashEmbedding_CodePatterns(t *testing.T) {
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	// Same file referenced in different contexts should be similar
	v1, _ := e.Embed(ctx, "file_read path=/src/lib/crypto.ts")
	v2, _ := e.Embed(ctx, "file_write path=/src/lib/crypto.ts content=...")
	v3, _ := e.Embed(ctx, "bash command=cargo build --release")

	simSameFile := CosineSimilarity(v1, v2)
	simDifferent := CosineSimilarity(v1, v3)

	if simSameFile <= simDifferent {
		t.Errorf("same file path should be more similar (%.3f) than unrelated command (%.3f)",
			simSameFile, simDifferent)
	}
	t.Logf("crypto.ts read vs write: %.3f | crypto.ts vs cargo: %.3f", simSameFile, simDifferent)
}

func TestLocalHashEmbedding_EmptyText(t *testing.T) {
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	v, err := e.Embed(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(v))
	}
	// All zeros for empty text
	for i, val := range v {
		if val != 0 {
			t.Errorf("empty text should produce zero vector, got %f at index %d", val, i)
			break
		}
	}
}

func TestLocalHashEmbedding_ErrorMessages(t *testing.T) {
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	// Error messages about the same topic should cluster
	v1, _ := e.Embed(ctx, "npm install failed: ENOENT package.json not found")
	v2, _ := e.Embed(ctx, "npm install error: missing package.json in directory")
	v3, _ := e.Embed(ctx, "go build: cannot find module providing package fmt")

	simSameError := CosineSimilarity(v1, v2)
	simDiffError := CosineSimilarity(v1, v3)

	if simSameError <= simDiffError {
		t.Errorf("similar npm errors should cluster (%.3f) more than go error (%.3f)",
			simSameError, simDiffError)
	}
	t.Logf("npm errors: %.3f | npm vs go error: %.3f", simSameError, simDiffError)
}
