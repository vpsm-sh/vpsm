package auditlog

import "context"

type Metadata struct {
	Provider     string
	ResourceType string
	ResourceID   string
	ResourceName string
}

type metadataKey struct{}

// WithMetadata attaches audit metadata to a context.
func WithMetadata(ctx context.Context, meta Metadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	existing, _ := ctx.Value(metadataKey{}).(Metadata)
	merged := Metadata{
		Provider:     pick(meta.Provider, existing.Provider),
		ResourceType: pick(meta.ResourceType, existing.ResourceType),
		ResourceID:   pick(meta.ResourceID, existing.ResourceID),
		ResourceName: pick(meta.ResourceName, existing.ResourceName),
	}
	return context.WithValue(ctx, metadataKey{}, merged)
}

// MetadataFromContext returns audit metadata stored in the context.
func MetadataFromContext(ctx context.Context) Metadata {
	if ctx == nil {
		return Metadata{}
	}
	meta, _ := ctx.Value(metadataKey{}).(Metadata)
	return meta
}

func pick(next, fallback string) string {
	if next != "" {
		return next
	}
	return fallback
}
