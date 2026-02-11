package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	napv1 "github.com/nupi-ai/nupi/api/nap/v1"

	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/adapterinfo"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/cache"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/config"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/elevenlabs"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/server"
	"github.com/nupi-ai/plugin-tts-remote-elevenlabs/internal/telemetry"
)

// lazyTTSServer wraps a TextToSpeechServiceServer and allows deferred initialization.
// It returns Unavailable errors until the underlying server is set via setServer.
type lazyTTSServer struct {
	napv1.UnimplementedTextToSpeechServiceServer
	server atomic.Pointer[napv1.TextToSpeechServiceServer]
}

func (l *lazyTTSServer) setServer(srv napv1.TextToSpeechServiceServer) {
	l.server.Store(&srv)
}

func (l *lazyTTSServer) StreamSynthesis(req *napv1.StreamSynthesisRequest, stream napv1.TextToSpeechService_StreamSynthesisServer) error {
	srv := l.server.Load()
	if srv == nil {
		return status.Error(codes.Unavailable, "TTS service is initializing, please retry in a moment")
	}
	return (*srv).StreamSynthesis(req, stream)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Loader{}.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("starting adapter",
		"adapter", adapterinfo.Info.Name,
		"adapter_slug", adapterinfo.Info.Slug,
		"adapter_version", adapterinfo.Version(),
		"listen_addr", cfg.ListenAddr,
		"voice_id", cfg.VoiceID,
		"model", cfg.Model,
		"stability", logFloatPtrField(cfg.Stability),
		"similarity_boost", logFloatPtrField(cfg.SimilarityBoost),
		"optimize_streaming_latency", logIntPtrField(cfg.OptimizeStreamingLatency),
	)

	recorder := telemetry.NewRecorder(logger)

	// STEP 1: Bind port IMMEDIATELY (before initializing client)
	// This allows the manager's readiness check to succeed while client initializes.
	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error("failed to bind listener", "error", err)
		os.Exit(1)
	}
	defer lis.Close()
	logger.Info("listener bound, port ready", "addr", lis.Addr().String())

	// STEP 2: Setup gRPC server with lazy TTS service wrapper
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, healthServer)

	serviceName := napv1.TextToSpeechService_ServiceDesc.ServiceName
	healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_NOT_SERVING)
	healthServer.SetServingStatus(serviceName, healthgrpc.HealthCheckResponse_NOT_SERVING)

	lazyService := &lazyTTSServer{}
	napv1.RegisterTextToSpeechServiceServer(grpcServer, lazyService)

	// STEP 3: Start gRPC server in background (port is already bound)
	serverErr := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serverErr <- err
		}
	}()
	logger.Info("gRPC server started (NOT_SERVING while initializing)")

	// STEP 4: Initialize synthesizer
	var synthesizer elevenlabs.Synthesizer
	if cfg.UseStubSynthesizer {
		synthesizer = elevenlabs.NewStubSynthesizer(logger)
		logger.Info("using STUB synthesizer — responses are deterministic, NOT from ElevenLabs API")
	} else {
		synthesizer = elevenlabs.NewClient(cfg.APIKey)
		logger.Info("ElevenLabs client initialized")
	}

	// STEP 5: Initialize cache (if configured)
	var audioCache *cache.Cache
	if cfg.CacheMaxSizeMB > 0 && cfg.CacheDir != "" {
		var err error
		audioCache, err = cache.New(cfg.CacheDir, int64(cfg.CacheMaxSizeMB)*1024*1024, logger)
		if err != nil {
			logger.Warn("failed to initialize cache, continuing without", "error", err)
		} else {
			logger.Info("audio cache initialized", "dir", cfg.CacheDir, "max_size_mb", cfg.CacheMaxSizeMB)
		}
	}

	// STEP 6: Activate the real TTS service now that client is ready
	realService := server.New(cfg, logger, synthesizer, recorder, audioCache)
	lazyService.setServer(realService)

	healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(serviceName, healthgrpc.HealthCheckResponse_SERVING)
	logger.Info("adapter ready to serve requests")

	// STEP 7: Setup graceful shutdown
	go func() {
		<-ctx.Done()
		logger.Info("shutdown requested, stopping gRPC server")
		healthServer.SetServingStatus(serviceName, healthgrpc.HealthCheckResponse_NOT_SERVING)
		healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_NOT_SERVING)

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			logger.Warn("graceful stop timed out, forcing stop")
			grpcServer.Stop()
		}
	}()

	// STEP 8: Wait for server to finish or error
	select {
	case err := <-serverErr:
		logger.Error("gRPC server terminated with error", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		// Normal shutdown via signal
	}

	logger.Info("adapter stopped")
}

func newLogger(level string) *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})
	return slog.New(handler)
}

func parseLevel(value string) slog.Leveler {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func logFloatPtrField(v *float64) any {
	if v == nil {
		return "default"
	}
	return *v
}

func logIntPtrField(v *int) any {
	if v == nil {
		return "default"
	}
	return *v
}
