# Nupi ElevenLabs Remote TTS

Remote text-to-speech adapter for the Nupi platform, backed by the ElevenLabs API. Implements the `TextToSpeechService` gRPC contract from `nap.nupi.ai/v1alpha1`.

## Quick Start

```bash
make dist
./dist/tts-remote-elevenlabs
```

The adapter is launched automatically by the Nupi daemon when configured as the TTS slot adapter.

## Configuration

Options are set via the Nupi adapter configuration system:

| Option | Default | Description |
|--------|---------|-------------|
| `api_key` | — | ElevenLabs API key (required) |
| `voice_id` | `UgBBYS2sOqTuMpoF3BR0` | ElevenLabs voice ID (default: Mark) |
| `model` | `eleven_turbo_v2_5` | ElevenLabs model for synthesis |
| `stability` | `0.5` | Voice stability (0.0-1.0) |
| `similarity_boost` | `0.75` | Voice similarity (0.0-1.0) |
| `optimize_streaming_latency` | `0` | Latency optimization level (0-4) |

## Repository Structure

- `cmd/adapter/` — Release entrypoint
- `internal/server/` — gRPC implementation of `TextToSpeechService`
- `internal/elevenlabs/` — ElevenLabs API client
- `internal/config/` — Configuration loader
- `internal/telemetry/` — Telemetry recorder
- `plugin.yaml` — NAP manifest consumed by the adapter runtime

## Environment Variables

| Env var | Default | Purpose |
|---------|---------|---------|
| `NUPI_ADAPTER_CONFIG` | — | JSON payload injected by the adapter runner |
| `NUPI_ADAPTER_LISTEN_ADDR` | `127.0.0.1:50051` | gRPC bind address |

## License

See [LICENSE](LICENSE).
