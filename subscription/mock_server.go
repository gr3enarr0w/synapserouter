package subscription

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// MockSubscriptionServer provides a local HTTP server that mimics the subscription API
type MockSubscriptionServer struct {
	server *http.Server
	models []ModelDefinition
}

// NewMockSubscriptionServer creates a new mock subscription server with default models
func NewMockSubscriptionServer() *MockSubscriptionServer {
	now := time.Now()
	models := []ModelDefinition{
		{
			ID:             "gpt-4-turbo",
			Name:           "GPT-4 Turbo",
			DisplayName:    "GPT-4 Turbo (Subscription)",
			Description:    "High-performance model for complex tasks",
			Status:         "available",
			Roles:          []string{"developer", "researcher", "analyst"},
			CreatedAt:      &now,
			MaxConcurrency: 5,
		},
		{
			ID:             "gpt-4",
			Name:           "GPT-4",
			DisplayName:    "GPT-4 (Subscription)",
			Description:    "Balanced model for general tasks",
			Status:         "available",
			Roles:          []string{"general", "writer", "assistant"},
			CreatedAt:      &now,
			MaxConcurrency: 10,
		},
		{
			ID:             "gpt-3.5-turbo",
			Name:           "GPT-3.5 Turbo",
			DisplayName:    "GPT-3.5 Turbo (Subscription)",
			Description:    "Fast model for simple tasks",
			Status:         "available",
			Roles:          []string{"general", "assistant", "chatbot"},
			CreatedAt:      &now,
			MaxConcurrency: 20,
		},
		{
			ID:             "claude-3-opus",
			Name:           "Claude 3 Opus",
			DisplayName:    "Claude 3 Opus (Subscription)",
			Description:    "Advanced reasoning model",
			Status:         "available",
			Roles:          []string{"researcher", "analyst", "developer"},
			CreatedAt:      &now,
			MaxConcurrency: 3,
		},
		{
			ID:             "gemini-pro",
			Name:           "Gemini Pro",
			DisplayName:    "Gemini Pro (Subscription)",
			Description:    "Google's advanced model",
			Status:         "available",
			Roles:          []string{"general", "developer", "analyst"},
			CreatedAt:      &now,
			MaxConcurrency: 8,
		},
	}

	return &MockSubscriptionServer{
		models: models,
	}
}

// NewMockSubscriptionServerWithRealModels returns a mock server whose roles
// line up with the current role-based routing tests.
func NewMockSubscriptionServerWithRealModels() *MockSubscriptionServer {
	now := time.Now()
	models := []ModelDefinition{
		{
			ID:             "qwen-2.5-72b",
			Name:           "Qwen 2.5 72B",
			DisplayName:    "Qwen 2.5 72B",
			Status:         "available",
			Roles:          []string{"architect", "research", "general"},
			CreatedAt:      &now,
			MaxConcurrency: 4,
		},
		{
			ID:             "qwen-2.5-coder-32b",
			Name:           "Qwen 2.5 Coder 32B",
			DisplayName:    "Qwen 2.5 Coder 32B",
			Status:         "available",
			Roles:          []string{"implementation", "code_review"},
			CreatedAt:      &now,
			MaxConcurrency: 6,
		},
		{
			ID:             "deepseek-chat",
			Name:           "DeepSeek Chat",
			DisplayName:    "DeepSeek Chat",
			Status:         "available",
			Roles:          []string{"debugging", "testing"},
			CreatedAt:      &now,
			MaxConcurrency: 6,
		},
		{
			ID:             "gemini-2.0-flash",
			Name:           "Gemini 2.0 Flash",
			DisplayName:    "Gemini 2.0 Flash",
			Status:         "available",
			Roles:          []string{"documentation", "general"},
			CreatedAt:      &now,
			MaxConcurrency: 8,
		},
	}

	return &MockSubscriptionServer{models: models}
}

// Start starts the mock server on the specified port
func (m *MockSubscriptionServer) Start(port int) error {
	mux := http.NewServeMux()

	// Register the models endpoint
	mux.HandleFunc("/api/subscription/v1/models", m.handleModels)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	m.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	log.Printf("[MOCK] Starting mock subscription server on port %d", port)
	log.Printf("[MOCK] Available models: %d", len(m.models))
	for _, model := range m.models {
		log.Printf("[MOCK]   - %s (roles: %v)", model.ID, model.Roles)
	}

	return m.server.ListenAndServe()
}

// Stop stops the mock server
func (m *MockSubscriptionServer) Stop() error {
	if m.server != nil {
		return m.server.Close()
	}
	return nil
}

// HandleModelsRequest exposes the models handler for httptest use.
func (m *MockSubscriptionServer) HandleModelsRequest(w http.ResponseWriter, r *http.Request) {
	m.handleModels(w, r)
}

// handleModels handles the /api/subscription/v1/models endpoint
func (m *MockSubscriptionServer) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("[MOCK] Serving models list request")

	response := ModelListResponse{
		Models:    m.models,
		UpdatedAt: &time.Time{},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[MOCK] Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[MOCK] Successfully served %d models", len(m.models))
}

// GetURL returns the base URL of the mock server
func (m *MockSubscriptionServer) GetURL(port int) string {
	return fmt.Sprintf("http://localhost:%d", port)
}
