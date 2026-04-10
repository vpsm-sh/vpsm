package domain

import "nathanbeddoewebdev/vpsm/internal/domain"

// Re-export shared sentinel errors so DNS callers do not need to import
// the cross-domain package directly.
var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = domain.ErrNotFound

	// ErrUnauthorized indicates missing or invalid credentials.
	ErrUnauthorized = domain.ErrUnauthorized

	// ErrRateLimited indicates the provider throttled the request.
	ErrRateLimited = domain.ErrRateLimited

	// ErrConflict indicates a state or uniqueness conflict.
	ErrConflict = domain.ErrConflict
)
