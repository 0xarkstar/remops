package plugin

import (
	"fmt"

	"github.com/0xarkstar/remops/internal/config"
)

// Registry holds registered plugins.
type Registry struct {
	plugins map[string]Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin)}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(p Plugin) error {
	if _, exists := r.plugins[p.Name()]; exists {
		return fmt.Errorf("plugin %q already registered", p.Name())
	}
	r.plugins[p.Name()] = p
	return nil
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) (Plugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

// All returns all registered plugins.
func (r *Registry) All() []Plugin {
	result := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// InitAll initializes plugins listed in enabledPlugins with the given config.
// If enabledPlugins is empty, no plugins are initialized.
func (r *Registry) InitAll(cfg *config.Config, enabledPlugins []string) error {
	if len(enabledPlugins) == 0 {
		return nil
	}
	enabled := make(map[string]bool, len(enabledPlugins))
	for _, name := range enabledPlugins {
		enabled[name] = true
	}
	for name, p := range r.plugins {
		if enabled[name] {
			if err := p.Init(cfg); err != nil {
				return fmt.Errorf("plugin %s init: %w", name, err)
			}
		}
	}
	return nil
}
