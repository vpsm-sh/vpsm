package domain

import "context"

// SearchResult represents the availability status of a single domain.
type SearchResult struct {
	// Domain is the fully qualified domain name (e.g. "example.com").
	Domain string `json:"domain"`

	// Available is true if the domain can be registered.
	Available bool `json:"available"`

	// Premium is true if the domain is premium-priced.
	Premium bool `json:"premium"`

	// Price is the registration price (e.g. "9.73"). Empty if unknown.
	Price string `json:"price"`

	// Renewal is the renewal price (e.g. "9.73"). Empty if unknown.
	Renewal string `json:"renewal"`

	// Currency is the price currency code (e.g. "USD"). Empty if unknown.
	Currency string `json:"currency"`
}

// SearchProvider is an optional interface for providers that support
// domain availability checking. Not all DNS providers are registrars.
type SearchProvider interface {
	// CheckAvailability checks whether the given domain is available for registration.
	CheckAvailability(ctx context.Context, domain string) (*SearchResult, error)
}
