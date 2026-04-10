package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

const (
	porkbunBaseURL     = "https://api.porkbun.com/api/json/v3"
	porkbunTimeout     = 30 * time.Second
	porkbunAPIKeyStore = "porkbun-apikey"
	porkbunSecretStore = "porkbun-secretapikey"
)

// Compile-time check that PorkbunProvider satisfies domain.Provider.
var _ domain.Provider = (*PorkbunProvider)(nil)

// PorkbunProvider implements domain.Provider using the Porkbun API v3.
type PorkbunProvider struct {
	apiKey    string
	secretKey string
	baseURL   string
	client    *http.Client
}

// NewPorkbunProvider creates a PorkbunProvider with the given credentials.
func NewPorkbunProvider(apiKey, secretKey string) *PorkbunProvider {
	return &PorkbunProvider{
		apiKey:    apiKey,
		secretKey: secretKey,
		baseURL:   porkbunBaseURL,
		client:    &http.Client{Timeout: porkbunTimeout},
	}
}

// RegisterPorkbun registers the Porkbun provider factory with the DNS registry.
// It reads two separate keychain entries: porkbun-apikey and porkbun-secretapikey.
func RegisterPorkbun() {
	Register("porkbun", func(store auth.Store) (domain.Provider, error) {
		apiKey, err := store.GetToken(porkbunAPIKeyStore)
		if err != nil {
			return nil, fmt.Errorf("porkbun auth: api key not found (run 'vpsm auth login porkbun'): %w", err)
		}
		secretKey, err := store.GetToken(porkbunSecretStore)
		if err != nil {
			return nil, fmt.Errorf("porkbun auth: secret key not found (run 'vpsm auth login porkbun'): %w", err)
		}
		return NewPorkbunProvider(apiKey, secretKey), nil
	})
}

// GetDisplayName returns the human-readable provider name.
func (p *PorkbunProvider) GetDisplayName() string {
	return "Porkbun"
}

// --- API request/response types ---

// porkbunAuth is embedded in every request body.
type porkbunAuth struct {
	APIKey    string `json:"apikey"`
	SecretKey string `json:"secretapikey"`
}

// porkbunResponse is the base response shape for all Porkbun API calls.
type porkbunResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (r porkbunResponse) err() error {
	if r.Status != "SUCCESS" {
		return fmt.Errorf("porkbun: %s", r.Message)
	}
	return nil
}

// porkbunDomainRecord maps to the Porkbun DNS record object.
type porkbunDomainRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     string `json:"ttl"`
	Prio    string `json:"prio"`
	Notes   string `json:"notes"`
}

// --- HTTP helpers ---

// post sends a POST request to the given path with the JSON body,
// and decodes the response into out.
func (p *PorkbunProvider) post(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("porkbun: failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("porkbun: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("porkbun: request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("porkbun: failed to decode response: %w", err)
	}

	return nil
}

// authBody returns the base request body with credentials embedded.
func (p *PorkbunProvider) authBody() porkbunAuth {
	return porkbunAuth{APIKey: p.apiKey, SecretKey: p.secretKey}
}

// mapAPIError converts Porkbun error messages to domain sentinels where recognisable.
func mapAPIError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "authentication"):
		return fmt.Errorf("%w: %s", domain.ErrUnauthorized, err.Error())
	case strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "invalid domain"):
		return fmt.Errorf("%w: %s", domain.ErrNotFound, err.Error())
	case strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests"):
		return fmt.Errorf("%w: %s", domain.ErrRateLimited, err.Error())
	case strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "conflict"):
		return fmt.Errorf("%w: %s", domain.ErrConflict, err.Error())
	}
	return err
}

// --- Provider implementation ---

// ListDomains returns all domains in the Porkbun account.
func (p *PorkbunProvider) ListDomains(ctx context.Context) ([]domain.Domain, error) {
	type request struct {
		porkbunAuth
		Start         string `json:"start,omitempty"`
		IncludeLabels string `json:"includeLabels,omitempty"`
	}

	type apiDomain struct {
		Domain     string `json:"domain"`
		Status     string `json:"status"`
		TLD        string `json:"tld"`
		CreateDate string `json:"createDate"`
		ExpireDate string `json:"expireDate"`
	}

	type response struct {
		porkbunResponse
		Domains []apiDomain `json:"domains"`
	}

	var out response
	if err := p.post(ctx, "/domain/listAll", request{porkbunAuth: p.authBody()}, &out); err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}
	if err := mapAPIError(out.err()); err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	domains := make([]domain.Domain, 0, len(out.Domains))
	for _, d := range out.Domains {
		domains = append(domains, domain.Domain{
			Name:       d.Domain,
			Status:     d.Status,
			TLD:        d.TLD,
			CreateDate: d.CreateDate,
			ExpireDate: d.ExpireDate,
		})
	}
	return domains, nil
}

// ListRecords returns all DNS records for the given domain.
func (p *PorkbunProvider) ListRecords(ctx context.Context, domainName string) ([]domain.Record, error) {
	type response struct {
		porkbunResponse
		Records []porkbunDomainRecord `json:"records"`
	}

	var out response
	if err := p.post(ctx, "/dns/retrieve/"+domainName, p.authBody(), &out); err != nil {
		return nil, fmt.Errorf("failed to list records for %q: %w", domainName, err)
	}
	if err := mapAPIError(out.err()); err != nil {
		return nil, fmt.Errorf("failed to list records for %q: %w", domainName, err)
	}

	records := make([]domain.Record, 0, len(out.Records))
	for _, r := range out.Records {
		records = append(records, toDomainRecord(domainName, r))
	}
	return records, nil
}

