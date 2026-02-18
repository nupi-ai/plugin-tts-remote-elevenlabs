package server

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	napv1 "github.com/nupi-ai/nupi/api/nap/v1"

	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/adapterinfo"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/cache"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/config"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/elevenlabs"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/telemetry"
)

const (
	defaultSampleRate = 16000
	defaultChannels   = 1
	defaultBitDepth   = 16
	chunkSize         = 4096 // bytes per chunk (~128ms at 16kHz mono PCM16)
)

// Server implements the TextToSpeechService and synthesizes audio via ElevenLabs.
type Server struct {
	napv1.UnimplementedTextToSpeechServiceServer

	cfg     config.Config
	log     *slog.Logger
	client  elevenlabs.Synthesizer
	metrics *telemetry.Recorder
	cache   *cache.Cache // nil when caching is disabled
}

// New returns a new Server instance.
func New(cfg config.Config, logger *slog.Logger, client elevenlabs.Synthesizer, metrics *telemetry.Recorder, audioCache *cache.Cache) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if client == nil {
		panic("server: elevenlabs client must not be nil")
	}
	if metrics == nil {
		metrics = telemetry.NewRecorder(logger)
	}
	return &Server{
		cfg: cfg,
		log: logger.With(
			"component", "server",
			"model", cfg.Model,
			"voice_id", cfg.VoiceID,
		),
		client:  client,
		metrics: metrics,
		cache:   audioCache,
	}
}

// StreamSynthesis accepts a text synthesis request and streams back audio chunks.
func (s *Server) StreamSynthesis(req *napv1.StreamSynthesisRequest, stream napv1.TextToSpeechService_StreamSynthesisServer) error {
	if req == nil {
		return fmt.Errorf("server: request is nil")
	}

	sessionID := req.GetSessionId()
	streamID := req.GetStreamId()
	text := req.GetText()

	logEntry := s.log.With(
		"session_id", sessionID,
		"stream_id", streamID,
		"text_length", len(text),
	)

	if text == "" {
		logEntry.Warn("empty text in synthesis request")
		return s.sendError(stream, "text is required")
	}

	// Resolve language from config mode and request metadata.
	resolvedLang := resolveLanguage(s.cfg.Language, req.GetMetadata())
	logEntry = logEntry.With("language", resolvedLang)

	logEntry.Info("synthesis request received")

	// Send STARTED status
	if err := s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_STARTED, nil); err != nil {
		logEntry.Error("failed to send started status", "error", err)
		return err
	}

	// Build synthesis request
	synthesisReq := elevenlabs.SynthesizeRequest{
		Text:    text,
		ModelID: s.cfg.Model,
	}

	// Pass language_code to ElevenLabs when a specific language is resolved.
	// "auto" means let ElevenLabs auto-detect, so we omit the field.
	if resolvedLang != "auto" {
		synthesisReq.LanguageCode = resolvedLang
	}

	// Apply voice settings if configured
	if s.cfg.Stability != nil || s.cfg.SimilarityBoost != nil {
		synthesisReq.VoiceSettings = &elevenlabs.VoiceSettings{
			Stability:       s.cfg.Stability,
			SimilarityBoost: s.cfg.SimilarityBoost,
		}
	}

	// Apply latency optimization if configured
	if s.cfg.OptimizeStreamingLatency != nil {
		synthesisReq.OptimizeStreamingLatency = s.cfg.OptimizeStreamingLatency
	}

	// Compute cache key
	var cacheKey string
	if s.cache != nil {
		cacheKey = cache.Key(text, s.cfg.Model, s.cfg.VoiceID, resolvedLang, s.cfg.Stability, s.cfg.SimilarityBoost, s.cfg.OptimizeStreamingLatency)
	}

	// Cache hit path
	if s.cache != nil {
		if data, ok := s.cache.Get(cacheKey); ok {
			logEntry.Info("cache hit", "key", cacheKey)
			return s.streamFromBytes(data, text, stream, logEntry)
		}
		logEntry.Debug("cache miss", "key", cacheKey)
	}

	ctx := stream.Context()
	start := time.Now()

	// Call ElevenLabs streaming API
	audioStream, err := s.client.SynthesizeStream(ctx, s.cfg.VoiceID, synthesisReq)
	if err != nil {
		logEntry.Error("elevenlabs synthesis failed", "error", err)
		return s.sendError(stream, fmt.Sprintf("synthesis failed: %v", err))
	}
	defer audioStream.Close()

	// Send PLAYING status
	if err := s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_PLAYING, nil); err != nil {
		logEntry.Error("failed to send playing status", "error", err)
		return err
	}

	// Stream audio chunks
	var sequence uint64
	buffer := make([]byte, chunkSize)
	totalBytes := 0
	var accumulated []byte

	for {
		select {
		case <-ctx.Done():
			logEntry.Info("synthesis interrupted", "reason", ctx.Err())
			return s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_INTERRUPTED, map[string]string{
				"reason": ctx.Err().Error(),
			})
		default:
		}

		n, err := audioStream.Read(buffer)
		if n > 0 {
			totalBytes += n
			sequence++

			chunk := &napv1.AudioChunk{
				Data:     append([]byte{}, buffer[:n]...),
				Sequence: sequence,
				First:    sequence == 1,
				Last:     err == io.EOF,
				Metadata: adapterinfo.SynthesisMetadata(s.cfg.Model, s.cfg.VoiceID),
			}

			// Calculate duration (PCM16, mono, 16kHz)
			samples := n / 2 // 16-bit = 2 bytes per sample
			durationMs := uint32((samples * 1000) / defaultSampleRate)
			chunk.DurationMs = durationMs

			resp := &napv1.SynthesisResponse{
				Status: napv1.SynthesisStatus_SYNTHESIS_STATUS_PLAYING,
				Chunk:  chunk,
			}

			if err := stream.Send(resp); err != nil {
				logEntry.Error("failed to send audio chunk", "error", err, "sequence", sequence)
				return err
			}

			// Accumulate for cache
			if s.cache != nil {
				accumulated = append(accumulated, buffer[:n]...)
			}

			logEntry.Debug("sent audio chunk",
				"sequence", sequence,
				"bytes", n,
				"duration_ms", durationMs,
			)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			logEntry.Error("error reading audio stream", "error", err)
			return s.sendError(stream, fmt.Sprintf("stream read error: %v", err))
		}
	}

	duration := time.Since(start)
	logEntry.Info("synthesis completed",
		"total_bytes", totalBytes,
		"chunks", sequence,
		"duration_sec", duration.Seconds(),
	)

	// Store in cache
	if s.cache != nil && len(accumulated) > 0 {
		if err := s.cache.Put(cacheKey, accumulated); err != nil {
			logEntry.Warn("failed to store in cache", "error", err)
		}
	}

	// Send FINISHED status
	metadata := map[string]string{
		"total_bytes":  fmt.Sprintf("%d", totalBytes),
		"total_chunks": fmt.Sprintf("%d", sequence),
		"duration_sec": fmt.Sprintf("%.2f", duration.Seconds()),
		"text_length":  fmt.Sprintf("%d", len(text)),
	}

	return s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_FINISHED, metadata)
}

