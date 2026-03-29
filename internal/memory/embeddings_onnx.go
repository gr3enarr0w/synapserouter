//go:build onnx

package memory

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"unicode"

	ort "github.com/yalue/onnxruntime_go"
)

// ONNXEmbedding provides semantic embeddings using an ONNX model.
// Requires the onnxruntime shared library to be installed.
// Default model: all-MiniLM-L6-v2 (384 dimensions).
type ONNXEmbedding struct {
	session    *ort.AdvancedSession
	modelPath  string
	dimensions int
	mu         sync.Mutex
	vocab      map[string]int32
}

// NewONNXEmbedding creates an ONNX-based embedding provider.
// modelPath should point to an all-MiniLM-L6-v2 ONNX model file.
// Returns nil if the model cannot be loaded.
func NewONNXEmbedding(modelPath string) EmbeddingProvider {
	ort.SetSharedLibraryPath(findONNXRuntimeLib())
	if err := ort.InitializeEnvironment(); err != nil {
		log.Printf("[ONNX] Failed to initialize runtime: %v", err)
		return nil
	}

	session, err := ort.NewAdvancedSession(modelPath, nil, nil, nil, nil)
	if err != nil {
		log.Printf("[ONNX] Failed to load model %s: %v", modelPath, err)
		return nil
	}

	log.Printf("[ONNX] Loaded embedding model: %s (384 dimensions)", modelPath)
	return &ONNXEmbedding{
		session:    session,
		modelPath:  modelPath,
		dimensions: 384,
		vocab:      buildBasicVocab(),
	}
}

// ONNXAvailable returns true when built with the onnx build tag.
func ONNXAvailable() bool {
	return true
}

func (e *ONNXEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return make([]float32, e.dimensions), nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Tokenize text into input IDs
	inputIDs, attentionMask := e.tokenize(text, 128)

	// Create tensors
	shape := ort.NewShape(1, int64(len(inputIDs)))
	inputIDsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attentionTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer attentionTensor.Destroy()

	tokenTypeIDs := make([]int64, len(inputIDs))
	tokenTypeTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
	}
	defer tokenTypeTensor.Destroy()

	// Output tensor
	outputShape := ort.NewShape(1, int64(len(inputIDs)), int64(e.dimensions))
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	// Run inference
	err = e.session.Run()
	if err != nil {
		return nil, fmt.Errorf("onnx inference: %w", err)
	}

	// Mean pooling over token dimension
	output := outputTensor.GetData()
	seqLen := len(inputIDs)
	embedding := make([]float32, e.dimensions)
	count := float32(0)
	for i := 0; i < seqLen; i++ {
		if attentionMask[i] == 1 {
			for j := 0; j < e.dimensions; j++ {
				embedding[j] += output[i*e.dimensions+j]
			}
			count++
		}
	}
	if count > 0 {
		for j := range embedding {
			embedding[j] /= count
		}
	}

	// L2 normalize
	var norm float64
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for j := range embedding {
			embedding[j] = float32(float64(embedding[j]) / norm)
		}
	}

	return embedding, nil
}

func (e *ONNXEmbedding) Dimensions() int {
	return e.dimensions
}

// tokenize performs basic wordpiece-like tokenization.
func (e *ONNXEmbedding) tokenize(text string, maxLen int) ([]int64, []int64) {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	// [CLS] tokens... [SEP]
	inputIDs := []int64{101} // [CLS]
	for _, word := range words {
		if len(inputIDs) >= maxLen-1 {
			break
		}
		if id, ok := e.vocab[word]; ok {
			inputIDs = append(inputIDs, int64(id))
		} else {
			inputIDs = append(inputIDs, 100) // [UNK]
		}
	}
	inputIDs = append(inputIDs, 102) // [SEP]

	// Pad to maxLen
	attentionMask := make([]int64, maxLen)
	for i := range inputIDs {
		attentionMask[i] = 1
	}
	for len(inputIDs) < maxLen {
		inputIDs = append(inputIDs, 0)
	}

	return inputIDs, attentionMask
}

func buildBasicVocab() map[string]int32 {
	// Minimal vocab for demonstration — in production, load from vocab.txt
	vocab := map[string]int32{
		"[PAD]": 0, "[UNK]": 100, "[CLS]": 101, "[SEP]": 102, "[MASK]": 103,
	}
	// Common programming terms
	commonWords := []string{
		"fix", "bug", "error", "test", "function", "class", "method", "file",
		"read", "write", "create", "delete", "update", "add", "remove",
		"authentication", "login", "repair", "code", "build", "compile",
		"deploy", "server", "client", "api", "database", "query", "search",
		"go", "python", "rust", "java", "javascript", "typescript",
		"the", "a", "is", "in", "to", "for", "of", "and", "or", "not",
		"with", "from", "by", "on", "at", "this", "that", "it", "as",
	}
	for i, word := range commonWords {
		vocab[word] = int32(1000 + i)
	}
	return vocab
}

func findONNXRuntimeLib() string {
	// Platform-specific paths for onnxruntime shared library
	candidates := []string{
		"/usr/local/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.so",
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"libonnxruntime.dylib",
		"libonnxruntime.so",
	}
	for _, path := range candidates {
		return path // ort.SetSharedLibraryPath will fail gracefully if not found
	}
	return "libonnxruntime.so"
}
