package providers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/google/uuid"
	"github.com/vultr/govultr/v3"
	"golang.org/x/oauth2"
)

var _ domain.Provider = (*VultrProvider)(nil)

// VultrProvider implements domain.Provider using the Vultr API.
type VultrProvider struct {
	client *govultr.Client
}

// NewVultrProvider creates a VultrProvider using the given API token.
func NewVultrProvider(token string) *VultrProvider {
	ctx := context.Background()
	config := &oauth2.Config{}
	token = normalizeVultrToken(token)
	tokenSource := config.TokenSource(ctx, &oauth2.Token{AccessToken: token})
	client := govultr.NewClient(oauth2.NewClient(ctx, tokenSource))
	client.SetUserAgent("vpsm")

	return &VultrProvider{client: client}
}

// RegisterVultr registers the Vultr provider factory with the global registry.
func RegisterVultr() {
	Register("vultr", func(store auth.Store) (domain.Provider, error) {
		token, err := store.GetToken("vultr")
		if err != nil {
			return nil, fmt.Errorf("vultr auth: %w", err)
		}

		return NewVultrProvider(token), nil
	})
}

// GetDisplayName returns the provider's human-readable display name.
func (v *VultrProvider) GetDisplayName() string {
	return "Vultr"
}

// CreateServer creates a new Vultr instance.
func (v *VultrProvider) CreateServer(ctx context.Context, opts domain.CreateServerOpts) (*domain.Server, error) {
	sshKeys, err := v.resolveSSHKeyIdentifiers(ctx, opts.SSHKeyIdentifiers)
	if err != nil {
		return nil, err
	}

	req := &govultr.InstanceCreateReq{
		Region:   opts.Location,
		Plan:     opts.ServerType,
		Label:    opts.Name,
		Hostname: opts.Name,
		SSHKeys:  sshKeys,
		UserData: opts.UserData,
		Tags:     labelsToTags(opts.Labels),
	}

	if osID, err := strconv.Atoi(opts.Image); err == nil {
		req.OsID = osID
	} else {
		req.ImageID = opts.Image
	}

	instance, resp, err := v.client.Instance.Create(ctx, req)
	if err != nil {
		return nil, mapVultrError("failed to create server", resp, err)
	}

	server := toDomainVultrServer(instance)
	return &server, nil
}

// DeleteServer removes a Vultr instance by ID.
func (v *VultrProvider) DeleteServer(ctx context.Context, id string) error {
	if err := v.client.Instance.Delete(ctx, id); err != nil {
		return mapVultrError("failed to delete server", nil, err)
	}
	return nil
}

// GetServer retrieves a single Vultr instance by ID.
func (v *VultrProvider) GetServer(ctx context.Context, id string) (*domain.Server, error) {
	instance, resp, err := v.client.Instance.Get(ctx, id)
	if err != nil {
		return nil, mapVultrError("failed to get server", resp, err)
	}
	if instance == nil {
		return nil, fmt.Errorf("server %q: %w", id, domain.ErrNotFound)
	}

	server := toDomainVultrServer(instance)
	return &server, nil
}

// ListServers retrieves all Vultr instances.
func (v *VultrProvider) ListServers(ctx context.Context) ([]domain.Server, error) {
	var servers []domain.Server
	options := &govultr.ListOptions{PerPage: 500}

	for {
		instances, meta, resp, err := v.client.Instance.List(ctx, options)
		if err != nil {
			return nil, mapVultrError("failed to list servers", resp, err)
		}

		for i := range instances {
			servers = append(servers, toDomainVultrServer(&instances[i]))
		}

		if meta == nil || meta.Links == nil || meta.Links.Next == "" {
			break
		}
		options.Cursor = meta.Links.Next
	}

	return servers, nil
}

// StartServer powers on a Vultr instance.
func (v *VultrProvider) StartServer(ctx context.Context, id string) (*domain.ActionStatus, error) {
	instance, resp, err := v.client.Instance.Get(ctx, id)
	if err != nil {
		return nil, mapVultrError("failed to start server", resp, err)
	}

	if instance != nil && normalizeVultrStatus(instance) == "running" {
		return &domain.ActionStatus{
			Status:   domain.ActionStatusSuccess,
			Progress: 100,
			Command:  "start_server",
		}, nil
	}

	if err := v.client.Instance.Start(ctx, id); err != nil {
		return nil, mapVultrError("failed to start server", nil, err)
	}

	return &domain.ActionStatus{
		Status:   domain.ActionStatusRunning,
		Progress: 0,
		Command:  "start_server",
	}, nil
}

// StopServer powers off a Vultr instance.
func (v *VultrProvider) StopServer(ctx context.Context, id string) (*domain.ActionStatus, error) {
	if err := v.client.Instance.Halt(ctx, id); err != nil {
		return nil, mapVultrError("failed to stop server", nil, err)
	}

	return &domain.ActionStatus{
		Status:   domain.ActionStatusRunning,
		Progress: 0,
		Command:  "stop_server",
	}, nil
}

