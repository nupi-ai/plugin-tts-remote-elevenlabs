package elevenlabs

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func TestStubSynthesizeStreamSuccess(t *testing.T) {
	stub := NewStubSynthesizer(slog.Default())
	rc, err := stub.SynthesizeStream(context.Background(), "voice-1", SynthesizeRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	want := len("hello") * 320
	if len(data) != want {
		t.Errorf("got %d bytes, want %d", len(data), want)
	}
}

func TestStubSynthesizeStreamDeterministic(t *testing.T) {
	stub := NewStubSynthesizer(slog.Default())
	req := SynthesizeRequest{Text: "deterministic test"}

	rc1, err := stub.SynthesizeStream(context.Background(), "voice-1", req)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	data1, _ := io.ReadAll(rc1)
	rc1.Close()

	rc2, err := stub.SynthesizeStream(context.Background(), "voice-1", req)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	data2, _ := io.ReadAll(rc2)
	rc2.Close()

	if len(data1) != len(data2) {
		t.Fatalf("length mismatch: %d vs %d", len(data1), len(data2))
	}
	for i := range data1 {
		if data1[i] != data2[i] {
			t.Fatalf("byte %d differs: %d vs %d", i, data1[i], data2[i])
		}
	}
}

func TestStubSynthesizeStreamEmptyText(t *testing.T) {
	stub := NewStubSynthesizer(slog.Default())
	_, err := stub.SynthesizeStream(context.Background(), "voice-1", SynthesizeRequest{Text: ""})
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestStubSynthesizeStreamEmptyVoiceID(t *testing.T) {
	stub := NewStubSynthesizer(slog.Default())
	_, err := stub.SynthesizeStream(context.Background(), "", SynthesizeRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected error for empty voiceID")
	}
}

func TestStubSynthesizeStreamNilLogger(t *testing.T) {
	stub := NewStubSynthesizer(nil)
	rc, err := stub.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if len(data) != len("test")*320 {
		t.Errorf("got %d bytes, want %d", len(data), len("test")*320)
	}
}

func TestStubSynthesizeStreamLongText(t *testing.T) {
	stub := NewStubSynthesizer(slog.Default())

	short := "hi"
	long := "this is a much longer sentence for testing proportional output size"

	rcShort, err := stub.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: short})
	if err != nil {
		t.Fatalf("short text error: %v", err)
	}
	dataShort, _ := io.ReadAll(rcShort)
	rcShort.Close()

	rcLong, err := stub.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: long})
	if err != nil {
		t.Fatalf("long text error: %v", err)
	}
	dataLong, _ := io.ReadAll(rcLong)
	rcLong.Close()

	wantShort := len(short) * 320
	wantLong := len(long) * 320

	if len(dataShort) != wantShort {
		t.Errorf("short: got %d bytes, want %d", len(dataShort), wantShort)
	}
	if len(dataLong) != wantLong {
		t.Errorf("long: got %d bytes, want %d", len(dataLong), wantLong)
	}
	if len(dataLong) <= len(dataShort) {
		t.Errorf("longer text should produce more bytes: short=%d, long=%d", len(dataShort), len(dataLong))
	}
}
