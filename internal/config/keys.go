package config

import (
	"fmt"
	"strings"
)

// KeySpec describes a single configuration key.
type KeySpec struct {
	// Name is the CLI-facing key name (e.g. "default-provider").
	Name string

	// Description is a short human-readable explanation shown in help text.
	Description string

	// Get returns the current value for this key from a loaded Config.
	Get func(cfg *Config) string

	// Set applies a value for this key to the given Config (in memory only;
	// the caller is responsible for calling Save).
	Set func(cfg *Config, value string)
}

// Keys is the authoritative list of all supported configuration keys.
// To add a new option: add a field to Config and append a KeySpec here.
var Keys = []KeySpec{
	{
		Name:        "default-provider",
		Description: "Cloud provider used when --provider is not specified",
		Get:         func(cfg *Config) string { return cfg.DefaultProvider },
		Set:         func(cfg *Config, v string) { cfg.DefaultProvider = v },
	},
	{
		Name:        "dns-provider",
		Description: "DNS provider used when --provider is not specified for DNS commands",
		Get:         func(cfg *Config) string { return cfg.DNSProvider },
		Set:         func(cfg *Config, v string) { cfg.DNSProvider = v },
	},
}

// Lookup returns the KeySpec for the given name, or nil if not found.
// The name is matched case-insensitively after trimming whitespace.
func Lookup(name string) *KeySpec {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for i := range Keys {
		if Keys[i].Name == normalized {
			return &Keys[i]
		}
	}
	return nil
}

// KeyNames returns the names of all registered keys.
func KeyNames() []string {
	names := make([]string, len(Keys))
	for i, k := range Keys {
		names[i] = k.Name
	}
	return names
}

// KeysHelp builds a formatted block listing all available keys and their
// descriptions, suitable for inclusion in Cobra Long help text.
func KeysHelp() string {
	if len(Keys) == 0 {
		return ""
	}

	// Find the longest key name for alignment.
	maxLen := 0
	for _, k := range Keys {
		if len(k.Name) > maxLen {
			maxLen = len(k.Name)
		}
	}

	var b strings.Builder
	b.WriteString("Available keys:\n")
	for _, k := range Keys {
		fmt.Fprintf(&b, "  %-*s   %s\n", maxLen, k.Name, k.Description)
	}
	return b.String()
}
