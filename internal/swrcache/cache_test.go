package swrcache

import (
	"context"
	"testing"
	"time"
)

func TestGetOrFetch_FreshCache(t *testing.T) {
	dir := t.TempDir()
	cache := WithTTLs(dir, 5*time.Minute, time.Hour)

	key := "porkbun_domains"
	if err := writeEntry(cache, key, Entry[string]{Data: "cached", FetchedAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("writeEntry error: %v", err)
	}

	called := 0
	fetch := func(ctx context.Context) (string, error) {
		called++
		return "fresh", nil
	}

	got, err := GetOrFetch(cache, context.Background(), key, fetch)
	if err != nil {
		t.Fatalf("GetOrFetch error: %v", err)
	}
	if got != "cached" {
		t.Fatalf("got %q, want %q", got, "cached")
	}
	if called != 0 {
		t.Fatalf("fetch called %d times, want 0", called)
	}
}

func TestGetOrFetch_StaleCacheRevalidates(t *testing.T) {
	dir := t.TempDir()
	cache := WithTTLs(dir, 5*time.Minute, time.Hour)

	key := "porkbun_records_example.com"
	if err := writeEntry(cache, key, Entry[string]{Data: "cached", FetchedAt: time.Now().Add(-10 * time.Minute)}); err != nil {
		t.Fatalf("writeEntry error: %v", err)
	}

	called := make(chan struct{}, 1)
	fetch := func(ctx context.Context) (string, error) {
		called <- struct{}{}
		return "fresh", nil
	}

	got, err := GetOrFetch(cache, context.Background(), key, fetch)
	if err != nil {
		t.Fatalf("GetOrFetch error: %v", err)
	}
	if got != "cached" {
		t.Fatalf("got %q, want %q", got, "cached")
	}

	select {
	case <-called:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected background revalidation")
	}

	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		entry, ok, _ := readEntry[string](cache, key)
		if ok && entry.Data == "fresh" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	entry, ok, _ := readEntry[string](cache, key)
	if !ok || entry.Data != "fresh" {
		t.Fatalf("expected cache to be refreshed, got ok=%v data=%q", ok, entry.Data)
	}
}

func TestGetOrFetch_ExpiredCacheFetchesSync(t *testing.T) {
	dir := t.TempDir()
	cache := WithTTLs(dir, 5*time.Minute, time.Hour)

	key := "porkbun_domains"
	if err := writeEntry(cache, key, Entry[string]{Data: "cached", FetchedAt: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatalf("writeEntry error: %v", err)
	}

	called := 0
	fetch := func(ctx context.Context) (string, error) {
		called++
		return "fresh", nil
	}

	got, err := GetOrFetch(cache, context.Background(), key, fetch)
	if err != nil {
		t.Fatalf("GetOrFetch error: %v", err)
	}
	if got != "fresh" {
		t.Fatalf("got %q, want %q", got, "fresh")
	}
	if called != 1 {
		t.Fatalf("fetch called %d times, want 1", called)
	}
}

func TestGetOrFetch_MissFetchesSync(t *testing.T) {
	dir := t.TempDir()
	cache := WithTTLs(dir, 5*time.Minute, time.Hour)

	called := 0
	fetch := func(ctx context.Context) (string, error) {
		called++
		return "fresh", nil
	}

	got, err := GetOrFetch(cache, context.Background(), "missing", fetch)
	if err != nil {
		t.Fatalf("GetOrFetch error: %v", err)
	}
	if got != "fresh" {
		t.Fatalf("got %q, want %q", got, "fresh")
	}
	if called != 1 {
		t.Fatalf("fetch called %d times, want 1", called)
	}
}

func TestInvalidatePrefix(t *testing.T) {
	dir := t.TempDir()
	cache := WithTTLs(dir, 5*time.Minute, time.Hour)

	if err := writeEntry(cache, "porkbun_domains", Entry[string]{Data: "a", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("writeEntry error: %v", err)
	}
	if err := writeEntry(cache, "porkbun_records_example.com", Entry[string]{Data: "b", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("writeEntry error: %v", err)
	}
	if err := writeEntry(cache, "cloudflare_domains", Entry[string]{Data: "c", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("writeEntry error: %v", err)
	}

	if err := cache.InvalidatePrefix("porkbun_"); err != nil {
		t.Fatalf("InvalidatePrefix error: %v", err)
	}

	if _, ok, _ := readEntry[string](cache, "porkbun_domains"); ok {
		t.Fatal("expected porkbun_domains to be removed")
	}
	if _, ok, _ := readEntry[string](cache, "porkbun_records_example.com"); ok {
		t.Fatal("expected porkbun_records_example.com to be removed")
	}
	if _, ok, _ := readEntry[string](cache, "cloudflare_domains"); !ok {
		t.Fatal("expected cloudflare_domains to remain")
	}
}
