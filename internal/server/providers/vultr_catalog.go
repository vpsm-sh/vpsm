package providers

import (
	"context"
	"fmt"
	"strconv"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	"github.com/vultr/govultr/v3"
)

var _ domain.CatalogProvider = (*VultrProvider)(nil)

// ListLocations retrieves all available Vultr regions.
func (v *VultrProvider) ListLocations(ctx context.Context) ([]domain.Location, error) {
	var locations []domain.Location
	options := &govultr.ListOptions{PerPage: 500}

	for {
		regions, meta, resp, err := v.client.Region.List(ctx, options)
		if err != nil {
			return nil, mapVultrError("failed to list locations", resp, err)
		}

		for _, region := range regions {
			locations = append(locations, domain.Location{
				ID:          region.ID,
				Name:        region.ID,
				Description: region.City,
				Country:     region.Country,
				City:        region.City,
			})
		}

		if !vultrHasNextPage(meta) {
			break
		}
		options.Cursor = meta.Links.Next
	}

	return locations, nil
}

// ListServerTypes retrieves all available Vultr plans.
func (v *VultrProvider) ListServerTypes(ctx context.Context) ([]domain.ServerTypeSpec, error) {
	var serverTypes []domain.ServerTypeSpec
	options := &govultr.ListOptions{PerPage: 500}

	for {
		plans, meta, resp, err := v.client.Plan.List(ctx, "", options)
		if err != nil {
			return nil, mapVultrError("failed to list server types", resp, err)
		}

		for _, plan := range plans {
			serverTypes = append(serverTypes, domain.ServerTypeSpec{
				ID:           plan.ID,
				Name:         plan.ID,
				Description:  plan.ID,
				Cores:        plan.VCPUCount,
				Memory:       float64(plan.RAM) / 1024,
				Disk:         plan.Disk,
				PriceMonthly: fmt.Sprintf("%.2f", plan.MonthlyCost),
				Locations:    plan.Locations,
			})
		}

		if !vultrHasNextPage(meta) {
			break
		}
		options.Cursor = meta.Links.Next
	}

	return serverTypes, nil
}

// ListImages retrieves all available Vultr operating system images.
func (v *VultrProvider) ListImages(ctx context.Context) ([]domain.ImageSpec, error) {
	var images []domain.ImageSpec
	options := &govultr.ListOptions{PerPage: 500}

	for {
		oses, meta, resp, err := v.client.OS.List(ctx, options)
		if err != nil {
			return nil, mapVultrError("failed to list images", resp, err)
		}

		for _, os := range oses {
			id := strconv.Itoa(os.ID)
			images = append(images, domain.ImageSpec{
				ID:           id,
				Name:         id,
				Description:  os.Name,
				Type:         "system",
				OSFlavor:     os.Family,
				Architecture: os.Arch,
			})
		}

		if !vultrHasNextPage(meta) {
			break
		}
		options.Cursor = meta.Links.Next
	}

	return images, nil
}

// ListSSHKeys retrieves all SSH keys registered with Vultr.
func (v *VultrProvider) ListSSHKeys(ctx context.Context) ([]domain.SSHKeySpec, error) {
	var keys []domain.SSHKeySpec
	options := &govultr.ListOptions{PerPage: 500}

	for {
		sshKeys, meta, resp, err := v.client.SSHKey.List(ctx, options)
		if err != nil {
			return nil, mapVultrError("failed to list SSH keys", resp, err)
		}

		for _, key := range sshKeys {
			keys = append(keys, domain.SSHKeySpec{
				ID:   key.ID,
				Name: key.Name,
			})
		}

		if !vultrHasNextPage(meta) {
			break
		}
		options.Cursor = meta.Links.Next
	}

	return keys, nil
}

func vultrHasNextPage(meta *govultr.Meta) bool {
	return meta != nil && meta.Links != nil && meta.Links.Next != ""
}
