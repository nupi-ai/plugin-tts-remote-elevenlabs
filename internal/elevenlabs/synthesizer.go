package elevenlabs

import (
	"context"
	"io"
)

// Synthesizer abstracts the ElevenLabs TTS streaming API so that the server
// can be tested with a mock implementation.
type Synthesizer interface {
	SynthesizeStream(ctx context.Context, voiceID string, req SynthesizeRequest) (io.ReadCloser, error)
}
