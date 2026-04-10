package services

import (
	"fmt"
	"net"
	"strings"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
)

// DefaultTTL is the TTL applied when none is specified (matches Porkbun's minimum).
const DefaultTTL = 600

// validRecordTypes is the set of supported DNS record types.
var validRecordTypes = map[domain.RecordType]bool{
	domain.RecordTypeA:     true,
	domain.RecordTypeAAAA:  true,
	domain.RecordTypeCNAME: true,
	domain.RecordTypeAlias: true,
	domain.RecordTypeTXT:   true,
	domain.RecordTypeNS:    true,
	domain.RecordTypeMX:    true,
	domain.RecordTypeSRV:   true,
	domain.RecordTypeTLSA:  true,
	domain.RecordTypeCAA:   true,
	domain.RecordTypeHTTPS: true,
	domain.RecordTypeSVCB:  true,
	domain.RecordTypeSSHFP: true,
}

// normalizeDomain lowercases and strips any trailing dot from a domain name.
func normalizeDomain(d string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(d), "."))
}

// normalizeSubdomain strips the root domain suffix from a subdomain if the
// user accidentally passes a fully-qualified name (e.g. "www.example.com"
// when the domain is "example.com"), and lowercases the result.
func normalizeSubdomain(sub, domainName string) string {
	sub = strings.TrimSpace(sub)
	sub = strings.TrimRight(sub, ".")
	sub = strings.ToLower(sub)

	// Strip ".domainName" suffix if present.
	suffix := "." + domainName
	if strings.HasSuffix(sub, suffix) {
		sub = sub[:len(sub)-len(suffix)]
	}
	// Also strip if the caller passed the bare domain as the subdomain.
	if sub == domainName {
		sub = ""
	}

	return sub
}

// validateRecordType returns an error if t is not a supported record type.
func validateRecordType(t domain.RecordType) error {
	if !validRecordTypes[t] {
		return fmt.Errorf("unsupported record type %q", t)
	}
	return nil
}

// validateContent checks that the content value is appropriate for the record type.
// It does not perform exhaustive validation — it catches obvious mismatches
// (e.g. a non-IP value for an A record) to give the user an early error.
func validateContent(t domain.RecordType, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("record content cannot be empty")
	}

	switch t {
	case domain.RecordTypeA:
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("A record content must be a valid IPv4 address, got %q", content)
		}
	case domain.RecordTypeAAAA:
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("AAAA record content must be a valid IPv6 address, got %q", content)
		}
	}

	return nil
}
