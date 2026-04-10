package domain

// CreateServerOpts holds the parameters for creating a new server.
// Required fields must be populated for every provider. Optional fields
// may be left at their zero values; providers will apply sensible defaults.
type CreateServerOpts struct {
	// Required
	Name       string
	Image      string // name or ID
	ServerType string // name or ID

	// Common optional
	Location          string
	SSHKeyIdentifiers []string // names or IDs
	Labels            map[string]string
	UserData          string
	StartAfterCreate  *bool // nil = provider default (usually true)

	// Provider-specific extensions (e.g. firewalls, networks, volumes).
	// Keyed by provider-defined strings; see each provider for details.
	Extra map[string]any
}
