package swrcache

import "time"

// Entry wraps cached data with metadata.
type Entry[T any] struct {
	Data      T         `json:"data"`
	FetchedAt time.Time `json:"fetched_at"`
}
