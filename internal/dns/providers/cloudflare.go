package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

const (
	cloudflareBaseURL    = "https://api.cloudflare.com/client/v4"
	cloudflareTimeout    = 30 * time.Second
	cloudflareTokenStore = "cloudflare"
)

// Compile-time check that CloudflareProvider satisfies domain.Provider.
var _ domain.Provider = (*CloudflareProvider)(nil)

// CloudflareProvider implements domain.Provider using the Cloudflare API v4.
// It authenticates via a scoped Account API Token (not a Global API Key).
// The token needs Zone:Read and DNS:Edit permissions.
// It uses a direct HTTP client rather than the official SDK to keep the
// dependency tree light and the code consistent with other providers.
type CloudflareProvider struct {
	token   string
	baseURL string
	client  *http.Client
}

// NewCloudflareProvider creates a CloudflareProvider with the given Account API Token.
func NewCloudflareProvider(token string) *CloudflareProvider {
	return &CloudflareProvider{
		token:   token,
		baseURL: cloudflareBaseURL,
		client:  &http.Client{Timeout: cloudflareTimeout},
	}
}

// RegisterCloudflare registers the Cloudflare provider factory with the DNS registry.
func RegisterCloudflare() {
	Register("cloudflare", func(store auth.Store) (domain.Provider, error) {
		token, err := store.GetToken(cloudflareTokenStore)
		if err != nil {
			return nil, fmt.Errorf("cloudflare auth: token not found (run 'vpsm auth login cloudflare'): %w", err)
		}
		return NewCloudflareProvider(token), nil
	})
}

// GetDisplayName returns the human-readable provider name.
func (c *CloudflareProvider) GetDisplayName() string {
	return "Cloudflare"
}

// --- API request/response types ---

// cfEnvelope is the standard Cloudflare API response wrapper.
type cfEnvelope[T any] struct {
	Success  bool      `json:"success"`
	Errors   []cfError `json:"errors"`
	Result   T         `json:"result"`
	Messages []cfError `json:"messages,omitempty"`
}

// cfError represents a single Cloudflare API error.
type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cfResultInfo holds pagination info from Cloudflare list responses.
type cfResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
}

// cfListEnvelope extends the envelope with pagination info.
type cfListEnvelope[T any] struct {
	Success    bool         `json:"success"`
	Errors     []cfError    `json:"errors"`
	Result     []T          `json:"result"`
	ResultInfo cfResultInfo `json:"result_info"`
}

// cfZone is the Cloudflare zone object.
type cfZone struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedOn string `json:"created_on"`
}

// cfDNSRecord is the Cloudflare DNS record object.
type cfDNSRecord struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id"`
	ZoneName string `json:"zone_name"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
	Comment  string `json:"comment"`
}

// cfCreateRecordBody is the request body for creating a DNS record.
type cfCreateRecordBody struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl,omitempty"`
	Priority *int   `json:"priority,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// cfUpdateRecordBody is the request body for updating (PATCH) a DNS record.
type cfUpdateRecordBody struct {
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Content  string  `json:"content"`
	TTL      int     `json:"ttl,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	Comment  *string `json:"comment,omitempty"`
}

// --- HTTP helpers ---

// envelopeError extracts a single error from a Cloudflare response envelope.
// It maps known HTTP-level and API-level error codes to domain sentinels.
func envelopeError(success bool, errors []cfError, httpStatus int) error {
	if success {
		return nil
	}

	// Map HTTP status codes to domain sentinels.
	switch httpStatus {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", domain.ErrUnauthorized, cfErrorString(errors))
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", domain.ErrNotFound, cfErrorString(errors))
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", domain.ErrRateLimited, cfErrorString(errors))
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", domain.ErrConflict, cfErrorString(errors))
	}

	// Fall back to inspecting the error codes/messages.
	for _, e := range errors {
		msg := strings.ToLower(e.Message)
		switch {
		case e.Code == 9109 || e.Code == 10000 || strings.Contains(msg, "authentication"):
			return fmt.Errorf("%w: %s", domain.ErrUnauthorized, e.Message)
		case e.Code == 81044 || strings.Contains(msg, "not found"):
			return fmt.Errorf("%w: %s", domain.ErrNotFound, e.Message)
		case e.Code == 81057 || strings.Contains(msg, "already exists"):
			return fmt.Errorf("%w: %s", domain.ErrConflict, e.Message)
		}
	}

	return fmt.Errorf("cloudflare: %s", cfErrorString(errors))
}

