package swrcache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultFreshTTL = 5 * time.Minute
	defaultMaxStale = time.Hour
	refreshTimeout  = 30 * time.Second
)

// Cache provides stale-while-revalidate caching with file-backed JSON storage.
type Cache struct {
	dir      string
	freshTTL time.Duration
	maxStale time.Duration
}

// New returns a cache rooted at dir with default TTLs.
func New(dir string) *Cache {
	return &Cache{dir: dir, freshTTL: defaultFreshTTL, maxStale: defaultMaxStale}
}

// NewDefault returns a cache rooted at the OS user cache dir.
func NewDefault() *Cache {
	return New(defaultDir())
}

// WithTTLs returns a new cache rooted at dir with custom TTLs.
func WithTTLs(dir string, freshTTL, maxStale time.Duration) *Cache {
	return &Cache{dir: dir, freshTTL: freshTTL, maxStale: maxStale}
}

// GetOrFetch returns cached data using stale-while-revalidate semantics.
func GetOrFetch[T any](c *Cache, ctx context.Context, key string, fetch func(context.Context) (T, error)) (T, error) {
	if c == nil || c.dir == "" {
		return fetch(ctx)
	}

	entry, ok, err := readEntry[T](c, key)
	if err != nil || !ok || entry.FetchedAt.IsZero() {
		return fetchAndStore(c, ctx, key, fetch)
	}

	age := time.Since(entry.FetchedAt)
	if age < 0 {
		return fetchAndStore(c, ctx, key, fetch)
	}

	if age <= c.freshTTL {
		return entry.Data, nil
	}

	if c.maxStale <= 0 || age <= c.maxStale {
		revalidate(c, key, fetch)
		return entry.Data, nil
	}

	return fetchAndStore(c, ctx, key, fetch)
}

// Invalidate removes a single cached entry.
func (c *Cache) Invalidate(key string) error {
	if c == nil || c.dir == "" {
		return nil
	}

	err := os.Remove(c.pathForKey(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// InvalidatePrefix removes cached entries with the given key prefix.
func (c *Cache) InvalidatePrefix(prefix string) error {
	if c == nil || c.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	sanitized := sanitizeKey(prefix)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, sanitized) {
			if err := os.RemoveAll(filepath.Join(c.dir, name)); err != nil {
				return err
			}
		}
	}

	return nil
}

// Clear removes all cached entries in the cache directory.
func (c *Cache) Clear() error {
	if c == nil || c.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(c.dir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

func fetchAndStore[T any](c *Cache, ctx context.Context, key string, fetch func(context.Context) (T, error)) (T, error) {
	data, err := fetch(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	_ = writeEntry(c, key, Entry[T]{Data: data, FetchedAt: time.Now()})
	return data, nil
}

func revalidate[T any](c *Cache, key string, fetch func(context.Context) (T, error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()
		data, err := fetch(ctx)
		if err != nil {
			return
		}
		_ = writeEntry(c, key, Entry[T]{Data: data, FetchedAt: time.Now()})
	}()
}

func readEntry[T any](c *Cache, key string) (Entry[T], bool, error) {
	var zero Entry[T]
	path := c.pathForKey(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, false, nil
		}
		return zero, false, err
	}

	var entry Entry[T]
	if err := json.Unmarshal(data, &entry); err != nil {
		return zero, false, nil
	}

	return entry, true, nil
}

func writeEntry[T any](c *Cache, key string, entry Entry[T]) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(c.dir, sanitizeKey(key)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()

	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}

	return os.Rename(name, c.pathForKey(key))
}

func (c *Cache) pathForKey(key string) string {
	return filepath.Join(c.dir, sanitizeKey(key)+".json")
}

func defaultDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "vpsm", "dns")
}

func sanitizeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return "cache"
	}

	var b strings.Builder
	b.Grow(len(key))
	for i := 0; i < len(key); i++ {
		ch := key[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteByte(ch)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}
