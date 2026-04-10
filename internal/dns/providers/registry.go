package providers

import (
	"fmt"
	"sync"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
	"nathanbeddoewebdev/vpsm/internal/util"
)

// Factory is a constructor function that builds a DNS Provider given an auth store.
type Factory func(store auth.Store) (domain.Provider, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a provider factory to the DNS registry.
// It panics on empty name, nil factory, or duplicate registration
// (programmer errors detected at startup).
func Register(name string, factory Factory) {
	normalizedName := util.NormalizeKey(name)
	if normalizedName == "" {
		panic("dns/providers: empty provider name")
	}
	if factory == nil {
		panic("dns/providers: nil factory")
	}

	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[normalizedName]; exists {
		panic(fmt.Sprintf("dns/providers: provider %q already registered", name))
	}

	registry[normalizedName] = factory
}

// Get constructs and returns the DNS Provider for the given name,
// using the store to retrieve credentials.
func Get(name string, store auth.Store) (domain.Provider, error) {
	normalizedName := util.NormalizeKey(name)
	mu.RLock()
	factory, ok := registry[normalizedName]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("dns/providers: unknown provider %q", name)
	}

	return factory(store)
}

// List returns the names of all registered DNS providers.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Reset clears the DNS provider registry. Intended for use in tests only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Factory{}
}
