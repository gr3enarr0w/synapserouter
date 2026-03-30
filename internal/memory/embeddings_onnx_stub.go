//go:build !onnx

package memory

// NewONNXEmbedding returns nil when built without the onnx build tag.
// The caller should fall back to LocalHashEmbedding.
func NewONNXEmbedding(modelPath string) EmbeddingProvider {
	return nil
}

// ONNXAvailable returns false when built without the onnx build tag.
func ONNXAvailable() bool {
	return false
}
