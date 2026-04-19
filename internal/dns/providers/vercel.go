package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

const (
	vercelBaseURL    = "https://api.vercel.com"
	vercelTimeout    = 30 * time.Second
	vercelTokenStore = "vercel"
	vercelTeamStore  = "vercel-team-id"
)

// Compile-time checks.
var _ domain.Provider = (*VercelProvider)(nil)
var _ domain.SearchProvider = (*VercelProvider)(nil)

// VercelProvider implements domain.Provider using the Vercel REST API.
// It authenticates via a Bearer token. An optional team ID scopes all
// requests to a specific Vercel team.
type VercelProvider struct {
	token   string
	teamID  string // optional
	baseURL string
	client  *http.Client
}

// NewVercelProvider creates a VercelProvider with the given token and optional team ID.
func NewVercelProvider(token, teamID string) *VercelProvider {
	return &VercelProvider{
		token:   token,
		teamID:  teamID,
		baseURL: vercelBaseURL,
		client:  &http.Client{Timeout: vercelTimeout},
	}
}

// RegisterVercel registers the Vercel provider factory with the DNS registry.
func RegisterVercel() {
	Register("vercel", func(store auth.Store) (domain.Provider, error) {
		token, err := store.GetToken(vercelTokenStore)
		if err != nil {
			return nil, fmt.Errorf("vercel auth: token not found (run 'vpsm auth login vercel'): %w", err)
		}
		teamID, _ := store.GetToken(vercelTeamStore) // optional, ignore error
		return NewVercelProvider(token, teamID), nil
	})
}

// GetDisplayName returns the human-readable provider name.
func (v *VercelProvider) GetDisplayName() string {
	return "Vercel"
}

// --- API request/response types ---

// vercelDNSRecord is the Vercel DNS record object.
type vercelDNSRecord struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	MXPriority *int   `json:"mxPriority,omitempty"`
	TTL        int    `json:"ttl"`
	Comment    string `json:"comment"`
	Creator    string `json:"creator"`
	Domain     string `json:"domain,omitempty"`
	RecordType string `json:"recordType,omitempty"`
	CreatedAt  int64  `json:"createdAt,omitempty"`
	UpdatedAt  int64  `json:"updatedAt,omitempty"`
}

// vercelListRecordsResponse is the response for listing DNS records.
type vercelListRecordsResponse struct {
	Records    []vercelDNSRecord  `json:"records"`
	Pagination *vercelPagination  `json:"pagination,omitempty"`
}

// vercelPagination holds pagination info from Vercel list responses.
type vercelPagination struct {
	Count int   `json:"count"`
	Next  int64 `json:"next"`
	Prev  int64 `json:"prev"`
}

// vercelDomain is the Vercel domain object.
type vercelDomain struct {
	Name        string `json:"name"`
	ExpiresAt   *int64 `json:"expiresAt,omitempty"`
	Verified    bool   `json:"verified"`
	ServiceType string `json:"serviceType"`
	CreatedAt   int64  `json:"createdAt"`
	BoughtAt    *int64 `json:"boughtAt,omitempty"`
	Renew       bool   `json:"renew"`
}

// vercelListDomainsResponse is the response for listing domains.
type vercelListDomainsResponse struct {
	Domains    []vercelDomain    `json:"domains"`
	Pagination *vercelPagination `json:"pagination,omitempty"`
}

// vercelCreateRecordResponse is the response for creating a DNS record.
type vercelCreateRecordResponse struct {
	UID     string `json:"uid"`
	Updated int64  `json:"updated"`
}

// vercelCreateRecordBody is the request body for creating a DNS record.
type vercelCreateRecordBody struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	TTL        int    `json:"ttl,omitempty"`
	MXPriority *int   `json:"mxPriority,omitempty"`
	Comment    string `json:"comment,omitempty"`
}

// vercelUpdateRecordBody is the request body for updating a DNS record.
type vercelUpdateRecordBody struct {
	Type       string  `json:"type,omitempty"`
	Name       string  `json:"name,omitempty"`
	Value      string  `json:"value,omitempty"`
	TTL        int     `json:"ttl,omitempty"`
	MXPriority *int    `json:"mxPriority,omitempty"`
	Comment    *string `json:"comment,omitempty"`
}

// vercelUpdateRecordResponse is the response for updating a DNS record.
type vercelUpdateRecordResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Creator    string `json:"creator"`
	Domain     string `json:"domain"`
	TTL        int    `json:"ttl"`
	Comment    string `json:"comment"`
	RecordType string `json:"recordType"`
	CreatedAt  int64  `json:"createdAt"`
}

// vercelAvailabilityResponse is the response for domain availability checks.
type vercelAvailabilityResponse struct {
	Available bool `json:"available"`
}

