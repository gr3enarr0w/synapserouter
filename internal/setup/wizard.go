package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const configDir = "~/.synroute"
const configFile = "config.yaml"
const envFile = ".env"

type UserConfig struct {
	Role      string   `yaml:"role,omitempty"`
	Languages []string `yaml:"languages,omitempty"`
	Tools     []string `yaml:"tools,omitempty"`
}

type FullConfig struct {
	User UserConfig `yaml:"user,omitempty"`
}

// Wizard runs the interactive setup wizard
func Wizard() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║           Welcome to SynapseRouter Setup              ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Step 1: Role selection
	role, err := selectRole(reader)
	if err != nil {
		return err
	}

	// Step 2: Language selection
	languages, err := selectLanguages(reader)
	if err != nil {
		return err
	}

	// Step 3: Team tools selection
	tools, err := selectTools(reader)
	if err != nil {
		return err
	}

	// Step 4: Provider detection and API key setup
	apiKeys, err := setupProviders(reader)
	if err != nil {
		return err
	}

	// Step 5: Save configuration
	err = saveConfig(role, languages, tools)
	if err != nil {
		return err
	}

	// Save API keys to .env
	if len(apiKeys) > 0 {
		err = saveEnvKeys(apiKeys)
		if err != nil {
			return err
		}
	}

	// Show summary
	showSummary(role, languages, tools, apiKeys)

	return nil
}

func selectRole(reader *bufio.Reader) (string, error) {
	fmt.Println("Step 1: What's your primary role?")
	fmt.Println("─────────────────────────────────")
	roles := []string{
		"Developer",
		"Data Scientist",
		"DevOps Engineer",
		"Security Engineer",
		"CTO/Manager",
		"Student",
	}
	for i, role := range roles {
		fmt.Printf("  %d. %s\n", i+1, role)
	}
	fmt.Print("\nEnter number: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(roles) {
		fmt.Println("Invalid selection, defaulting to Developer")
		return roles[0], nil
	}

	return roles[idx-1], nil
}

func selectLanguages(reader *bufio.Reader) ([]string, error) {
	fmt.Println("\nStep 2: Which languages do you work with? (multi-select)")
	fmt.Println("────────────────────────────────────────────────────────")
	languages := []string{
		"Go",
		"Python",
		"JavaScript/TypeScript",
		"Rust",
		"Java",
		"Ruby",
		"C/C++",
		"Other",
	}
	for i, lang := range languages {
		fmt.Printf("  %d. %s\n", i+1, lang)
	}
	fmt.Print("\nEnter numbers (comma-separated), or press Enter to skip: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	input = strings.TrimSpace(input)

	if input == "" {
		return []string{}, nil
	}

	parts := strings.Split(input, ",")
	var selected []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		idx, err := strconv.Atoi(p)
		if err != nil || idx < 1 || idx > len(languages) {
			continue
		}
		selected = append(selected, languages[idx-1])
	}

	if len(selected) == 0 {
		fmt.Println("No valid selection, defaulting to empty")
		return []string{}, nil
	}

	return selected, nil
}

func selectTools(reader *bufio.Reader) ([]string, error) {
	fmt.Println("\nStep 3: What team tools do you use? (multi-select)")
	fmt.Println("────────────────────────────────────────────────────")
	tools := []string{
		"GitHub",
		"GitLab",
		"Jira",
		"Slack",
		"Linear",
		"Notion",
		"Docker",
		"Kubernetes",
		"AWS",
		"GCP",
		"Azure",
	}
	for i, tool := range tools {
		fmt.Printf("  %d. %s\n", i+1, tool)
	}
	fmt.Print("\nEnter numbers (comma-separated), or press Enter to skip: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	input = strings.TrimSpace(input)

	if input == "" {
		return []string{}, nil
	}

	parts := strings.Split(input, ",")
	var selected []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		idx, err := strconv.Atoi(p)
		if err != nil || idx < 1 || idx > len(tools) {
			continue
		}
		selected = append(selected, tools[idx-1])
	}

	return selected, nil
}

