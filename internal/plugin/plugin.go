package plugin

import (
	"github.com/0xarkstar/remops/internal/config"
	"github.com/spf13/cobra"
)

// Plugin is the interface that all remops plugins must implement.
type Plugin interface {
	// Name returns the plugin's unique identifier.
	Name() string
	// Version returns the plugin's version string.
	Version() string
	// Description returns a short description of the plugin.
	Description() string
	// Commands returns the cobra commands this plugin provides.
	Commands() []*cobra.Command
	// Init is called once with the loaded config.
	Init(cfg *config.Config) error
}