func toDomainVultrServer(instance *govultr.Instance) domain.Server {
	if instance == nil {
		return domain.Server{Provider: "vultr", Metadata: map[string]any{}}
	}

	server := domain.Server{
		ID:          instance.ID,
		Name:        instance.Label,
		Status:      normalizeVultrStatus(instance),
		CreatedAt:   parseVultrTime(instance.DateCreated),
		PublicIPv4:  instance.MainIP,
		PublicIPv6:  instance.V6MainIP,
		PrivateIPv4: instance.InternalIP,
		Region:      instance.Region,
		ServerType:  instance.Plan,
		Image:       vultrImageName(instance),
		Provider:    "vultr",
		Metadata: map[string]any{
			"hostname":          instance.Hostname,
			"os_id":             instance.OsID,
			"ram_mb":            instance.RAM,
			"disk_gb":           instance.Disk,
			"vcpu_count":        instance.VCPUCount,
			"allowed_bandwidth": instance.AllowedBandwidth,
			"server_status":     instance.ServerStatus,
			"features":          instance.Features,
			"tags":              instance.Tags,
		},
	}

	if server.Name == "" {
		server.Name = instance.Hostname
	}

	return server
}

func normalizeVultrStatus(instance *govultr.Instance) string {
	status := strings.ToLower(strings.TrimSpace(instance.Status))
	powerStatus := strings.ToLower(strings.TrimSpace(instance.PowerStatus))

	switch status {
	case "active":
		switch powerStatus {
		case "running":
			return "running"
		case "stopped":
			return "off"
		case "":
			return "running"
		default:
			return powerStatus
		}
	case "pending":
		return "initializing"
	case "":
		switch powerStatus {
		case "running":
			return "running"
		case "stopped":
			return "off"
		default:
			return powerStatus
		}
	default:
		return status
	}
}

func vultrImageName(instance *govultr.Instance) string {
	switch {
	case instance.Os != "":
		return instance.Os
	case instance.ImageID != "":
		return instance.ImageID
	case instance.SnapshotID != "":
		return instance.SnapshotID
	default:
		return ""
	}
}

func parseVultrTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed
	}

	parsed, _ = time.Parse("2006-01-02 15:04:05", value)
	return parsed
}

func labelsToTags(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}

	tags := make([]string, 0, len(labels))
	for key, value := range labels {
		if value == "" {
			tags = append(tags, key)
			continue
		}
		tags = append(tags, key+":"+value)
	}
	return tags
}

func mapVultrError(action string, resp *http.Response, err error) error {
	if err == nil {
		return nil
	}

	if resp != nil {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%s: %w (check 'vpsm auth login vultr'; Vultr API keys can also be restricted by source IP)", action, domain.ErrUnauthorized)
		case http.StatusNotFound:
			return fmt.Errorf("%s: %w", action, domain.ErrNotFound)
		case http.StatusConflict:
			return fmt.Errorf("%s: %w", action, domain.ErrConflict)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: %w", action, domain.ErrRateLimited)
		}
	}

	lowerErr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lowerErr, "401"), strings.Contains(lowerErr, "403"):
		return fmt.Errorf("%s: %w (check 'vpsm auth login vultr'; Vultr API keys can also be restricted by source IP)", action, domain.ErrUnauthorized)
	case strings.Contains(lowerErr, "404"):
		return fmt.Errorf("%s: %w", action, domain.ErrNotFound)
	case strings.Contains(lowerErr, "409"):
		return fmt.Errorf("%s: %w", action, domain.ErrConflict)
	case strings.Contains(lowerErr, "429"):
		return fmt.Errorf("%s: %w", action, domain.ErrRateLimited)
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}

func normalizeVultrToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	return strings.TrimSpace(token)
}

func (v *VultrProvider) resolveSSHKeyIdentifiers(ctx context.Context, identifiers []string) ([]string, error) {
	if len(identifiers) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(identifiers))
	var keysByName map[string]string

	for _, identifier := range identifiers {
		identifier = strings.TrimSpace(identifier)
		if identifier == "" {
			continue
		}
		if _, err := uuid.Parse(identifier); err == nil {
			resolved = append(resolved, identifier)
			continue
		}

		if keysByName == nil {
			keys, err := v.ListSSHKeys(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve SSH key %q: %w", identifier, err)
			}
			keysByName = make(map[string]string, len(keys))
			for _, key := range keys {
				keysByName[key.Name] = key.ID
			}
		}

		id, ok := keysByName[identifier]
		if !ok {
			return nil, fmt.Errorf("failed to resolve SSH key %q: %w", identifier, domain.ErrNotFound)
		}
		resolved = append(resolved, id)
	}

	return resolved, nil
}
