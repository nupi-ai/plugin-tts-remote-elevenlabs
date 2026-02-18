package config

import "testing"

func fakeEnv(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLoaderFromJSON(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{
			"api_key": "sk-test",
			"voice_id": "voice-1",
			"model": "eleven_turbo_v2_5",
			"cache_dir": "/tmp/cache",
			"cache_max_size_mb": 50
		}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test")
	}
	if cfg.VoiceID != "voice-1" {
		t.Errorf("VoiceID = %q, want %q", cfg.VoiceID, "voice-1")
	}
	if cfg.CacheDir != "/tmp/cache" {
		t.Errorf("CacheDir = %q, want %q", cfg.CacheDir, "/tmp/cache")
	}
	if cfg.CacheMaxSizeMB != 50 {
		t.Errorf("CacheMaxSizeMB = %d, want 50", cfg.CacheMaxSizeMB)
	}
}

func TestLoaderDefaults(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"api_key": "sk-test"}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.VoiceID != DefaultVoiceID {
		t.Errorf("VoiceID = %q, want default %q", cfg.VoiceID, DefaultVoiceID)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want default %q", cfg.Model, DefaultModel)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want default %q", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.ListenAddr != DefaultListenAddr {
		t.Errorf("ListenAddr = %q, want default %q", cfg.ListenAddr, DefaultListenAddr)
	}
	if cfg.CacheMaxSizeMB != DefaultCacheMaxSizeMB {
		t.Errorf("CacheMaxSizeMB = %d, want default %d", cfg.CacheMaxSizeMB, DefaultCacheMaxSizeMB)
	}
	if cfg.Language != DefaultLanguage {
		t.Errorf("Language = %q, want default %q", cfg.Language, DefaultLanguage)
	}
}

func TestLoaderLanguageFromJSON(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"api_key": "sk-test", "language": "pl"}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Language != "pl" {
		t.Errorf("Language = %q, want %q", cfg.Language, "pl")
	}
}

func TestLoaderLanguageAutoFromJSON(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"api_key": "sk-test", "language": "auto"}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Language != "auto" {
		t.Errorf("Language = %q, want %q", cfg.Language, "auto")
	}
}

func TestLoaderLanguageWhitespaceFromJSON(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"api_key": "sk-test", "language": "  client  "}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Language != "client" {
		t.Errorf("Language = %q, want %q (whitespace should be trimmed)", cfg.Language, "client")
	}
}

func TestLoaderLanguageCaseInsensitive(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"api_key": "sk-test", "language": "CLIENT"}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Language != "client" {
		t.Errorf("Language = %q, want %q (should be lowercased)", cfg.Language, "client")
	}
}

func TestLoaderEnvOverrides(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG":      `{"api_key": "sk-test"}`,
		"NUPI_ADAPTER_LISTEN_ADDR": "0.0.0.0:9090",
		"NUPI_LOG_LEVEL":           "debug",
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:9090")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoaderMissingAPIKey(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"voice_id": "v1"}`,
	})

	_, err := (Loader{Lookup: env}).Load()
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestLoaderInvalidJSON(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{invalid}`,
	})

	_, err := (Loader{Lookup: env}).Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoaderCacheConfig(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{
			"api_key": "sk-test",
			"cache_dir": "/data/cache",
			"cache_max_size_mb": 200
		}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CacheDir != "/data/cache" {
		t.Errorf("CacheDir = %q, want %q", cfg.CacheDir, "/data/cache")
	}
	if cfg.CacheMaxSizeMB != 200 {
		t.Errorf("CacheMaxSizeMB = %d, want 200", cfg.CacheMaxSizeMB)
	}
}

func TestLoaderCacheDisabledExplicitly(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{
			"api_key": "sk-test",
			"cache_max_size_mb": 0
		}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CacheMaxSizeMB != 0 {
		t.Errorf("CacheMaxSizeMB = %d, want 0 (disabled)", cfg.CacheMaxSizeMB)
	}
}

func TestLoaderStubSynthesizer(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG": `{"use_stub_synthesizer": true}`,
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.UseStubSynthesizer {
		t.Error("UseStubSynthesizer should be true")
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
}

func TestLoaderStubSynthesizerEnvOverride(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_USE_STUB_SYNTHESIZER": "true",
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.UseStubSynthesizer {
		t.Error("UseStubSynthesizer should be true from env override")
	}
}

func TestLoaderStubSynthesizerEnvOverrideOne(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_USE_STUB_SYNTHESIZER": "1",
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.UseStubSynthesizer {
		t.Error("UseStubSynthesizer should be true from env '1'")
	}
}

func TestLoaderStubSynthesizerEnvFalse(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG":               `{"api_key": "sk-test", "use_stub_synthesizer": true}`,
		"NUPI_ADAPTER_USE_STUB_SYNTHESIZER": "false",
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.UseStubSynthesizer {
		t.Error("UseStubSynthesizer should be false when env override is 'false'")
	}
}

func TestLoaderStubSynthesizerEnvInvalid(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_USE_STUB_SYNTHESIZER": "banana",
	})

	_, err := (Loader{Lookup: env}).Load()
	if err == nil {
		t.Fatal("expected error for invalid bool value")
	}
}

func TestLoaderCacheDirFromDataDir(t *testing.T) {
	env := fakeEnv(map[string]string{
		"NUPI_ADAPTER_CONFIG":   `{"api_key": "sk-test"}`,
		"NUPI_ADAPTER_DATA_DIR": "/var/nupi/data",
	})

	cfg, err := (Loader{Lookup: env}).Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CacheDir != "/var/nupi/data/cache" {
		t.Errorf("CacheDir = %q, want %q", cfg.CacheDir, "/var/nupi/data/cache")
	}
}
