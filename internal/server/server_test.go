package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	napv1 "github.com/nupi-ai/nupi/api/nap/v1"

	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/cache"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/config"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/elevenlabs"
)

// mockSynthesizer implements elevenlabs.Synthesizer for testing.
type mockSynthesizer struct {
	data []byte
	err  error

	// Captured call arguments
	called  bool
	voiceID string
	req     elevenlabs.SynthesizeRequest
}

func (m *mockSynthesizer) SynthesizeStream(_ context.Context, voiceID string, req elevenlabs.SynthesizeRequest) (io.ReadCloser, error) {
	m.called = true
	m.voiceID = voiceID
	m.req = req
	if m.err != nil {
		return nil, m.err
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

func testConfig() config.Config {
	return config.Config{
		ListenAddr: "bufconn",
		APIKey:     "test-key",
		VoiceID:    "test-voice",
		Model:      "test-model",
		LogLevel:   "error",
		Language:   "client",
	}
}

// setup creates a bufconn gRPC server+client pair and returns the TTS client and a cleanup func.
func setup(t *testing.T, synth elevenlabs.Synthesizer, audioCache *cache.Cache) (napv1.TextToSpeechServiceClient, func()) {
	return setupWithConfig(t, testConfig(), synth, audioCache)
}

// setupWithConfig creates a bufconn gRPC server+client pair with a custom config.
func setupWithConfig(t *testing.T, cfg config.Config, synth elevenlabs.Synthesizer, audioCache *cache.Cache) (napv1.TextToSpeechServiceClient, func()) {
	t.Helper()
	buf := bufconn.Listen(1024 * 1024)

	srv := grpc.NewServer()
	svc := New(cfg, slog.Default(), synth, nil, audioCache)
	napv1.RegisterTextToSpeechServiceServer(srv, svc)

	go func() {
		if err := srv.Serve(buf); err != nil {
			t.Logf("server exited: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return buf.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	client := napv1.NewTextToSpeechServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return client, cleanup
}

// collectResponses drains all responses from the stream.
func collectResponses(t *testing.T, stream napv1.TextToSpeechService_StreamSynthesisClient) []*napv1.SynthesisResponse {
	t.Helper()
	var responses []*napv1.SynthesisResponse
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("recv: %v", err)
		}
		responses = append(responses, resp)
	}
	return responses
}

// collectResponsesAllowError drains responses, returning whatever was collected
// before an error (the server may close with an error after sending STATUS_ERROR).
func collectResponsesAllowError(stream napv1.TextToSpeechService_StreamSynthesisClient) []*napv1.SynthesisResponse {
	var responses []*napv1.SynthesisResponse
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		responses = append(responses, resp)
	}
	return responses
}

func TestStreamSynthesisSuccess(t *testing.T) {
	// 8192 bytes = 2 chunks of 4096
	pcm := make([]byte, 8192)
	for i := range pcm {
		pcm[i] = byte(i % 256)
	}
	mock := &mockSynthesizer{data: pcm}
	client, cleanup := setup(t, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "hello world",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponses(t, stream)

	// Expect: STARTED, PLAYING (no chunk), PLAYING+chunk1, PLAYING+chunk2, FINISHED
	if len(responses) < 4 {
		t.Fatalf("got %d responses, want at least 4", len(responses))
	}

	// First response: STARTED
	if responses[0].Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_STARTED {
		t.Errorf("response[0].Status = %v, want STARTED", responses[0].Status)
	}

	// Second response: PLAYING (status-only, no chunk)
	if responses[1].Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_PLAYING {
		t.Errorf("response[1].Status = %v, want PLAYING", responses[1].Status)
	}

	// Audio chunks
	var totalAudioBytes int
	var maxSeq uint64
	for _, resp := range responses {
		if resp.Chunk != nil {
			totalAudioBytes += len(resp.Chunk.Data)
			if resp.Chunk.Sequence > maxSeq {
				maxSeq = resp.Chunk.Sequence
			}
			// Verify duration: 4096 bytes / 2 = 2048 samples / 16000 * 1000 = 128ms
			if len(resp.Chunk.Data) == 4096 && resp.Chunk.DurationMs != 128 {
				t.Errorf("chunk seq %d: DurationMs = %d, want 128", resp.Chunk.Sequence, resp.Chunk.DurationMs)
			}
		}
	}

	if totalAudioBytes != 8192 {
		t.Errorf("total audio bytes = %d, want 8192", totalAudioBytes)
	}
	if maxSeq != 2 {
		t.Errorf("max sequence = %d, want 2", maxSeq)
	}

	// Last response: FINISHED
	last := responses[len(responses)-1]
	if last.Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_FINISHED {
		t.Errorf("last response Status = %v, want FINISHED", last.Status)
	}
}

func TestStreamSynthesisPartialChunk(t *testing.T) {
	// 5000 bytes = 4096 + 904
	pcm := make([]byte, 5000)
	mock := &mockSynthesizer{data: pcm}
	client, cleanup := setup(t, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "partial",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponses(t, stream)

	var chunks []*napv1.AudioChunk
	for _, r := range responses {
		if r.Chunk != nil {
			chunks = append(chunks, r.Chunk)
		}
	}

	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if len(chunks[0].Data) != 4096 {
		t.Errorf("chunk[0] = %d bytes, want 4096", len(chunks[0].Data))
	}
	if len(chunks[1].Data) != 904 {
		t.Errorf("chunk[1] = %d bytes, want 904", len(chunks[1].Data))
	}
	// 904 bytes / 2 = 452 samples; 452 * 1000 / 16000 = 28ms
	if chunks[1].DurationMs != 28 {
		t.Errorf("chunk[1] DurationMs = %d, want 28", chunks[1].DurationMs)
	}
}

func TestStreamSynthesisEmptyText(t *testing.T) {
	mock := &mockSynthesizer{data: []byte("unused")}
	client, cleanup := setup(t, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponsesAllowError(stream)

	found := false
	for _, r := range responses {
		if r.Status == napv1.SynthesisStatus_SYNTHESIS_STATUS_ERROR {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected STATUS_ERROR for empty text")
	}
}

func TestStreamSynthesisAPIError(t *testing.T) {
	mock := &mockSynthesizer{err: fmt.Errorf("elevenlabs: API error (status 500): internal")}
	client, cleanup := setup(t, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "fail",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponsesAllowError(stream)

	found := false
	for _, r := range responses {
		if r.Status == napv1.SynthesisStatus_SYNTHESIS_STATUS_ERROR {
			found = true
			if r.ErrorMessage == "" {
				t.Error("expected non-empty error message")
			}
			break
		}
	}
	if !found {
		t.Error("expected STATUS_ERROR for API failure")
	}
}

func TestStreamSynthesisStatusSequence(t *testing.T) {
	pcm := make([]byte, 100)
	mock := &mockSynthesizer{data: pcm}
	client, cleanup := setup(t, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "sequence test",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponses(t, stream)

	if len(responses) < 3 {
		t.Fatalf("got %d responses, want at least 3 (STARTED, PLAYING, FINISHED)", len(responses))
	}

	// First must be STARTED
	if responses[0].Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_STARTED {
		t.Errorf("first status = %v, want STARTED", responses[0].Status)
	}

	// Last must be FINISHED
	last := responses[len(responses)-1]
	if last.Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_FINISHED {
		t.Errorf("last status = %v, want FINISHED", last.Status)
	}

	// Middle statuses must be PLAYING
	for i := 1; i < len(responses)-1; i++ {
		if responses[i].Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_PLAYING {
			t.Errorf("response[%d].Status = %v, want PLAYING", i, responses[i].Status)
		}
	}
}

func TestStreamSynthesisMetadata(t *testing.T) {
	pcm := make([]byte, 100)
	mock := &mockSynthesizer{data: pcm}
	client, cleanup := setup(t, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "metadata test",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponses(t, stream)

	for _, r := range responses {
		if r.Chunk != nil && r.Chunk.Metadata != nil {
			if r.Chunk.Metadata["model"] != "test-model" {
				t.Errorf("chunk metadata model = %q, want %q", r.Chunk.Metadata["model"], "test-model")
			}
			if r.Chunk.Metadata["voice_id"] != "test-voice" {
				t.Errorf("chunk metadata voice_id = %q, want %q", r.Chunk.Metadata["voice_id"], "test-voice")
			}
			return // verified at least one chunk
		}
	}
	t.Error("no audio chunks with metadata found")
}

func TestStreamSynthesisCacheHit(t *testing.T) {
	dir := t.TempDir()
	audioCache, err := cache.New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}

	cfg := testConfig()
	key := cache.Key("cached text", cfg.Model, cfg.VoiceID, "auto", cfg.Stability, cfg.SimilarityBoost, cfg.OptimizeStreamingLatency)
	cachedData := make([]byte, 4096)
	for i := range cachedData {
		cachedData[i] = 0xAB
	}
	audioCache.Put(key, cachedData)

	mock := &mockSynthesizer{data: []byte("should not be used")}
	client, cleanup := setup(t, mock, audioCache)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "cached text",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponses(t, stream)

	if mock.called {
		t.Error("synthesizer was called despite cache hit")
	}

	// Check FINISHED metadata has source=cache
	last := responses[len(responses)-1]
	if last.Status != napv1.SynthesisStatus_SYNTHESIS_STATUS_FINISHED {
		t.Errorf("last status = %v, want FINISHED", last.Status)
	}
	if last.Metadata["source"] != "cache" {
		t.Errorf("FINISHED metadata source = %q, want %q", last.Metadata["source"], "cache")
	}

	// Verify audio data
	var totalBytes int
	var lastChunk *napv1.AudioChunk
	for _, r := range responses {
		if r.Chunk != nil {
			totalBytes += len(r.Chunk.Data)
			lastChunk = r.Chunk
		}
	}
	if totalBytes != 4096 {
		t.Errorf("total audio bytes = %d, want 4096", totalBytes)
	}
	if lastChunk == nil {
		t.Fatal("no audio chunks received")
	}
	if !lastChunk.Last {
		t.Error("last chunk from cache should have Last=true")
	}
}

func TestStreamSynthesisCacheMiss(t *testing.T) {
	dir := t.TempDir()
	audioCache, err := cache.New(dir, 1024*1024, nil)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}

	pcm := make([]byte, 2048)
	mock := &mockSynthesizer{data: pcm}
	client, cleanup := setup(t, mock, audioCache)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "new text",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}

	responses := collectResponses(t, stream)

	if !mock.called {
		t.Error("synthesizer should have been called on cache miss")
	}

	// Verify data was cached
	cfg := testConfig()
	key := cache.Key("new text", cfg.Model, cfg.VoiceID, "auto", cfg.Stability, cfg.SimilarityBoost, cfg.OptimizeStreamingLatency)
	cached, ok := audioCache.Get(key)
	if !ok {
		t.Error("data should have been stored in cache after miss")
	}
	if len(cached) != 2048 {
		t.Errorf("cached data = %d bytes, want 2048", len(cached))
	}

	// Verify no source=cache in FINISHED metadata
	last := responses[len(responses)-1]
	if last.Metadata["source"] == "cache" {
		t.Error("FINISHED metadata should not have source=cache on miss path")
	}
}

func TestStreamSynthesisLanguageCodePassedToAPI(t *testing.T) {
	pcm := make([]byte, 100)
	mock := &mockSynthesizer{data: pcm}

	cfg := testConfig()
	cfg.Language = "pl"
	client, cleanup := setupWithConfig(t, cfg, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "test language",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}
	collectResponses(t, stream)

	if !mock.called {
		t.Fatal("synthesizer was not called")
	}
	if mock.req.LanguageCode != "pl" {
		t.Errorf("LanguageCode = %q, want %q", mock.req.LanguageCode, "pl")
	}
}

func TestStreamSynthesisAutoOmitsLanguageCode(t *testing.T) {
	pcm := make([]byte, 100)
	mock := &mockSynthesizer{data: pcm}

	cfg := testConfig()
	cfg.Language = "auto"
	client, cleanup := setupWithConfig(t, cfg, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text: "test auto",
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}
	collectResponses(t, stream)

	if !mock.called {
		t.Fatal("synthesizer was not called")
	}
	if mock.req.LanguageCode != "" {
		t.Errorf("LanguageCode = %q, want empty (auto mode)", mock.req.LanguageCode)
	}
}

func TestStreamSynthesisClientModeWithMetadata(t *testing.T) {
	pcm := make([]byte, 100)
	mock := &mockSynthesizer{data: pcm}

	cfg := testConfig()
	cfg.Language = "client"
	client, cleanup := setupWithConfig(t, cfg, mock, nil)
	defer cleanup()

	stream, err := client.StreamSynthesis(context.Background(), &napv1.StreamSynthesisRequest{
		Text:     "test client mode",
		Metadata: map[string]string{"nupi.lang.iso1": "de"},
	})
	if err != nil {
		t.Fatalf("StreamSynthesis: %v", err)
	}
	collectResponses(t, stream)

	if !mock.called {
		t.Fatal("synthesizer was not called")
	}
	if mock.req.LanguageCode != "de" {
		t.Errorf("LanguageCode = %q, want %q", mock.req.LanguageCode, "de")
	}
}
