//go:build onnx

package memory

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

	// Read the model file
	modelBytes, err := os.ReadFile(modelPath)
	if err != nil {
		log.Printf("[ONNX] Failed to read model file: %v", err)
		return nil
	}

	// all-MiniLM-L6-v2 model has known input/output names
	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}
	
	// Create input/output value slices (will be populated during inference)
	inputs := make([]ort.Value, len(inputNames))
	outputs := make([]ort.Value, len(outputNames))

	session, err := ort.NewAdvancedSessionWithONNXData(modelBytes, inputNames, outputNames, inputs, outputs, nil)
	if err != nil {
		log.Printf("[ONNX] Session error: %v", err)
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

	// Get output tensor
	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(len(inputIDs)), int64(e.dimensions)))
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
	outputData := outputTensor.GetData()
	return meanPool(outputData, len(inputIDs), e.dimensions), nil
}

func (e *ONNXEmbedding) Dimensions() int {
	return e.dimensions
}

func (e *ONNXEmbedding) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}
	return nil
}

// tokenize converts text to token IDs using a basic vocabulary.
// Returns inputIDs and attentionMask padded to maxLen.
func (e *ONNXEmbedding) tokenize(text string, maxLen int) ([]int64, []int64) {
	// Lowercase and split on whitespace
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	// [CLS] tokens... [SEP]
	inputIDs := []int64{101} // [CLS]
	attentionMask := []int64{1}

	for _, word := range words {
		if id, ok := e.vocab[word]; ok {
			inputIDs = append(inputIDs, int64(id))
			attentionMask = append(attentionMask, 1)
		} else {
			// Unknown token
			inputIDs = append(inputIDs, 100) // [UNK]
			attentionMask = append(attentionMask, 1)
		}
		if len(inputIDs) >= maxLen-1 {
			break
		}
	}

	inputIDs = append(inputIDs, 102) // [SEP]
	attentionMask = append(attentionMask, 1)

	// Pad to maxLen
	for len(inputIDs) < maxLen {
		inputIDs = append(inputIDs, 0)
		attentionMask = append(attentionMask, 0)
	}

	return inputIDs[:maxLen], attentionMask[:maxLen]
}

func meanPool(data []float32, seqLen, dimensions int) []float32 {
	result := make([]float32, dimensions)
	for i := 0; i < seqLen; i++ {
		for j := 0; j < dimensions; j++ {
			result[j] += data[i*dimensions+j] / float32(seqLen)
		}
	}
	return result
}

// buildBasicVocab creates a minimal vocabulary for all-MiniLM-L6-v2.
// In production, load from vocab.txt instead.
func buildBasicVocab() map[string]int32 {
	return make(map[string]int32)
}

// findONNXRuntimeLib locates the ONNX Runtime shared library.
func findONNXRuntimeLib() string {
	// Check common installation paths
	candidates := []string{
		"/usr/local/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/libonnxruntime.so",
		"/opt/homebrew/lib/libonnxruntime.dylib",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "libonnxruntime.so" // fallback — ort will fail gracefully if not found
}

// EnsureModelDownload downloads the all-MiniLM-L6-v2 ONNX model if missing.
// Returns the path to the model file.
func EnsureModelDownload() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}

	modelDir := filepath.Join(homeDir, ".synroute", "models")
	modelPath := filepath.Join(modelDir, "all-MiniLM-L6-v2.onnx")

	// Check if model already exists
	if _, err := os.Stat(modelPath); err == nil {
		return modelPath, nil
	}

	// Create directory
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return "", fmt.Errorf("create model directory: %w", err)
	}

	// Download model
	log.Printf("[ONNX] Downloading model to %s", modelPath)
	modelURL := "https://huggingface.co/onnx-models/all-MiniLM-L6-v2-onnx/resolve/main/model.onnx"
	
	resp, err := http.Get(modelURL)
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Write to file
	out, err := os.Create(modelPath)
	if err != nil {
		return "", fmt.Errorf("create model file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("write model file: %w", err)
	}

	log.Printf("[ONNX] Model downloaded successfully")
	return modelPath, nil
}