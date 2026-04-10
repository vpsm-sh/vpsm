package domain

// RecordType represents a DNS record type.
type RecordType string

const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
	RecordTypeAlias RecordType = "ALIAS"
	RecordTypeTXT   RecordType = "TXT"
	RecordTypeNS    RecordType = "NS"
	RecordTypeMX    RecordType = "MX"
	RecordTypeSRV   RecordType = "SRV"
	RecordTypeTLSA  RecordType = "TLSA"
	RecordTypeCAA   RecordType = "CAA"
	RecordTypeHTTPS RecordType = "HTTPS"
	RecordTypeSVCB  RecordType = "SVCB"
	RecordTypeSSHFP RecordType = "SSHFP"
)

// Record represents a single DNS record.
type Record struct {
	// ID is the provider-assigned record identifier.
	ID string `json:"id"`

	// Domain is the root domain this record belongs to (e.g. "example.com").
	Domain string `json:"domain"`

	// Name is the fully-qualified record name as returned by the provider
	// (e.g. "www.example.com" or "example.com" for a root record).
	Name string `json:"name"`

	// Type is the DNS record type (A, AAAA, CNAME, etc.).
	Type RecordType `json:"type"`

	// Content is the record value (IP address, hostname, text, etc.).
	Content string `json:"content"`

	// TTL is the time-to-live in seconds. The minimum is provider-dependent.
	TTL int `json:"ttl"`

	// Priority is used for record types that support it (MX, SRV, etc.).
	// Zero means not applicable.
	Priority int `json:"priority"`

	// Notes is an optional human-readable annotation on the record.
	Notes string `json:"notes"`
}

// Domain represents a domain name in the provider account.
type Domain struct {
	// Name is the registered domain name (e.g. "example.com").
	Name string `json:"name"`

	// Status is the current domain status (e.g. "ACTIVE").
	Status string `json:"status"`

	// TLD is the top-level domain suffix (e.g. "com").
	TLD string `json:"tld"`

	// CreateDate is when the domain was registered.
	CreateDate string `json:"create_date"`

	// ExpireDate is when the domain registration expires.
	ExpireDate string `json:"expire_date"`
}
