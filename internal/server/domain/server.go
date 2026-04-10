package domain

import "time"

// Server represents a virtual server instance across providers
type Server struct {
	// Core fields (common across all providers)
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	PublicIPv4  string    `json:"public_ipv4,omitempty"`
	PublicIPv6  string    `json:"public_ipv6,omitempty"`
	PrivateIPv4 string    `json:"private_ipv4,omitempty"`
	Region      string    `json:"region"`
	ServerType  string    `json:"server_type"`
	Image       string    `json:"image,omitempty"`
	Provider    string    `json:"provider"`

	// Metadata holds provider-specific fields
	// Examples: floating_ips, firewalls, volumes, tags, etc.
	Metadata map[string]any `json:"metadata,omitempty"`
}
