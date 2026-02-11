package elevenlabs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
)

// StubSynthesizer implements the Synthesizer interface with deterministic
// PCM output (silence). It is intended for CI and testing environments
// where the real ElevenLabs API is unavailable.
type StubSynthesizer struct {
	log *slog.Logger
}

// NewStubSynthesizer returns a stub that generates silent PCM data
// proportional to the input text length.
func NewStubSynthesizer(logger *slog.Logger) *StubSynthesizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &StubSynthesizer{log: logger}
}

// SynthesizeStream returns an io.ReadCloser streaming deterministic silent PCM.
// The output size is len(text) * 320 bytes (320 bytes ≈ 10 ms at 16 kHz mono PCM16).
func (s *StubSynthesizer) SynthesizeStream(_ context.Context, voiceID string, req SynthesizeRequest) (io.ReadCloser, error) {
	if voiceID == "" {
		return nil, fmt.Errorf("elevenlabs: voice_id is required")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("elevenlabs: text is required")
	}

	pcmLen := len(req.Text) * 320
	pcm := make([]byte, pcmLen)

	s.log.Info("stub synthesis",
		"text_length", len(req.Text),
		"voice_id", voiceID,
		"bytes", pcmLen,
	)

	return io.NopCloser(bytes.NewReader(pcm)), nil
}