// cfErrorString joins multiple Cloudflare errors into a single string.
func cfErrorString(errors []cfError) string {
	if len(errors) == 0 {
		return "unknown error"
	}
	msgs := make([]string, 0, len(errors))
	for _, e := range errors {
		msgs = append(msgs, fmt.Sprintf("[%d] %s", e.Code, e.Message))
	}
	return strings.Join(msgs, "; ")
}

// doJSONWithStatus is like doJSON but also captures the HTTP status code
// for use in error mapping.
func (c *CloudflareProvider) doJSONWithStatus(ctx context.Context, method, path string, body any, out any) (int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("cloudflare: failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return 0, fmt.Errorf("cloudflare: failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("cloudflare: request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("cloudflare: failed to decode response: %w", err)
	}

	return resp.StatusCode, nil
}

// --- Zone lookup ---

// getZoneID resolves a domain name to its Cloudflare zone ID.
func (c *CloudflareProvider) getZoneID(ctx context.Context, domainName string) (string, error) {
	var out cfListEnvelope[cfZone]
	status, err := c.doJSONWithStatus(ctx, http.MethodGet, "/zones?name="+domainName+"&per_page=1", nil, &out)
	if err != nil {
		return "", fmt.Errorf("failed to look up zone for %q: %w", domainName, err)
	}
	if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
		return "", fmt.Errorf("failed to look up zone for %q: %w", domainName, apiErr)
	}

	if len(out.Result) == 0 {
		return "", fmt.Errorf("zone for %q: %w", domainName, domain.ErrNotFound)
	}

	return out.Result[0].ID, nil
}

// --- Provider implementation ---

// ListDomains returns all zones (domains) in the Cloudflare account.
func (c *CloudflareProvider) ListDomains(ctx context.Context) ([]domain.Domain, error) {
	var allZones []cfZone
	page := 1

	for {
		path := fmt.Sprintf("/zones?page=%d&per_page=50", page)
		var out cfListEnvelope[cfZone]
		status, err := c.doJSONWithStatus(ctx, http.MethodGet, path, nil, &out)
		if err != nil {
			return nil, fmt.Errorf("failed to list domains: %w", err)
		}
		if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
			return nil, fmt.Errorf("failed to list domains: %w", apiErr)
		}

		allZones = append(allZones, out.Result...)

		if page >= out.ResultInfo.TotalPages {
			break
		}
		page++
	}

	domains := make([]domain.Domain, 0, len(allZones))
	for _, z := range allZones {
		domains = append(domains, domain.Domain{
			Name:       z.Name,
			Status:     z.Status,
			TLD:        extractTLD(z.Name),
			CreateDate: z.CreatedOn,
			ExpireDate: "N/A",
		})
	}
	return domains, nil
}

// ListRecords returns all DNS records for the given domain.
func (c *CloudflareProvider) ListRecords(ctx context.Context, domainName string) ([]domain.Record, error) {
	zoneID, err := c.getZoneID(ctx, domainName)
	if err != nil {
		return nil, err
	}

	var allRecords []cfDNSRecord
	page := 1

	for {
		path := fmt.Sprintf("/zones/%s/dns_records?page=%d&per_page=100", zoneID, page)
		var out cfListEnvelope[cfDNSRecord]
		status, err := c.doJSONWithStatus(ctx, http.MethodGet, path, nil, &out)
		if err != nil {
			return nil, fmt.Errorf("failed to list records for %q: %w", domainName, err)
		}
		if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
			return nil, fmt.Errorf("failed to list records for %q: %w", domainName, apiErr)
		}

		allRecords = append(allRecords, out.Result...)

		if page >= out.ResultInfo.TotalPages {
			break
		}
		page++
	}

	records := make([]domain.Record, 0, len(allRecords))
	for _, r := range allRecords {
		records = append(records, cfToDomainRecord(domainName, r))
	}
	return records, nil
}

// GetRecord returns a single DNS record by its ID.
func (c *CloudflareProvider) GetRecord(ctx context.Context, domainName string, id string) (*domain.Record, error) {
	zoneID, err := c.getZoneID(ctx, domainName)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, id)
	var out cfEnvelope[cfDNSRecord]
	status, err := c.doJSONWithStatus(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		return nil, fmt.Errorf("failed to get record %q for %q: %w", id, domainName, err)
	}
	if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
		return nil, fmt.Errorf("failed to get record %q for %q: %w", id, domainName, apiErr)
	}

	rec := cfToDomainRecord(domainName, out.Result)
	return &rec, nil
}

