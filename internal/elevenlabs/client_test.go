package elevenlabs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSynthesizeStreamSuccess(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(pcm)
	}))
	defer srv.Close()

	c := &Client{
		httpClient: srv.Client(),
		apiKey:     "test-key",
		baseURL:    srv.URL,
	}

	rc, err := c.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: "hello", ModelID: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(got) != len(pcm) {
		t.Errorf("got %d bytes, want %d", len(got), len(pcm))
	}
}

func TestSynthesizeStreamOutputFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "output_format=pcm_16000") {
			t.Errorf("URL query = %q, want output_format=pcm_16000", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		httpClient: srv.Client(),
		apiKey:     "test-key",
		baseURL:    srv.URL,
	}

	rc, err := c.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: "hello", ModelID: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc.Close()
}

func TestSynthesizeStreamRequestBody(t *testing.T) {
	stability := 0.5
	similarity := 0.8

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if payload["text"] != "hello world" {
			t.Errorf("text = %v, want %q", payload["text"], "hello world")
		}
		if payload["model_id"] != "eleven_turbo_v2_5" {
			t.Errorf("model_id = %v, want %q", payload["model_id"], "eleven_turbo_v2_5")
		}
		vs, ok := payload["voice_settings"].(map[string]interface{})
		if !ok {
			t.Fatal("voice_settings missing")
		}
		if vs["stability"] != 0.5 {
			t.Errorf("stability = %v, want 0.5", vs["stability"])
		}
		if vs["similarity_boost"] != 0.8 {
			t.Errorf("similarity_boost = %v, want 0.8", vs["similarity_boost"])
		}

		if r.Header.Get("xi-api-key") != "test-key" {
			t.Errorf("xi-api-key = %q, want %q", r.Header.Get("xi-api-key"), "test-key")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		httpClient: srv.Client(),
		apiKey:     "test-key",
		baseURL:    srv.URL,
	}

	rc, err := c.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{
		Text:    "hello world",
		ModelID: "eleven_turbo_v2_5",
		VoiceSettings: &VoiceSettings{
			Stability:       &stability,
			SimilarityBoost: &similarity,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc.Close()
}

func TestSynthesizeStreamAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate_limit"}`))
	}))
	defer srv.Close()

	c := &Client{
		httpClient: srv.Client(),
		apiKey:     "test-key",
		baseURL:    srv.URL,
	}

	_, err := c.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: "hello", ModelID: "m1"})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %q, want to contain status 429", err.Error())
	}
}

func TestSynthesizeStreamEmptyVoiceID(t *testing.T) {
	c := &Client{apiKey: "k", baseURL: "http://localhost"}
	_, err := c.SynthesizeStream(context.Background(), "", SynthesizeRequest{Text: "hello"})
	if err == nil {
		t.Fatal("expected error for empty voice_id")
	}
}

func TestSynthesizeStreamEmptyText(t *testing.T) {
	c := &Client{apiKey: "k", baseURL: "http://localhost"}
	_, err := c.SynthesizeStream(context.Background(), "v1", SynthesizeRequest{Text: ""})
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}
