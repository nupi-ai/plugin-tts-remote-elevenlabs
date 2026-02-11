package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader loads configuration from environment variables. Tests can override
// Lookup to inject deterministic maps.
type Loader struct {
	Lookup func(string) (string, bool)
}

// Load retrieves the adapter configuration from environment variables and validates it.
func (l Loader) Load() (Config, error) {
	if l.Lookup == nil {
		l.Lookup = os.LookupEnv
	}

	cfg := Config{
		ListenAddr:     DefaultListenAddr,
		CacheMaxSizeMB: DefaultCacheMaxSizeMB,
	}

	if raw, ok := l.Lookup("NUPI_ADAPTER_CONFIG"); ok && strings.TrimSpace(raw) != "" {
		if err := applyJSON(raw, &cfg); err != nil {
			return Config{}, err
		}
	}

	overrideString(l.Lookup, "NUPI_ADAPTER_LISTEN_ADDR", &cfg.ListenAddr)
	overrideString(l.Lookup, "NUPI_LOG_LEVEL", &cfg.LogLevel)

	// Default cache directory
	if cfg.CacheDir == "" {
		if dataDir, ok := l.Lookup("NUPI_ADAPTER_DATA_DIR"); ok && dataDir != "" {
			cfg.CacheDir = filepath.Join(dataDir, "cache")
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyJSON(raw string, cfg *Config) error {
	type jsonConfig struct {
		ListenAddr               string   `json:"listen_addr"`
		APIKey                   string   `json:"api_key"`
		VoiceID                  string   `json:"voice_id"`
		Model                    string   `json:"model"`
		LogLevel                 string   `json:"log_level"`
		Stability                *float64 `json:"stability"`
		SimilarityBoost          *float64 `json:"similarity_boost"`
		OptimizeStreamingLatency *int     `json:"optimize_streaming_latency"`
		CacheDir                 string   `json:"cache_dir"`
		CacheMaxSizeMB           *int     `json:"cache_max_size_mb"`
	}
	var payload jsonConfig
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("config: decode NUPI_ADAPTER_CONFIG: %w", err)
	}
	if payload.ListenAddr != "" {
		cfg.ListenAddr = payload.ListenAddr
	}
	if payload.APIKey != "" {
		cfg.APIKey = payload.APIKey
	}
	if payload.VoiceID != "" {
		cfg.VoiceID = payload.VoiceID
	}
	if payload.Model != "" {
		cfg.Model = payload.Model
	}
	if payload.LogLevel != "" {
		cfg.LogLevel = payload.LogLevel
	}
	if payload.Stability != nil {
		assignFloat64Ptr(&cfg.Stability, *payload.Stability)
	}
	if payload.SimilarityBoost != nil {
		assignFloat64Ptr(&cfg.SimilarityBoost, *payload.SimilarityBoost)
	}
	if payload.OptimizeStreamingLatency != nil {
		assignIntPtr(&cfg.OptimizeStreamingLatency, *payload.OptimizeStreamingLatency)
	}
	if payload.CacheDir != "" {
		cfg.CacheDir = payload.CacheDir
	}
	if payload.CacheMaxSizeMB != nil {
		cfg.CacheMaxSizeMB = *payload.CacheMaxSizeMB
	}
	return nil
}

func overrideString(lookup func(string) (string, bool), key string, target *string) {
	if lookup == nil || target == nil {
		return
	}
	if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
		*target = strings.TrimSpace(value)
	}
}

func assignFloat64Ptr(target **float64, value float64) {
	v := value
	*target = &v
}

func assignIntPtr(target **int, value int) {
	v := value
	*target = &v
}
