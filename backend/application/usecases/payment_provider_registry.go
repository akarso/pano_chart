package usecases

import (
	"fmt"
	"sync"

	"pano_chart/backend/application/ports"
)

// PaymentProviderRegistry holds the set of available payment providers,
// keyed by provider name.  It is safe for concurrent access.
type PaymentProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]ports.PaymentProviderPort
}

// NewPaymentProviderRegistry creates an empty registry.
func NewPaymentProviderRegistry() *PaymentProviderRegistry {
	return &PaymentProviderRegistry{
		providers: make(map[string]ports.PaymentProviderPort),
	}
}

// Register adds a payment provider to the registry.
// Panics if a provider with the same name is already registered.
func (r *PaymentProviderRegistry) Register(provider ports.PaymentProviderPort) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := provider.ProviderName()
	if _, exists := r.providers[name]; exists {
		panic(fmt.Sprintf("payment provider %q already registered", name))
	}
	r.providers[name] = provider
}

// Get returns the provider with the given name, or an error if not found.
func (r *PaymentProviderRegistry) Get(name string) (ports.PaymentProviderPort, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("payment provider %q not registered", name)
	}
	return p, nil
}

// Names returns the list of registered provider names.
func (r *PaymentProviderRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