// vercelPriceResponse is the response for domain price queries.
type vercelPriceResponse struct {
	Years         int    `json:"years"`
	PurchasePrice any    `json:"purchasePrice"` // can be number or object
	RenewalPrice  any    `json:"renewalPrice"`  // can be number or object
}

// vercelErrorResponse is the error shape returned by the Vercel API.
type vercelErrorResponse struct {
	Error *vercelError `json:"error,omitempty"`
}

type vercelError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- HTTP helpers ---

// buildURL constructs the full URL with optional teamId query parameter.
func (v *VercelProvider) buildURL(path string) string {
	url := v.baseURL + path
	if v.teamID != "" {
		if strings.Contains(path, "?") {
			url += "&teamId=" + v.teamID
		} else {
			url += "?teamId=" + v.teamID
		}
	}
	return url
}

// doJSON sends an HTTP request and decodes the JSON response.
// It returns the HTTP status code for error mapping.
func (v *VercelProvider) doJSON(ctx context.Context, method, path string, body any, out any) (int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("vercel: failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, v.buildURL(path), bodyReader)
	if err != nil {
		return 0, fmt.Errorf("vercel: failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("vercel: request failed: %w", err)
	}
	defer resp.Body.Close()

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("vercel: failed to decode response: %w", err)
		}
	} else {
		io.Copy(io.Discard, resp.Body)
	}

	return resp.StatusCode, nil
}

// mapHTTPError maps an HTTP status code to a domain sentinel error.
// If the status indicates success (2xx), nil is returned.
func mapHTTPError(status int, errResp *vercelErrorResponse) error {
	if status >= 200 && status < 300 {
		return nil
	}

	msg := "unknown error"
	if errResp != nil && errResp.Error != nil {
		msg = errResp.Error.Message
	}

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", domain.ErrUnauthorized, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", domain.ErrNotFound, msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", domain.ErrRateLimited, msg)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", domain.ErrConflict, msg)
	}

	return fmt.Errorf("vercel: HTTP %d: %s", status, msg)
}

