package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPutAndGet(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	data := []byte("hello pcm audio")
	if err := c.Put("key1", data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("Get returned false, want true")
	}
	if string(got) != string(data) {
		t.Errorf("Get = %q, want %q", got, data)
	}
}

func TestGetMiss(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("Get returned true for nonexistent key")
	}
}

func TestEvictionLRU(t *testing.T) {
	dir := t.TempDir()
	// 100 bytes max
	c, err := New(dir, 100, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Put 60 bytes
	if err := c.Put("a", make([]byte, 60)); err != nil {
		t.Fatalf("Put a: %v", err)
	}
	// Put 60 bytes — should evict "a"
	if err := c.Put("b", make([]byte, 60)); err != nil {
		t.Fatalf("Put b: %v", err)
	}

	if _, ok := c.Get("a"); ok {
		t.Error("key 'a' should have been evicted")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("key 'b' should still exist")
	}
}

func TestEvictionOrder(t *testing.T) {
	dir := t.TempDir()
	// 150 bytes max — fits 2 entries of 50
	c, err := New(dir, 150, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	c.Put("old", make([]byte, 50))
	c.Put("mid", make([]byte, 50))

	// Access "old" to make it more recent than "mid"
	c.Get("old")

	// This should evict "mid" (least recently accessed), not "old"
	c.Put("new", make([]byte, 60))

	if _, ok := c.Get("mid"); ok {
		t.Error("key 'mid' should have been evicted (least recently accessed)")
	}
	if _, ok := c.Get("old"); !ok {
		t.Error("key 'old' should still exist (recently accessed)")
	}
	if _, ok := c.Get("new"); !ok {
		t.Error("key 'new' should exist")
	}
}

func TestPutOversized(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir, 50, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 100 bytes > 50 max — should be silently ignored
	if err := c.Put("big", make([]byte, 100)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, ok := c.Get("big"); ok {
		t.Error("oversized entry should not be cached")
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := Key("text", "model", "voice", nil, nil, nil)
			c.Put(key, make([]byte, 100))
			c.Get(key)
		}()
	}
	wg.Wait()
}

func TestKeyDeterministic(t *testing.T) {
	s := 0.5
	k1 := Key("hello", "m1", "v1", &s, nil, nil)
	k2 := Key("hello", "m1", "v1", &s, nil, nil)
	if k1 != k2 {
		t.Errorf("same input produced different keys: %q vs %q", k1, k2)
	}
}

func TestKeyDifferent(t *testing.T) {
	k1 := Key("hello", "m1", "v1", nil, nil, nil)
	k2 := Key("world", "m1", "v1", nil, nil, nil)
	if k1 == k2 {
		t.Error("different input produced same key")
	}
}

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-create files
	os.WriteFile(filepath.Join(dir, "abc123.pcm"), []byte("audio data"), 0o644)
	os.WriteFile(filepath.Join(dir, "def456.pcm"), []byte("more audio"), 0o644)

	c, err := New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got1, ok1 := c.Get("abc123")
	if !ok1 {
		t.Error("expected abc123 to be loaded")
	}
	if string(got1) != "audio data" {
		t.Errorf("abc123 = %q, want %q", got1, "audio data")
	}

	got2, ok2 := c.Get("def456")
	if !ok2 {
		t.Error("expected def456 to be loaded")
	}
	if string(got2) != "more audio" {
		t.Errorf("def456 = %q, want %q", got2, "more audio")
	}
}

func TestLoadExistingEvictsOverCapacity(t *testing.T) {
	dir := t.TempDir()

	// Pre-create 3 files totaling 150 bytes, but maxBytes will be 100
	os.WriteFile(filepath.Join(dir, "aaa.pcm"), make([]byte, 50), 0o644)
	os.WriteFile(filepath.Join(dir, "bbb.pcm"), make([]byte, 50), 0o644)
	os.WriteFile(filepath.Join(dir, "ccc.pcm"), make([]byte, 50), 0o644)

	c, err := New(dir, 100, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Total should be <= 100 after eviction — at most 2 entries remain
	c.mu.Lock()
	total := c.totalSize()
	count := len(c.entries)
	c.mu.Unlock()

	if total > 100 {
		t.Errorf("totalSize after loadExisting = %d, want <= 100", total)
	}
	if count > 2 {
		t.Errorf("entry count = %d, want <= 2", count)
	}
}

func TestKeyWithOptimizeLatency(t *testing.T) {
	latency0 := 0
	latency4 := 4

	k1 := Key("hello", "m1", "v1", nil, nil, &latency0)
	k2 := Key("hello", "m1", "v1", nil, nil, &latency4)
	k3 := Key("hello", "m1", "v1", nil, nil, nil)

	if k1 == k2 {
		t.Error("different optimize_latency should produce different keys")
	}
	if k1 == k3 {
		t.Error("optimize_latency=0 should differ from optimize_latency=nil")
	}
}

func TestStaleFileCleanup(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	c.Put("stale", []byte("data"))

	// Delete the file behind the cache's back
	os.Remove(filepath.Join(dir, "stale.pcm"))

	_, ok := c.Get("stale")
	if ok {
		t.Error("Get should return false for deleted file")
	}

	// Subsequent Get should also return false (entry cleaned up)
	_, ok = c.Get("stale")
	if ok {
		t.Error("second Get should also return false")
	}
}
