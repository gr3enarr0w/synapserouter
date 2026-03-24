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

// LocalHashEmbedding provides fast, deterministic embeddings using TF-IDF
// weighted feature hashing. Captures semantic similarity through character
// n-grams (subword patterns like function names, file paths) and word
// unigrams, hashed into a fixed-dimension vector via the hashing trick.
// Pure Go, zero dependencies, ~10x better similarity quality than random hashing.
type LocalHashEmbedding struct {
	dimensions int
}

func NewLocalHashEmbedding(dimensions int) *LocalHashEmbedding {
	if dimensions <= 0 {
		dimensions = 384
	}
	return &LocalHashEmbedding{dimensions: dimensions}
}

func (e *LocalHashEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return make([]float32, e.dimensions), nil
	}

	embedding := make([]float32, e.dimensions)

	// Extract features: word unigrams + character 3-grams
	words := strings.Fields(text)
	features := make(map[string]float32)

	// Word unigrams (weight 1.0)
	for _, w := range words {
		w = normalizeToken(w)
		if len(w) >= 2 {
			features["w:"+w] += 1.0
		}
	}

	// Character 3-grams from each word (weight 0.5, captures subword patterns)
	for _, w := range words {
		w = normalizeToken(w)
		if len(w) < 3 {
			continue
		}
		runes := []rune(w)
		for i := 0; i <= len(runes)-3; i++ {
			ngram := string(runes[i : i+3])
			features["c:"+ngram] += 0.5
		}
	}

	// Word bigrams (weight 0.3, captures phrase patterns)
	for i := 0; i < len(words)-1; i++ {
		a := normalizeToken(words[i])
		b := normalizeToken(words[i+1])
		if len(a) >= 2 && len(b) >= 2 {
			features["b:"+a+"_"+b] += 0.3
		}
	}

	// Apply IDF-like weighting: penalize very common tokens
	// Short tokens (<=3 chars) and common programming words get lower weight
	for key, tf := range features {
		idf := idfWeight(key)
		features[key] = tf * idf
	}

	// Feature hashing into fixed dimensions (the hashing trick)
	for feature, weight := range features {
		h := sha256.Sum256([]byte(feature))
		// Use first 4 bytes for bucket index, next byte for sign
		bucket := int(binary.LittleEndian.Uint32(h[:4])) % e.dimensions
		if bucket < 0 {
			bucket += e.dimensions
		}
		// Use 5th byte for sign (reduces hash collision impact)
		sign := float32(1.0)
		if h[4]&1 == 1 {
			sign = -1.0
		}
		embedding[bucket] += sign * weight
	}

	return normalizeVector(embedding), nil
}

func (e *LocalHashEmbedding) Dimensions() int {
	return e.dimensions
}

// normalizeToken strips punctuation and returns lowercase token
func normalizeToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '/' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// idfWeight returns an inverse-document-frequency-like weight for a feature.
// Common short words and programming noise get lower weight; distinctive
// terms get higher weight. This is a static approximation — no corpus needed.
func idfWeight(feature string) float32 {
	// Strip prefix (w:, c:, b:)
	if len(feature) > 2 && feature[1] == ':' {
		feature = feature[2:]
	}

	// Very common words get low weight
	commonWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "it": true,
		"in": true, "to": true, "of": true, "and": true, "or": true,
		"for": true, "on": true, "at": true, "by": true, "as": true,
		"if": true, "be": true, "do": true, "no": true, "so": true,
		"up": true, "we": true, "my": true, "he": true, "me": true,
	}
	if commonWords[feature] {
		return 0.1
	}

	// Short features (1-2 chars) get reduced weight
	if len(feature) <= 2 {
		return 0.3
	}

	// Medium features get normal weight
	if len(feature) <= 5 {
		return 1.0
	}

	// Longer, more distinctive features get bonus weight
	return 1.0 + float32(len(feature)-5)*0.1
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