func setupProviders(reader *bufio.Reader) (map[string]string, error) {
	fmt.Println("\nStep 4: API Provider Configuration")
	fmt.Println("───────────────────────────────────")

	apiKeys := make(map[string]string)

	// Check for existing keys
	existingKeys := checkExistingKeys()
	if len(existingKeys) > 0 {
		fmt.Println("\nDetected existing API keys:")
		for key := range existingKeys {
			fmt.Printf("  ✓ %s\n", key)
		}
		fmt.Print("\nKeep existing keys? (Y/n): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "n" {
			existingKeys = make(map[string]string)
		}
	}

	// Ollama
	fmt.Println("\nOllama Configuration:")
	fmt.Println("  Checking for local Ollama at localhost:11434...")
	if checkOllamaLocal() {
		fmt.Println("  ✓ Local Ollama detected")
		apiKeys["OLLAMA_HOST"] = "http://localhost:11434"
	} else {
		fmt.Print("  Enter Ollama Cloud API key (or press Enter to skip): ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key != "" {
			apiKeys["OLLAMA_API_KEY"] = key
		}
	}

	// Anthropic
	if _, exists := existingKeys["ANTHROPIC_API_KEY"]; !exists {
		fmt.Print("\nEnter Anthropic API key (or press Enter to skip): ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key != "" {
			apiKeys["ANTHROPIC_API_KEY"] = key
		}
	}

	// OpenAI
	if _, exists := existingKeys["OPENAI_API_KEY"]; !exists {
		fmt.Print("\nEnter OpenAI API key (or press Enter to skip): ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key != "" {
			apiKeys["OPENAI_API_KEY"] = key
		}
	}

	// Google
	if _, exists := existingKeys["GOOGLE_API_KEY"]; !exists {
		fmt.Print("\nEnter Google API key (or press Enter to skip): ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key != "" {
			apiKeys["GOOGLE_API_KEY"] = key
		}
	}

	return apiKeys, nil
}

func checkExistingKeys() map[string]string {
	existing := make(map[string]string)
	keys := []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GOOGLE_API_KEY",
		"OLLAMA_API_KEY",
	}
	for _, key := range keys {
		if os.Getenv(key) != "" {
			existing[key] = os.Getenv(key)
		}
	}
	return existing
}

func checkOllamaLocal() bool {
	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://localhost:11434/api/tags")
	err := cmd.Run()
	return err == nil
}

func saveConfig(role string, languages, tools []string) error {
	// Expand ~ to home directory
	configPath := expandTilde(configDir)
	configFile := filepath.Join(configPath, "config.yaml")

	// Create directory if it doesn't exist
	err := os.MkdirAll(configPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	config := FullConfig{
		User: UserConfig{
			Role:      role,
			Languages: languages,
			Tools:     tools,
		},
	}

	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = os.WriteFile(configFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("\n✓ Configuration saved to %s\n", configFile)
	return nil
}

func saveEnvKeys(apiKeys map[string]string) error {
	if len(apiKeys) == 0 {
		return nil
	}

	envPath := expandTilde(envFile)

	// Check if file exists and read existing content
	var existingContent strings.Builder
	if data, err := os.ReadFile(envPath); err == nil {
		existingContent.Write(data)
		if !strings.HasSuffix(existingContent.String(), "\n") {
			existingContent.WriteString("\n")
		}
	}

	// Append new keys
	for key, value := range apiKeys {
		existingContent.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	err := os.WriteFile(envPath, []byte(existingContent.String()), 0600)
	if err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	fmt.Printf("✓ API keys saved to %s\n", envPath)
	return nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}

func showSummary(role string, languages, tools []string, apiKeys map[string]string) {
	fmt.Println("\n╔════════════════════════════════════════════════════════╗")
	fmt.Println("║                  Setup Complete!                       ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Configuration Summary:")
	fmt.Println("──────────────────────")
	fmt.Printf("  Role: %s\n", role)

	if len(languages) > 0 {
		fmt.Printf("  Languages: %s\n", strings.Join(languages, ", "))
	} else {
		fmt.Println("  Languages: (none selected)")
	}

	if len(tools) > 0 {
		fmt.Printf("  Tools: %s\n", strings.Join(tools, ", "))
	} else {
		fmt.Println("  Tools: (none selected)")
	}

	if len(apiKeys) > 0 {
		fmt.Printf("  API Keys configured: %d\n", len(apiKeys))
	}

	// Suggest a first prompt based on role
	suggestedPrompt := getSuggestedPrompt(role, languages)
	fmt.Println()
	fmt.Println("Suggested first command:")
	fmt.Printf("  synroute chat --message \"%s\"\n", suggestedPrompt)
	fmt.Println()
	fmt.Println("Run 'synroute doctor' to verify your setup.")
}

func getSuggestedPrompt(role string, languages []string) string {
	switch role {
	case "Developer":
		if contains(languages, "Go") {
			return "Help me create a new Go microservice with REST API endpoints"
		}
		if contains(languages, "Python") {
			return "Help me create a new Python FastAPI application"
		}
		if contains(languages, "JavaScript/TypeScript") {
			return "Help me create a new React component with TypeScript"
		}
		return "Help me set up a new project with best practices"
	case "Data Scientist":
		return "Help me create a machine learning pipeline for classification"
	case "DevOps Engineer":
		return "Help me create a Dockerfile and Kubernetes deployment"
	case "Security Engineer":
		return "Help me review this code for security vulnerabilities"
	case "CTO/Manager":
		return "Help me create a technical roadmap for our Q4 goals"
	case "Student":
		return "Help me understand this code and explain it step by step"
	default:
		return "Help me get started with SynapseRouter"
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// CheckConfigExists checks if the config file exists
func CheckConfigExists() bool {
	configPath := expandTilde(filepath.Join(configDir, configFile))
	_, err := os.Stat(configPath)
	return err == nil
}
