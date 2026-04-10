package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache provides a simple file-backed cache.
type Cache struct {
	dir string
}

// New returns a cache rooted at dir.
func New(dir string) *Cache {
	return &Cache{dir: dir}
}

// NewDefault returns a cache rooted at the OS user cache dir.
func NewDefault() *Cache {
	return &Cache{dir: defaultDir()}
}

// Get returns true if a valid cache entry was found and decoded into dest.
func (c *Cache) Get(key string, ttl time.Duration, dest any) (bool, error) {
	if c == nil || c.dir == "" || ttl <= 0 {
		return false, nil
	}

	path := c.pathForKey(key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if time.Now().After(info.ModTime().Add(ttl)) {
		_ = os.Remove(path)
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return false, nil
	}

	return true, nil
}

// Set stores data in the cache under key.
func (c *Cache) Set(key string, data any) error {
	if c == nil || c.dir == "" {
		return nil
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(c.dir, sanitizeKey(key)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	return os.Rename(tmpName, c.pathForKey(key))
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

func (c *Cache) pathForKey(key string) string {
	return filepath.Join(c.dir, sanitizeKey(key)+".json")
}

func defaultDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "vpsm")
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
