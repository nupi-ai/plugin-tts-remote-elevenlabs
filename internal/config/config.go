package config

import "fmt"

const (
	// DefaultListenAddr is used when the adapter runner does not inject an explicit address.
	DefaultListenAddr = "127.0.0.1:50051"
	DefaultVoiceID    = "UgBBYS2sOqTuMpoF3BR0" // Mark
	DefaultModel      = "eleven_turbo_v2_5"
	DefaultLogLevel   = "info"
)

// Config captures bootstrap configuration extracted from environment variables
// or injected JSON payload (`NUPI_ADAPTER_CONFIG`).
type Config struct {
	ListenAddr string
	APIKey     string
	VoiceID    string
	Model      string
	LogLevel   string

	// Voice settings (optional)
	Stability                *float64
	SimilarityBoost          *float64
	OptimizeStreamingLatency *int
}

// Validate applies defaults and raises an error when required fields are missing.
func (c *Config) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("config: listen address is required")
	}
	if c.APIKey == "" {
		return fmt.Errorf("config: api_key is required (set in NUPI_ADAPTER_CONFIG)")
	}
	if c.VoiceID == "" {
		c.VoiceID = DefaultVoiceID
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}

	// Validate voice settings ranges if provided
	if c.Stability != nil {
		if *c.Stability < 0.0 || *c.Stability > 1.0 {
			return fmt.Errorf("config: stability must be between 0.0 and 1.0, got %f", *c.Stability)
		}
	}
	if c.SimilarityBoost != nil {
		if *c.SimilarityBoost < 0.0 || *c.SimilarityBoost > 1.0 {
			return fmt.Errorf("config: similarity_boost must be between 0.0 and 1.0, got %f", *c.SimilarityBoost)
		}
	}
	if c.OptimizeStreamingLatency != nil {
		if *c.OptimizeStreamingLatency < 0 || *c.OptimizeStreamingLatency > 4 {
			return fmt.Errorf("config: optimize_streaming_latency must be between 0 and 4, got %d", *c.OptimizeStreamingLatency)
		}
	}

	return nil
}