// doJSONChecked sends a request, decodes the response, and checks for errors.
// On non-2xx responses it attempts to decode a Vercel error object.
func (v *VercelProvider) doJSONChecked(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("vercel: failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, v.buildURL(path), bodyReader)
	if err != nil {
		return fmt.Errorf("vercel: failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("vercel: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("vercel: failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp vercelErrorResponse
		json.Unmarshal(respBody, &errResp) // best-effort
		return mapHTTPError(resp.StatusCode, &errResp)
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("vercel: failed to decode response: %w", err)
		}
	}

	return nil
}

// --- Provider implementation ---

// ListDomains returns all domains in the Vercel account.
func (v *VercelProvider) ListDomains(ctx context.Context) ([]domain.Domain, error) {
	var allDomains []vercelDomain
	var until int64

	for {
		path := "/v5/domains?limit=50"
		if until > 0 {
			path += "&until=" + strconv.FormatInt(until, 10)
		}

		var out vercelListDomainsResponse
		if err := v.doJSONChecked(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, fmt.Errorf("failed to list domains: %w", err)
		}

		allDomains = append(allDomains, out.Domains...)

		if out.Pagination == nil || out.Pagination.Next == 0 || len(out.Domains) == 0 {
			break
		}
		until = out.Pagination.Next
	}

	domains := make([]domain.Domain, 0, len(allDomains))
	for _, d := range allDomains {
		domains = append(domains, vercelToDomain(d))
	}
	return domains, nil
}

// ListRecords returns all DNS records for the given domain.
func (v *VercelProvider) ListRecords(ctx context.Context, domainName string) ([]domain.Record, error) {
	var allRecords []vercelDNSRecord
	var until int64

	for {
		path := fmt.Sprintf("/v5/domains/%s/records?limit=100", domainName)
		if until > 0 {
			path += "&until=" + strconv.FormatInt(until, 10)
		}

		var out vercelListRecordsResponse
		if err := v.doJSONChecked(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, fmt.Errorf("failed to list records for %q: %w", domainName, err)
		}

		allRecords = append(allRecords, out.Records...)

		if out.Pagination == nil || out.Pagination.Next == 0 || len(out.Records) == 0 {
			break
		}
		until = out.Pagination.Next
	}

	records := make([]domain.Record, 0, len(allRecords))
	for _, r := range allRecords {
		records = append(records, vercelToDomainRecord(domainName, r))
	}
	return records, nil
}

// GetRecord returns a single DNS record by its ID.
// Vercel has no single-record endpoint so we list all records and filter.
func (v *VercelProvider) GetRecord(ctx context.Context, domainName string, id string) (*domain.Record, error) {
	records, err := v.ListRecords(ctx, domainName)
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("record %q for %q: %w", id, domainName, domain.ErrNotFound)
}

// CreateRecord creates a new DNS record and returns the created record.
func (v *VercelProvider) CreateRecord(ctx context.Context, domainName string, opts domain.CreateRecordOpts) (*domain.Record, error) {
	body := vercelCreateRecordBody{
		Type:    string(opts.Type),
		Name:    opts.Name,
		Value:   opts.Content,
		TTL:     opts.TTL,
		Comment: opts.Notes,
	}
	if opts.Priority > 0 {
		p := opts.Priority
		body.MXPriority = &p
	}

	path := fmt.Sprintf("/v2/domains/%s/records", domainName)
	var out vercelCreateRecordResponse
	if err := v.doJSONChecked(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, fmt.Errorf("failed to create record for %q: %w", domainName, err)
	}

	// Vercel only returns the UID; fetch the full record.
	return v.GetRecord(ctx, domainName, out.UID)
}

// UpdateRecord updates an existing DNS record by its ID.
func (v *VercelProvider) UpdateRecord(ctx context.Context, domainName string, id string, opts domain.UpdateRecordOpts) error {
	body := vercelUpdateRecordBody{
		Type:  string(opts.Type),
		Name:  opts.Name,
		Value: opts.Content,
		TTL:   opts.TTL,
	}
	if opts.Priority > 0 {
		p := opts.Priority
		body.MXPriority = &p
	}
	if opts.Notes != nil {
		body.Comment = opts.Notes
	}

	path := fmt.Sprintf("/v1/domains/records/%s", id)
	if err := v.doJSONChecked(ctx, http.MethodPatch, path, body, nil); err != nil {
		return fmt.Errorf("failed to update record %q for %q: %w", id, domainName, err)
	}

	return nil
}

// DeleteRecord deletes a DNS record by its ID.
func (v *VercelProvider) DeleteRecord(ctx context.Context, domainName string, id string) error {
	path := fmt.Sprintf("/v2/domains/%s/records/%s", domainName, id)
	if err := v.doJSONChecked(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("failed to delete record %q for %q: %w", id, domainName, err)
	}

	return nil
}

// CheckAvailability checks whether a domain is available for registration via Vercel.
func (v *VercelProvider) CheckAvailability(ctx context.Context, domainName string) (*domain.SearchResult, error) {
	// Check availability.
	var avail vercelAvailabilityResponse
	if err := v.doJSONChecked(ctx, http.MethodGet, "/v1/registrar/domains/"+domainName+"/availability", nil, &avail); err != nil {
		return nil, fmt.Errorf("failed to check availability for %q: %w", domainName, err)
	}

	result := &domain.SearchResult{
		Domain:    domainName,
		Available: avail.Available,
	}

	// If available, also fetch pricing.
	if avail.Available {
		var price vercelPriceResponse
		if err := v.doJSONChecked(ctx, http.MethodGet, "/v1/registrar/domains/"+domainName+"/price", nil, &price); err == nil {
			result.Price = formatPrice(price.PurchasePrice)
			result.Renewal = formatPrice(price.RenewalPrice)
		}
	}

	return result, nil
}

// --- Conversion helpers ---

// vercelToDomain converts a Vercel API domain to a domain.Domain.
func vercelToDomain(d vercelDomain) domain.Domain {
	status := "inactive"
	if d.Verified {
		status = "active"
	}

	expireDate := "N/A"
	if d.ExpiresAt != nil && *d.ExpiresAt > 0 {
		expireDate = time.UnixMilli(*d.ExpiresAt).UTC().Format("2006-01-02")
	}

	createDate := ""
	if d.CreatedAt > 0 {
		createDate = time.UnixMilli(d.CreatedAt).UTC().Format("2006-01-02")
	}

	return domain.Domain{
		Name:       d.Name,
		Status:     status,
		TLD:        extractTLD(d.Name),
		CreateDate: createDate,
		ExpireDate: expireDate,
	}
}

// vercelToDomainRecord converts a Vercel API DNS record to a domain.Record.
func vercelToDomainRecord(domainName string, r vercelDNSRecord) domain.Record {
	prio := 0
	if r.MXPriority != nil {
		prio = *r.MXPriority
	}

	// Vercel stores just the subdomain part in Name; build the FQDN.
	name := r.Name
	if name == "" {
		name = domainName
	} else {
		name = r.Name + "." + domainName
	}

	return domain.Record{
		ID:       r.ID,
		Domain:   domainName,
		Name:     name,
		Type:     domain.RecordType(r.Type),
		Content:  r.Value,
		TTL:      r.TTL,
		Priority: prio,
		Notes:    r.Comment,
	}
}

// formatPrice extracts a string price from the Vercel price response field,
// which can be either a number or an object with an "amount" field.
func formatPrice(v any) string {
	switch p := v.(type) {
	case float64:
		return strconv.FormatFloat(p, 'f', 2, 64)
	case map[string]any:
		if amount, ok := p["amount"]; ok {
			if f, ok := amount.(float64); ok {
				return strconv.FormatFloat(f, 'f', 2, 64)
			}
		}
	}
	return ""
}