// CreateRecord creates a new DNS record and returns the created record.
func (c *CloudflareProvider) CreateRecord(ctx context.Context, domainName string, opts domain.CreateRecordOpts) (*domain.Record, error) {
	zoneID, err := c.getZoneID(ctx, domainName)
	if err != nil {
		return nil, err
	}

	// Build the FQDN name: subdomain.domain or just domain for root.
	name := domainName
	if opts.Name != "" {
		name = opts.Name + "." + domainName
	}

	body := cfCreateRecordBody{
		Type:    string(opts.Type),
		Name:    name,
		Content: opts.Content,
		TTL:     opts.TTL,
		Comment: opts.Notes,
	}
	if opts.Priority > 0 {
		p := opts.Priority
		body.Priority = &p
	}

	path := fmt.Sprintf("/zones/%s/dns_records", zoneID)
	var out cfEnvelope[cfDNSRecord]
	status, err := c.doJSONWithStatus(ctx, http.MethodPost, path, body, &out)
	if err != nil {
		return nil, fmt.Errorf("failed to create record for %q: %w", domainName, err)
	}
	if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
		return nil, fmt.Errorf("failed to create record for %q: %w", domainName, apiErr)
	}

	rec := cfToDomainRecord(domainName, out.Result)
	return &rec, nil
}

// UpdateRecord updates an existing DNS record by its ID.
func (c *CloudflareProvider) UpdateRecord(ctx context.Context, domainName string, id string, opts domain.UpdateRecordOpts) error {
	zoneID, err := c.getZoneID(ctx, domainName)
	if err != nil {
		return err
	}

	// Build the FQDN name.
	name := domainName
	if opts.Name != "" {
		name = opts.Name + "." + domainName
	}

	body := cfUpdateRecordBody{
		Type:    string(opts.Type),
		Name:    name,
		Content: opts.Content,
		TTL:     opts.TTL,
	}
	if opts.Priority > 0 {
		p := opts.Priority
		body.Priority = &p
	}
	if opts.Notes != nil {
		body.Comment = opts.Notes
	}

	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, id)
	var out cfEnvelope[cfDNSRecord]
	status, err := c.doJSONWithStatus(ctx, http.MethodPatch, path, body, &out)
	if err != nil {
		return fmt.Errorf("failed to update record %q for %q: %w", id, domainName, err)
	}
	if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
		return fmt.Errorf("failed to update record %q for %q: %w", id, domainName, apiErr)
	}

	return nil
}

// DeleteRecord deletes a DNS record by its ID.
func (c *CloudflareProvider) DeleteRecord(ctx context.Context, domainName string, id string) error {
	zoneID, err := c.getZoneID(ctx, domainName)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, id)
	var out cfEnvelope[struct {
		ID string `json:"id"`
	}]
	status, err := c.doJSONWithStatus(ctx, http.MethodDelete, path, nil, &out)
	if err != nil {
		return fmt.Errorf("failed to delete record %q for %q: %w", id, domainName, err)
	}
	if apiErr := envelopeError(out.Success, out.Errors, status); apiErr != nil {
		return fmt.Errorf("failed to delete record %q for %q: %w", id, domainName, apiErr)
	}

	return nil
}

// --- Conversion helpers ---

// extractTLD returns the top-level domain suffix from a domain name.
// For "example.com" it returns "com"; for "example.co.uk" it returns "co.uk".
// This is a simple heuristic based on the first dot — it does not handle
// multi-part public suffixes (co.uk, com.au, etc.) precisely.
func extractTLD(name string) string {
	idx := strings.IndexByte(name, '.')
	if idx < 0 || idx >= len(name)-1 {
		return ""
	}
	return name[idx+1:]
}

// cfToDomainRecord converts a Cloudflare API record to a domain.Record.
func cfToDomainRecord(domainName string, r cfDNSRecord) domain.Record {
	prio := 0
	if r.Priority != nil {
		prio = *r.Priority
	}

	return domain.Record{
		ID:       r.ID,
		Domain:   domainName,
		Name:     r.Name,
		Type:     domain.RecordType(r.Type),
		Content:  r.Content,
		TTL:      r.TTL,
		Priority: prio,
		Notes:    r.Comment,
	}
}
