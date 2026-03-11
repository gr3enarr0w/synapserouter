package subscriptions

// ModelInfo captures model metadata for /v1/models.
type ModelInfo struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	OwnedBy     string `json:"owned_by"`
	Context     int    `json:"context"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	Category    string `json:"category"`
}
