package memory

import (
	"context"
	"math"
	"testing"
)

func TestONNXAvailable(t *testing.T) {
	// Without onnx build tag, should return false
	if ONNXAvailable() {
		t.Log("ONNX is available (built with -tags=onnx)")
	} else {
		t.Log("ONNX is not available (built without onnx tag)")
	}
}

func TestNewONNXEmbeddingStub(t *testing.T) {
	if ONNXAvailable() {
		t.Skip("ONNX is available, testing real implementation")
	}

	provider := NewONNXEmbedding("/nonexistent/model.onnx")
	if provider != nil {
		t.Fatal("stub should return nil")
	}
}

func TestEmbeddingProviderInterface(t *testing.T) {
	// Verify that LocalHashEmbedding satisfies EmbeddingProvider
	var _ EmbeddingProvider = NewLocalHashEmbedding(384)
}

func TestSemanticSimilarityBaseline(t *testing.T) {
	// This test documents the current hash embedding similarity scores
	// as a baseline. ONNX embeddings should improve these.
	e := NewLocalHashEmbedding(384)
	ctx := context.Background()

	v1, _ := e.Embed(ctx, "fix authentication")
	v2, _ := e.Embed(ctx, "repair login")
	v3, _ := e.Embed(ctx, "database migration")

	sim12 := cosineSim(v1, v2) // should be high with real embeddings
	sim13 := cosineSim(v1, v3) // should be low

	t.Logf("'fix authentication' vs 'repair login': %.3f (target: >0.7 with ONNX)", sim12)
	t.Logf("'fix authentication' vs 'database migration': %.3f (target: <0.3 with ONNX)", sim13)

	// Hash embeddings won't achieve the ONNX targets, but should at least
	// produce valid normalized vectors
	if math.IsNaN(float64(sim12)) || math.IsNaN(float64(sim13)) {
		t.Fatal("similarity produced NaN")
	}
}

func cosineSim(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
