package llm

import (
	"fmt"
)

// ProviderFactory is a function that creates a Provider from a Config.
// Each provider package registers its factory via RegisterProvider.
type ProviderFactory func(cfg *Config) (Provider, error)

var factories = map[string]ProviderFactory{}

// RegisterProvider registers a provider factory for the given type name.
// Called by provider packages in their init() functions.
func RegisterProvider(typeName string, factory ProviderFactory) {
	factories[typeName] = factory
}

// NewProvider creates a Provider based on the given Config.
func NewProvider(cfg *Config) (Provider, error) {
	factory, ok := factories[cfg.ProviderType]
	if !ok {
		available := make([]string, 0, len(factories))
		for k := range factories {
			available = append(available, k)
		}
		return nil, fmt.Errorf("unknown provider type %q (available: %v)", cfg.ProviderType, available)
	}
	return factory(cfg)
}

// NewProviderFromEnv is a convenience function that loads config from
// environment variables and creates the corresponding Provider.
func NewProviderFromEnv() (Provider, error) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return NewProvider(cfg)
}