// streamFromBytes streams pre-cached audio data using the same chunking logic as the live path.
func (s *Server) streamFromBytes(data []byte, text string, stream napv1.TextToSpeechService_StreamSynthesisServer, logEntry *slog.Logger) error {
	// Send PLAYING status
	if err := s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_PLAYING, nil); err != nil {
		return err
	}

	var sequence uint64
	totalBytes := len(data)

	ctx := stream.Context()
	for offset := 0; offset < len(data); offset += chunkSize {
		if err := ctx.Err(); err != nil {
			return s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_INTERRUPTED, map[string]string{
				"reason": err.Error(),
			})
		}

		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}

		n := end - offset
		sequence++

		chunk := &napv1.AudioChunk{
			Data:     data[offset:end],
			Sequence: sequence,
			First:    sequence == 1,
			Last:     end == len(data),
			Metadata: adapterinfo.SynthesisMetadata(s.cfg.Model, s.cfg.VoiceID),
		}

		samples := n / 2
		chunk.DurationMs = uint32((samples * 1000) / defaultSampleRate)

		resp := &napv1.SynthesisResponse{
			Status: napv1.SynthesisStatus_SYNTHESIS_STATUS_PLAYING,
			Chunk:  chunk,
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	logEntry.Info("served from cache",
		"total_bytes", totalBytes,
		"chunks", sequence,
	)

	metadata := map[string]string{
		"total_bytes":  fmt.Sprintf("%d", totalBytes),
		"total_chunks": fmt.Sprintf("%d", sequence),
		"text_length":  fmt.Sprintf("%d", len(text)),
		"source":       "cache",
	}

	return s.sendStatus(stream, napv1.SynthesisStatus_SYNTHESIS_STATUS_FINISHED, metadata)
}

func (s *Server) sendStatus(stream napv1.TextToSpeechService_StreamSynthesisServer, status napv1.SynthesisStatus, metadata map[string]string) error {
	resp := &napv1.SynthesisResponse{
		Status:   status,
		Metadata: metadata,
	}
	return stream.Send(resp)
}

func (s *Server) sendError(stream napv1.TextToSpeechService_StreamSynthesisServer, message string) error {
	resp := &napv1.SynthesisResponse{
		Status:       napv1.SynthesisStatus_SYNTHESIS_STATUS_ERROR,
		ErrorMessage: message,
	}
	if err := stream.Send(resp); err != nil {
		return err
	}
	return fmt.Errorf("synthesis error: %s", message)
}

// resolveLanguage returns the effective ISO 639-1 language code to pass to the
// ElevenLabs API as language_code, based on the configured language mode and
// request metadata.
//
// Modes:
//   - "client": read nupi.lang.iso1 from metadata; fall back to "auto" if absent.
//   - "auto":   always return "auto" (ElevenLabs auto-detects, language_code omitted).
//   - other:    return the configured ISO 639-1 code verbatim (ignore metadata).
func resolveLanguage(configLang string, metadata map[string]string) string {
	if configLang != "client" {
		return configLang
	}
	if code := strings.TrimSpace(metadata["nupi.lang.iso1"]); code != "" {
		return code
	}
	return "auto"
}
