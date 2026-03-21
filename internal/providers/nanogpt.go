package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// NanoGPTProvider for NanoGPT API
type NanoGPTProvider struct {
	BaseProvider
	client       *http.Client
	tier         string // "subscription" or "paid"
	modelMu      sync.Mutex
	cachedModels []map[string]interface{}
	modelsExp    time.Time
}

// NewNanoGPTProvider creates a NanoGPT provider for the given tier.
// tier must be "subscription" or "paid".
func NewNanoGPTProvider(apiKey, tier string) *NanoGPTProvider {
	var name, baseURL string
	switch tier {
	case "subscription":
		name = "nanogpt-sub"
		baseURL = "https://nano-gpt.com/api/subscription/v1"
	case "paid":
		name = "nanogpt-paid"
		baseURL = "https://nano-gpt.com/api/paid/v1"
	default:
		name = "nanogpt-sub"
		baseURL = "https://nano-gpt.com/api/subscription/v1"
	}

	// Allow env override for testing
	if envURL := os.Getenv("NANOGPT_BASE_URL"); envURL != "" {
		baseURL = envURL
	}

	baseURL = strings.TrimSuffix(baseURL, "/")

	return &NanoGPTProvider{
		BaseProvider: BaseProvider{
			name:       name,
			baseURL:    baseURL,
			apiKey:     apiKey,
			maxContext: 0, // determined per-model via MaxContextTokens()
			timeout:    120 * time.Second,
		},
		client: NewLLMClient(120 * time.Second),
		tier:   tier,
	}
}

func (p *NanoGPTProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	if req.Model == "" || strings.EqualFold(req.Model, "auto") {
		if p.tier == "paid" {
			req.Model = "chatgpt-4o-latest"
		} else {
			req.Model = "qwen/qwen3.5-397b-a17b"
		}
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
	if model == "" || strings.EqualFold(model, "auto") {
		// Auto requests go to subscription tier only; paid tier is reachable via fallback
		return p.tier == "subscription"
	}
	tier := nanoGPTModelTier(model)
	switch p.tier {
	case "subscription":
		return tier == NanoGPTTierSubscription
	case "paid":
		return tier == NanoGPTTierAPI
	}
	return false
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
			"context":  nanoGPTModelContext(id),
			"tier":     tier,
		})
	}

	p.modelMu.Lock()
	p.cachedModels = models
	p.modelsExp = time.Now().Add(10 * time.Minute)
	p.modelMu.Unlock()

	log.Printf("[%s] discovered %d models", p.name, len(models))
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

// MaxContextTokens returns the context limit for this tier's default model.
// NanoGPT proxies to different upstream models, each with its own limit.
func (p *NanoGPTProvider) MaxContextTokens() int {
	if p.tier == "paid" {
		return nanoGPTModelContext("chatgpt-4o-latest")
	}
	return nanoGPTModelContext("qwen/qwen3.5-397b-a17b")
}

// NanoGPTModelContext returns the context token limit for a specific NanoGPT model.
func NanoGPTModelContext(model string) int {
	return nanoGPTModelContext(model)
}

// nanoGPTModelContextLimits maps model prefixes to their real context limits.
var nanoGPTModelContextLimits = map[string]int{
	// Subscription models
	"qwen/qwen3.5-":          131072,  // Qwen 3.5: 128K
	"qwen3.5-":               131072,
	"qwen-max":               32768,   // Qwen Max: 32K
	"qwen/qwen3-max":         32768,
	"qwen3-max":              32768,
	"qwen/qwen3-coder":       131072,  // Qwen 3 Coder: 128K
	"qwen3-coder":            131072,
	"deepseek-r1":            65536,   // DeepSeek R1: 64K
	"deepseek-ai/deepseek-r1": 65536,
	"deepseek-chat":          65536,   // DeepSeek V3: 64K
	"deepseek-ai/deepseek-v3": 65536,
	"deepseek-reasoner":      65536,
	"deepseek/deepseek-v3":   65536,
	"kimi-k2.5":              131072,  // Kimi K2.5: 128K
	"moonshotai/kimi-k2.5":   131072,
	"kimi-k2-":               131072,  // Kimi K2: 128K
	"moonshotai/kimi-k2":     131072,
	"glm-5":                  131072,  // GLM-5: 128K
	"zai-org/glm-5":          131072,
	"glm-4.7":                131072,
	"zai-org/glm-4.7":        131072,
	"glm-4.6":                131072,
	"zai-org/glm-4.6":        131072,
	"minimax-m2":             1048576, // MiniMax M2: 1M
	"minimax/minimax-m2":     1048576,
	"step-3":                 131072,  // Step-3: 128K
	"stepfun-ai/step-3":      131072,

	// API (paid) models
	"chatgpt-4o-latest":             128000,  // GPT-4o: 128K
	"openai/gpt-5":                  256000,  // GPT-5: 256K
	"openai/o3":                     200000,  // O3: 200K
	"openai/o4":                     200000,  // O4: 200K
	"anthropic/claude-opus-4.6":     200000,  // Claude Opus: 200K
	"anthropic/claude-sonnet-4.6":   200000,  // Claude Sonnet: 200K
	"google/gemini-3":               1048576, // Gemini 3: 1M
	"grok-4":                        131072,  // Grok-4: 128K
	"x-ai/grok-4":                   131072,
}

func nanoGPTModelContext(model string) int {
	lower := strings.ToLower(model)

	// Exact match first
	if ctx, ok := nanoGPTModelContextLimits[lower]; ok {
		return ctx
	}

	// Prefix match
	for prefix, ctx := range nanoGPTModelContextLimits {
		if strings.HasPrefix(lower, prefix) {
			return ctx
		}
	}

	// Conservative default for unknown models
	return 128000
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
