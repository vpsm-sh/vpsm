package domain

// CreateRecordOpts holds the parameters for creating a new DNS record.
type CreateRecordOpts struct {
	// Name is the subdomain portion of the record, not including the root domain.
	// Leave empty to create a record on the root domain.
	// Use "*" to create a wildcard record.
	Name string

	// Type is the DNS record type. Required.
	Type RecordType

	// Content is the record value. Required.
	Content string

	// TTL is the time-to-live in seconds.
	// Zero means use the provider default (600 for Porkbun).
	TTL int

	// Priority is used for record types that support it (MX, SRV, etc.).
	Priority int

	// Notes is an optional human-readable annotation.
	Notes string
}

// UpdateRecordOpts holds the parameters for updating an existing DNS record.
// All fields are applied when provided.
type UpdateRecordOpts struct {
	// Name is the new subdomain portion. Leave empty to keep unchanged.
	Name string

	// Type is the new record type.
	Type RecordType

	// Content is the new record value. Required.
	Content string

	// TTL is the new time-to-live in seconds.
	// Zero means use the provider default.
	TTL int

	// Priority is the new priority value.
	Priority int

	// Notes controls the record annotation.
	// nil means no change; pointer to empty string clears the notes.
	Notes *string
}
