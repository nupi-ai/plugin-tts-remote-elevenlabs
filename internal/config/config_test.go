package config

import "testing"

func TestValidateAppliesDefaults(t *testing.T) {
	cfg := Config{
		ListenAddr: "127.0.0.1:50051",
		APIKey:     "test-key",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VoiceID != DefaultVoiceID {
		t.Errorf("VoiceID = %q, want %q", cfg.VoiceID, DefaultVoiceID)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", cfg.Model, DefaultModel)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
}

func TestValidateRequiresAPIKey(t *testing.T) {
	cfg := Config{
		ListenAddr: "127.0.0.1:50051",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestValidateStabilityRange(t *testing.T) {
	base := func() Config {
		return Config{
			ListenAddr: "127.0.0.1:50051",
			APIKey:     "test-key",
		}
	}

	tests := []struct {
		name    string
		val     float64
		wantErr bool
	}{
		{"zero", 0.0, false},
		{"one", 1.0, false},
		{"negative", -0.1, true},
		{"over_one", 1.1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			cfg.Stability = &tt.val
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Stability=%f: err=%v, wantErr=%v", tt.val, err, tt.wantErr)
			}
		})
	}
}

func TestValidateStubSkipsAPIKey(t *testing.T) {
	cfg := Config{
		ListenAddr:         "127.0.0.1:50051",
		UseStubSynthesizer: true,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error when UseStubSynthesizer=true and APIKey empty, got: %v", err)
	}
}

func TestValidateCacheMaxSizeMB(t *testing.T) {
	cfg := Config{
		ListenAddr:     "127.0.0.1:50051",
		APIKey:         "test-key",
		CacheMaxSizeMB: -1,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative CacheMaxSizeMB")
	}

	cfg.CacheMaxSizeMB = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("CacheMaxSizeMB=0 should be valid (disabled): %v", err)
	}

	cfg.CacheMaxSizeMB = 200
	if err := cfg.Validate(); err != nil {
		t.Fatalf("CacheMaxSizeMB=200 should be valid: %v", err)
	}
}