// GetRecord returns a single DNS record by its ID.
func (p *PorkbunProvider) GetRecord(ctx context.Context, domainName string, id string) (*domain.Record, error) {
	type response struct {
		porkbunResponse
		Records []porkbunDomainRecord `json:"records"`
	}

	var out response
	if err := p.post(ctx, "/dns/retrieve/"+domainName+"/"+id, p.authBody(), &out); err != nil {
		return nil, fmt.Errorf("failed to get record %q for %q: %w", id, domainName, err)
	}
	if err := mapAPIError(out.err()); err != nil {
		return nil, fmt.Errorf("failed to get record %q for %q: %w", id, domainName, err)
	}

	if len(out.Records) == 0 {
		return nil, fmt.Errorf("record %q for %q: %w", id, domainName, domain.ErrNotFound)
	}

	rec := toDomainRecord(domainName, out.Records[0])
	return &rec, nil
}

// CreateRecord creates a new DNS record and returns the created record with its assigned ID.
func (p *PorkbunProvider) CreateRecord(ctx context.Context, domainName string, opts domain.CreateRecordOpts) (*domain.Record, error) {
	type request struct {
		porkbunAuth
		Name    string `json:"name,omitempty"`
		Type    string `json:"type"`
		Content string `json:"content"`
		TTL     string `json:"ttl,omitempty"`
		Prio    string `json:"prio,omitempty"`
		Notes   string `json:"notes,omitempty"`
	}

	type response struct {
		porkbunResponse
		ID int `json:"id"`
	}

	body := request{
		porkbunAuth: p.authBody(),
		Name:        opts.Name,
		Type:        string(opts.Type),
		Content:     opts.Content,
		Notes:       opts.Notes,
	}
	if opts.TTL > 0 {
		body.TTL = fmt.Sprintf("%d", opts.TTL)
	}
	if opts.Priority > 0 {
		body.Prio = fmt.Sprintf("%d", opts.Priority)
	}

	var out response
	if err := p.post(ctx, "/dns/create/"+domainName, body, &out); err != nil {
		return nil, fmt.Errorf("failed to create record for %q: %w", domainName, err)
	}
	if err := mapAPIError(out.err()); err != nil {
		return nil, fmt.Errorf("failed to create record for %q: %w", domainName, err)
	}

	// Fetch the newly created record so we can return a fully-populated struct.
	return p.GetRecord(ctx, domainName, fmt.Sprintf("%d", out.ID))
}

// UpdateRecord updates an existing DNS record by its ID.
func (p *PorkbunProvider) UpdateRecord(ctx context.Context, domainName string, id string, opts domain.UpdateRecordOpts) error {
	type request struct {
		porkbunAuth
		Name    string  `json:"name,omitempty"`
		Type    string  `json:"type"`
		Content string  `json:"content"`
		TTL     string  `json:"ttl,omitempty"`
		Prio    string  `json:"prio,omitempty"`
		Notes   *string `json:"notes"`
	}

	body := request{
		porkbunAuth: p.authBody(),
		Name:        opts.Name,
		Type:        string(opts.Type),
		Content:     opts.Content,
		Notes:       opts.Notes,
	}
	if opts.TTL > 0 {
		body.TTL = fmt.Sprintf("%d", opts.TTL)
	}
	if opts.Priority > 0 {
		body.Prio = fmt.Sprintf("%d", opts.Priority)
	}

	var out porkbunResponse
	if err := p.post(ctx, "/dns/edit/"+domainName+"/"+id, body, &out); err != nil {
		return fmt.Errorf("failed to update record %q for %q: %w", id, domainName, err)
	}
	if err := mapAPIError(out.err()); err != nil {
		return fmt.Errorf("failed to update record %q for %q: %w", id, domainName, err)
	}

	return nil
}

// DeleteRecord deletes a DNS record by its ID.
func (p *PorkbunProvider) DeleteRecord(ctx context.Context, domainName string, id string) error {
	var out porkbunResponse
	if err := p.post(ctx, "/dns/delete/"+domainName+"/"+id, p.authBody(), &out); err != nil {
		return fmt.Errorf("failed to delete record %q for %q: %w", id, domainName, err)
	}
	if err := mapAPIError(out.err()); err != nil {
		return fmt.Errorf("failed to delete record %q for %q: %w", id, domainName, err)
	}

	return nil
}

// --- Conversion helpers ---

// toDomainRecord converts a Porkbun API record to a domain.Record.
func toDomainRecord(domainName string, r porkbunDomainRecord) domain.Record {
	ttl := parseInt(r.TTL)
	prio := parseInt(r.Prio)

	return domain.Record{
		ID:       r.ID,
		Domain:   domainName,
		Name:     r.Name,
		Type:     domain.RecordType(r.Type),
		Content:  r.Content,
		TTL:      ttl,
		Priority: prio,
		Notes:    r.Notes,
	}
}

// parseInt converts a string to int, returning 0 on failure.
func parseInt(s string) int {
	if s == "" {
		return 0
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}
