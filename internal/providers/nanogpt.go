package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// NanoGPTProvider for NanoGPT API
type NanoGPTProvider struct {
	BaseProvider
	client       *http.Client
	modelMu      sync.Mutex
	cachedModels []map[string]interface{}
	modelsExp    time.Time
}

func NewNanoGPTProvider(baseURL, apiKey string) *NanoGPTProvider {
	if baseURL == "" {
		baseURL = "https://nano-gpt.com/api/v1"
	}
	// Ensure base URL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &NanoGPTProvider{
		BaseProvider: BaseProvider{
			name:       "nanogpt",
			baseURL:    baseURL,
			apiKey:     apiKey,
			maxContext: 2000000, // Model-dependent context (check per model)
			timeout:    120 * time.Second,
		},
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *NanoGPTProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	// NanoGPT doesn't support "auto" — default to Qwen (best open model, subscription-included)
	if req.Model == "" || strings.EqualFold(req.Model, "auto") {
		req.Model = "qwen/qwen3.5-plus"
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.baseURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("nanogpt request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("nanogpt error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse nanogpt response: %w", err)
	}

	return chatResp, nil
}

func (p *NanoGPTProvider) SupportsModel(model string) bool {
	return true // NanoGPT is a catch-all
}

// ListModels queries the NanoGPT API for all available models.
func (p *NanoGPTProvider) ListModels() []map[string]interface{} {
	p.modelMu.Lock()
	if p.cachedModels != nil && time.Now().Before(p.modelsExp) {
		models := p.cachedModels
		p.modelMu.Unlock()
		return models
	}
	p.modelMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(body, &listing) != nil {
		return nil
	}

	models := make([]map[string]interface{}, 0)
	for _, m := range listing.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		tier := nanoGPTModelTier(id)
		if tier == "" {
			continue
		}
		models = append(models, map[string]interface{}{
			"id":       id,
			"object":   "model",
			"owned_by": "nanogpt",
			"context":  2000000,
			"tier":     tier,
		})
	}

	p.modelMu.Lock()
	p.cachedModels = models
	p.modelsExp = time.Now().Add(10 * time.Minute)
	p.modelMu.Unlock()

	log.Printf("[NanoGPT] discovered %d models", len(models))
	return models
}

// NanoGPT model tier classification
const (
	NanoGPTTierSubscription = "subscription" // included in NanoGPT subscription
	NanoGPTTierAPI          = "api"          // pay-per-use, costs extra
)

// nanoGPTTierPrefixes maps model prefixes to their billing tier.
var nanoGPTTierPrefixes = map[string]string{
	// Subscription-included models
	// Qwen
	"qwen3.5-": NanoGPTTierSubscription, "qwen/qwen3.5-": NanoGPTTierSubscription,
	"qwen3-max": NanoGPTTierSubscription, "qwen/qwen3-max": NanoGPTTierSubscription,
	"qwen-max": NanoGPTTierSubscription, "qwen/qwen3-coder": NanoGPTTierSubscription,
	"qwen3-coder": NanoGPTTierSubscription,
	// GLM / Zhipu
	"glm-5": NanoGPTTierSubscription, "glm-4.7": NanoGPTTierSubscription, "glm-4.6": NanoGPTTierSubscription,
	"zai-org/glm-5": NanoGPTTierSubscription, "zai-org/glm-4.7": NanoGPTTierSubscription, "zai-org/glm-4.6": NanoGPTTierSubscription,
	"tee/glm-5": NanoGPTTierSubscription, "tee/glm-4.7": NanoGPTTierSubscription, "tee/glm-4.6": NanoGPTTierSubscription,
	// Kimi / Moonshot
	"kimi-k2.5": NanoGPTTierSubscription, "kimi-k2-": NanoGPTTierSubscription,
	"moonshotai/kimi-k2.5": NanoGPTTierSubscription, "moonshotai/kimi-k2": NanoGPTTierSubscription,
	"tee/kimi-k2": NanoGPTTierSubscription,
	// MiniMax
	"minimax-m2": NanoGPTTierSubscription, "minimax/minimax-m2": NanoGPTTierSubscription, "tee/minimax-m2": NanoGPTTierSubscription,
	// DeepSeek
	"deepseek-r1": NanoGPTTierSubscription, "deepseek-ai/deepseek-r1": NanoGPTTierSubscription,
	"deepseek-ai/deepseek-v3": NanoGPTTierSubscription, "deepseek/deepseek-v3": NanoGPTTierSubscription,
	"tee/deepseek": NanoGPTTierSubscription, "deepseek-chat": NanoGPTTierSubscription, "deepseek-reasoner": NanoGPTTierSubscription,
	// Step
	"step-3": NanoGPTTierSubscription, "stepfun-ai/step-3": NanoGPTTierSubscription,

	// API (pay-per-use) models
	"grok-4": NanoGPTTierAPI, "x-ai/grok-4": NanoGPTTierAPI,
	"openai/gpt-5": NanoGPTTierAPI, "openai/o3": NanoGPTTierAPI, "openai/o4": NanoGPTTierAPI,
	"anthropic/claude-opus-4.6": NanoGPTTierAPI, "anthropic/claude-sonnet-4.6": NanoGPTTierAPI,
	"google/gemini-3": NanoGPTTierAPI,
}

func nanoGPTModelTier(id string) string {
	lower := strings.ToLower(id)
	for prefix, tier := range nanoGPTTierPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return tier
		}
	}
	return ""
}

func isNanoGPTFlagshipModel(id string) bool {
	return nanoGPTModelTier(id) != ""
}

func (p *NanoGPTProvider) IsHealthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return false
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
