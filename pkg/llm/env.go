package llm

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds the common configuration for creating an LLM provider.
type Config struct {
	ProviderType string // "anthropic", "openai", "gemini"
	APIKey       string
	Model        string
	BaseURL      string // optional override
}

// LoadEnvFile loads environment variables from a .env file.
// Existing environment variables are not overwritten.
func LoadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

// LoadConfigFromEnv reads provider configuration from environment variables.
//
// Environment variables:
//   - PROVIDER_TYPE: "anthropic" (default), "openai", or "gemini"
//   - MODEL_ID: model name (required)
//   - API_KEY: generic API key (fallback)
//   - ANTHROPIC_API_KEY, OPENAI_API_KEY, GEMINI_API_KEY: provider-specific keys (priority)
//   - BASE_URL or ANTHROPIC_BASE_URL/OPENAI_BASE_URL/GEMINI_BASE_URL: endpoint override
func LoadConfigFromEnv() (*Config, error) {
	providerType := strings.ToLower(os.Getenv("PROVIDER_TYPE"))
	if providerType == "" {
		providerType = "anthropic"
	}

	model := os.Getenv("MODEL_ID")
	if model == "" {
		return nil, fmt.Errorf("MODEL_ID environment variable is required")
	}

	apiKey := resolveAPIKey(providerType)
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required: set API_KEY or %s_API_KEY", strings.ToUpper(providerType))
	}

	baseURL := resolveBaseURL(providerType)

	return &Config{
		ProviderType: providerType,
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      baseURL,
	}, nil
}

// resolveAPIKey returns the API key for the given provider type.
// Provider-specific keys take priority over the generic API_KEY.
func resolveAPIKey(providerType string) string {
	var specificKey string
	switch providerType {
	case "anthropic":
		specificKey = os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		specificKey = os.Getenv("OPENAI_API_KEY")
	case "gemini":
		specificKey = os.Getenv("GEMINI_API_KEY")
	}
	if specificKey != "" {
		return specificKey
	}
	return os.Getenv("API_KEY")
}

// resolveBaseURL returns the base URL for the given provider type.
// Provider-specific URLs take priority over the generic BASE_URL.
func resolveBaseURL(providerType string) string {
	var specificURL string
	switch providerType {
	case "anthropic":
		specificURL = os.Getenv("ANTHROPIC_BASE_URL")
	case "openai":
		specificURL = os.Getenv("OPENAI_BASE_URL")
	case "gemini":
		specificURL = os.Getenv("GEMINI_BASE_URL")
	}
	if specificURL != "" {
		return specificURL
	}
	return os.Getenv("BASE_URL")
}
