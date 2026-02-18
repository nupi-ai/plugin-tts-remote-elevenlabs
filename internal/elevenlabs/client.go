package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// BaseURL is the ElevenLabs API base URL.
	BaseURL = "https://api.elevenlabs.io/v1"

	// DefaultTimeout for HTTP requests (can be overridden per-request).
	DefaultTimeout = 30 * time.Second
)

// Client wraps HTTP calls to the ElevenLabs API.
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

// NewClient constructs an ElevenLabs API client with the provided API key.
func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		apiKey:  apiKey,
		baseURL: BaseURL,
	}
}

// VoiceSettings contains optional voice configuration parameters.
type VoiceSettings struct {
	Stability       *float64 `json:"stability,omitempty"`
	SimilarityBoost *float64 `json:"similarity_boost,omitempty"`
}

// SynthesizeRequest describes a TTS synthesis request.
type SynthesizeRequest struct {
	Text                     string         `json:"text"`
	ModelID                  string         `json:"model_id,omitempty"`
	LanguageCode             string         `json:"language_code,omitempty"`
	VoiceSettings            *VoiceSettings `json:"voice_settings,omitempty"`
	OptimizeStreamingLatency *int           `json:"optimize_streaming_latency,omitempty"`
}

// SynthesizeStream calls the ElevenLabs streaming TTS endpoint and returns an io.ReadCloser
// streaming the audio data. The caller must close the reader when done.
// Audio is returned as PCM 16-bit signed little-endian mono at 16000Hz.
func (c *Client) SynthesizeStream(ctx context.Context, voiceID string, req SynthesizeRequest) (io.ReadCloser, error) {
	if voiceID == "" {
		return nil, fmt.Errorf("elevenlabs: voice_id is required")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("elevenlabs: text is required")
	}

	// Request PCM format (16000Hz, 16-bit mono) for direct playback without transcoding
	url := fmt.Sprintf("%s/text-to-speech/%s/stream?output_format=pcm_16000", c.baseURL, voiceID)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("elevenlabs: API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	return resp.Body, nil
}
