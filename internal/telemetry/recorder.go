package telemetry

import "log/slog"

// Recorder centralises telemetry (logs, metrics) for the adapter. Phase 1 only
// emits structured logs via slog; future releases will integrate with
// distributed tracing and metrics aggregation.
type Recorder struct {
	logger *slog.Logger
}

// NewRecorder constructs a telemetry recorder using the provided slog.Logger.
func NewRecorder(logger *slog.Logger) *Recorder {
	return &Recorder{logger: logger}
}

// Logger returns the underlying slog.Logger for direct use.
func (r *Recorder) Logger() *slog.Logger {
	return r.logger
}
