package cache

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache is a disk-backed LRU cache for synthesized PCM audio.
type Cache struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
	log      *slog.Logger
	entries  map[string]*entry
}

type entry struct {
	size       int64
	accessedAt time.Time
	path       string
}

// New creates a Cache that stores files in dir with a total size cap of maxBytes.
// It creates dir if it does not exist and loads any existing .pcm files into the index.
func New(dir string, maxBytes int64, logger *slog.Logger) (*Cache, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cache: create dir: %w", err)
	}
	c := &Cache{
		dir:      dir,
		maxBytes: maxBytes,
		log:      logger.With("component", "cache"),
		entries:  make(map[string]*entry),
	}
	c.loadExisting()
	return c, nil
}

// Get returns cached data for key and true on hit, or nil and false on miss.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	data, err := os.ReadFile(e.path)
	if err != nil {
		// File disappeared — remove stale entry.
		c.log.Warn("cache file unreadable, removing entry", "key", key, "error", err)
		delete(c.entries, key)
		return nil, false
	}

	e.accessedAt = time.Now()
	return data, true
}

// Put stores data under key, evicting least-recently-used entries if necessary.
// Entries larger than maxBytes are silently ignored.
func (c *Cache) Put(key string, data []byte) error {
	newSize := int64(len(data))
	if newSize > c.maxBytes {
		return nil // silently skip oversized entries
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, remove old entry first.
	if old, ok := c.entries[key]; ok {
		os.Remove(old.path)
		delete(c.entries, key)
	}

	c.evict(newSize)

	p := filepath.Join(c.dir, key+".pcm")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("cache: write: %w", err)
	}

	c.entries[key] = &entry{
		size:       newSize,
		accessedAt: time.Now(),
		path:       p,
	}
	return nil
}

// Key produces a deterministic SHA-256 hex key from synthesis parameters.
func Key(text, model, voiceID, languageCode string, stability, similarityBoost *float64, optimizeLatency *int) string {
	h := sha256.New()
	fmt.Fprintf(h, "text=%s\nmodel=%s\nvoice=%s\nlang=%s\n", text, model, voiceID, languageCode)
	if stability != nil {
		fmt.Fprintf(h, "stability=%f\n", *stability)
	}
	if similarityBoost != nil {
		fmt.Fprintf(h, "similarity_boost=%f\n", *similarityBoost)
	}
	if optimizeLatency != nil {
		fmt.Fprintf(h, "optimize_streaming_latency=%d\n", *optimizeLatency)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// totalSize returns the sum of all entry sizes. Must be called with mu held.
func (c *Cache) totalSize() int64 {
	var total int64
	for _, e := range c.entries {
		total += e.size
	}
	return total
}

// evict removes least-recently-used entries until totalSize + needed <= maxBytes.
// Must be called with mu held.
func (c *Cache) evict(needed int64) {
	total := c.totalSize()
	for total+needed > c.maxBytes {
		oldest := c.oldestKey()
		if oldest == "" {
			break
		}
		e := c.entries[oldest]
		os.Remove(e.path)
		delete(c.entries, oldest)
		total -= e.size
		c.log.Debug("evicted cache entry", "key", oldest, "size", e.size)
	}
}

// oldestKey returns the key with the earliest accessedAt. Must be called with mu held.
func (c *Cache) oldestKey() string {
	var oldest string
	var oldestTime time.Time
	first := true
	for k, e := range c.entries {
		if first || e.accessedAt.Before(oldestTime) {
			oldest = k
			oldestTime = e.accessedAt
			first = false
		}
	}
	return oldest
}

// loadExisting scans dir for .pcm files and rebuilds the index from mod times.
func (c *Cache) loadExisting() {
	matches, err := filepath.Glob(filepath.Join(c.dir, "*.pcm"))
	if err != nil {
		c.log.Warn("cache: glob existing files", "error", err)
		return
	}
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		base := filepath.Base(p)
		key := base[:len(base)-len(".pcm")]
		c.entries[key] = &entry{
			size:       info.Size(),
			accessedAt: info.ModTime(),
			path:       p,
		}
	}
	if len(c.entries) > 0 {
		c.log.Info("loaded existing cache entries", "count", len(c.entries), "total_bytes", c.totalSize())
		// Evict entries if loaded data exceeds maxBytes (e.g. limit was reduced).
		c.evict(0)
	}
}
