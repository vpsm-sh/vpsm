// Package services provides the DNS service layer.
//
// The Service type wraps a domain.Provider and adds input normalisation,
// validation, and default value application before delegating to the provider.
// CLI commands construct a Service from a resolved provider and call service
// methods rather than calling the provider directly.
package services

import (
	"context"
	"fmt"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/swrcache"
)

// Service is the DNS business logic layer. It sits between CLI commands and
// the provider, applying normalisation and validation to all inputs.
type Service struct {
	provider domain.Provider
	cache    *swrcache.Cache
}

// Option configures a Service.
type Option func(*Service)

// WithCache enables stale-while-revalidate caching for read operations.
func WithCache(cache *swrcache.Cache) Option {
	return func(s *Service) {
		s.cache = cache
	}
}

// New returns a Service backed by the given provider.
func New(provider domain.Provider, opts ...Option) *Service {
	svc := &Service{provider: provider}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// ListDomains returns all domains in the provider account.
func (s *Service) ListDomains(ctx context.Context) ([]domain.Domain, error) {
	if s.cache == nil {
		return s.provider.ListDomains(ctx)
	}

	key := cacheKey(s.provider.GetDisplayName(), "domains")
	return swrcache.GetOrFetch(s.cache, ctx, key, s.provider.ListDomains)
}

// ListRecords returns all DNS records for the given domain.
func (s *Service) ListRecords(ctx context.Context, domainName string) ([]domain.Record, error) {
	domainName = normalizeDomain(domainName)
	if domainName == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	if s.cache == nil {
		return s.provider.ListRecords(ctx, domainName)
	}

	key := cacheKey(s.provider.GetDisplayName(), "records", domainName)
	return swrcache.GetOrFetch(s.cache, ctx, key, func(ctx context.Context) ([]domain.Record, error) {
		return s.provider.ListRecords(ctx, domainName)
	})
}

// GetRecord returns a single DNS record by domain and ID.
func (s *Service) GetRecord(ctx context.Context, domainName string, id string) (*domain.Record, error) {
	domainName = normalizeDomain(domainName)
	if domainName == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	if id == "" {
		return nil, fmt.Errorf("record ID is required")
	}
	return s.provider.GetRecord(ctx, domainName, id)
}

// CreateRecord creates a new DNS record after normalising and validating the opts.
func (s *Service) CreateRecord(ctx context.Context, domainName string, opts domain.CreateRecordOpts) (*domain.Record, error) {
	domainName = normalizeDomain(domainName)
	if domainName == "" {
		return nil, fmt.Errorf("domain name is required")
	}

	if err := validateRecordType(opts.Type); err != nil {
		return nil, err
	}
	if err := validateContent(opts.Type, opts.Content); err != nil {
		return nil, err
	}

	// Apply default TTL if none specified.
	if opts.TTL <= 0 {
		opts.TTL = DefaultTTL
	}

	// Normalise the subdomain portion.
	opts.Name = normalizeSubdomain(opts.Name, domainName)

	record, err := s.provider.CreateRecord(ctx, domainName, opts)
	if err == nil && s.cache != nil {
		_ = s.cache.Invalidate(cacheKey(s.provider.GetDisplayName(), "records", domainName))
	}
	return record, err
}

// UpdateRecord updates an existing DNS record after normalising and validating opts.
func (s *Service) UpdateRecord(ctx context.Context, domainName string, id string, opts domain.UpdateRecordOpts) error {
	domainName = normalizeDomain(domainName)
	if domainName == "" {
		return fmt.Errorf("domain name is required")
	}
	if id == "" {
		return fmt.Errorf("record ID is required")
	}

	if opts.Type != "" {
		if err := validateRecordType(opts.Type); err != nil {
			return err
		}
	}
	if opts.Content != "" {
		if err := validateContent(opts.Type, opts.Content); err != nil {
			return err
		}
	}

	if opts.TTL <= 0 {
		opts.TTL = DefaultTTL
	}

	if opts.Name != "" {
		opts.Name = normalizeSubdomain(opts.Name, domainName)
	}

	err := s.provider.UpdateRecord(ctx, domainName, id, opts)
	if err == nil && s.cache != nil {
		_ = s.cache.Invalidate(cacheKey(s.provider.GetDisplayName(), "records", domainName))
	}
	return err
}

// DeleteRecord deletes a DNS record by domain and ID.
func (s *Service) DeleteRecord(ctx context.Context, domainName string, id string) error {
	domainName = normalizeDomain(domainName)
	if domainName == "" {
		return fmt.Errorf("domain name is required")
	}
	if id == "" {
		return fmt.Errorf("record ID is required")
	}
	err := s.provider.DeleteRecord(ctx, domainName, id)
	if err == nil && s.cache != nil {
		_ = s.cache.Invalidate(cacheKey(s.provider.GetDisplayName(), "records", domainName))
	}
	return err
}
