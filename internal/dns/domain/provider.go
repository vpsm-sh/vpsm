package domain

import "context"

// Provider is the interface that DNS providers must implement.
// It covers domain listing and full DNS record CRUD.
type Provider interface {
	// GetDisplayName returns the human-readable provider name (e.g. "Porkbun").
	GetDisplayName() string

	// ListDomains returns all domains registered in the provider account.
	ListDomains(ctx context.Context) ([]Domain, error)

	// ListRecords returns all DNS records for the given domain.
	ListRecords(ctx context.Context, domain string) ([]Record, error)

	// GetRecord returns a single DNS record by its ID.
	GetRecord(ctx context.Context, domain string, id string) (*Record, error)

	// CreateRecord creates a new DNS record and returns the created record.
	CreateRecord(ctx context.Context, domain string, opts CreateRecordOpts) (*Record, error)

	// UpdateRecord updates an existing DNS record by its ID.
	UpdateRecord(ctx context.Context, domain string, id string, opts UpdateRecordOpts) error

	// DeleteRecord deletes a DNS record by its ID.
	DeleteRecord(ctx context.Context, domain string, id string) error
}
