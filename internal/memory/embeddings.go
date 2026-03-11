package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// EmbeddingProvider generates vector embeddings for text
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
}

// LocalHashEmbedding provides fast, deterministic embeddings using hashing
// This is a lightweight fallback when no proper embedding model is available
type LocalHashEmbedding struct {
	dimensions int
}

func NewLocalHashEmbedding(dimensions int) *LocalHashEmbedding {
	if dimensions <= 0 {
		dimensions = 384 // Default dimension
	}
	return &LocalHashEmbedding{dimensions: dimensions}
}

func (e *LocalHashEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	// Normalize text
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return make([]float32, e.dimensions), nil
	}

	// Generate embedding using simhash-inspired approach
	embedding := make([]float32, e.dimensions)

	// Hash text with multiple seeds for diversity
	seeds := []string{text, reverse(text), strings.Join(strings.Fields(text), "")}

	for i, seed := range seeds {
		hash := sha256.Sum256([]byte(seed + fmt.Sprintf("_salt_%d", i)))

		// Convert hash bytes to float values
		for j := 0; j < e.dimensions; j++ {
			byteIdx := (j * len(hash)) / e.dimensions
			if byteIdx < len(hash) {
				// Normalize to [-1, 1]
				embedding[j] += (float32(hash[byteIdx]) / 128.0) - 1.0
			}
		}
	}

	// Normalize the embedding vector
	return normalizeVector(embedding), nil
}

func (e *LocalHashEmbedding) Dimensions() int {
	return e.dimensions
}

// OpenAIEmbedding uses OpenAI's embeddings API
type OpenAIEmbedding struct {
	apiKey     string
	model      string
	client     *http.Client
	dimensions int
	cache      sync.Map
}

func NewOpenAIEmbedding(apiKey string) *OpenAIEmbedding {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	return &OpenAIEmbedding{
		apiKey:     apiKey,
		model:      "text-embedding-3-small", // 1536 dimensions
		client:     &http.Client{Timeout: 30 * time.Second},
		dimensions: 1536,
		cache:      sync.Map{},
	}
}

type openAIEmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (e *OpenAIEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	if e.apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	// Check cache
	if cached, ok := e.cache.Load(text); ok {
		return cached.([]float32), nil
	}

	reqBody, _ := json.Marshal(openAIEmbeddingRequest{
		Input: text,
		Model: e.model,
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/embeddings",
		bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI embeddings API error %d: %s", resp.StatusCode, string(body))
	}

	var result openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	embedding := result.Data[0].Embedding

	// Cache the result
	e.cache.Store(text, embedding)

	return embedding, nil
}

func (e *OpenAIEmbedding) Dimensions() int {
	return e.dimensions
}

// CosineSimilarity computes the cosine similarity between two vectors
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	normA = float32(math.Sqrt(float64(normA)))
	normB = float32(math.Sqrt(float64(normB)))

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (normA * normB)
}

// normalizeVector normalizes a vector to unit length
func normalizeVector(v []float32) []float32 {
	var norm float32
	for _, val := range v {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm == 0 {
		return v
	}

	normalized := make([]float32, len(v))
	for i, val := range v {
		normalized[i] = val / norm
	}
	return normalized
}

// reverse reverses a string
func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// EncodeEmbedding converts float32 slice to bytes for storage
func EncodeEmbedding(embedding []float32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, embedding)
	return buf.Bytes()
}

// DecodeEmbedding converts bytes back to float32 slice
func DecodeEmbedding(data []byte) ([]float32, error) {
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding data length")
	}

	embedding := make([]float32, len(data)/4)
	buf := bytes.NewReader(data)
	err := binary.Read(buf, binary.LittleEndian, &embedding)
	return embedding, err
}
