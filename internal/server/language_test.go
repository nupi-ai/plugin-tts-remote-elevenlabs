package server

import "testing"

func TestResolveLanguage_ClientWithMetadata(t *testing.T) {
	meta := map[string]string{"nupi.lang.iso1": "pl", "nupi.lang.english": "Polish"}
	if got := resolveLanguage("client", meta); got != "pl" {
		t.Errorf("resolveLanguage(client, meta) = %q, want %q", got, "pl")
	}
}

func TestResolveLanguage_ClientNilMetadata(t *testing.T) {
	if got := resolveLanguage("client", nil); got != "auto" {
		t.Errorf("resolveLanguage(client, nil) = %q, want %q", got, "auto")
	}
}

func TestResolveLanguage_ClientEmptyMetadata(t *testing.T) {
	if got := resolveLanguage("client", map[string]string{}); got != "auto" {
		t.Errorf("resolveLanguage(client, {}) = %q, want %q", got, "auto")
	}
}

func TestResolveLanguage_ClientEmptyISO(t *testing.T) {
	meta := map[string]string{"nupi.lang.iso1": ""}
	if got := resolveLanguage("client", meta); got != "auto" {
		t.Errorf("resolveLanguage(client, empty iso) = %q, want %q", got, "auto")
	}
}

func TestResolveLanguage_AutoWithMetadata(t *testing.T) {
	meta := map[string]string{"nupi.lang.iso1": "pl"}
	if got := resolveLanguage("auto", meta); got != "auto" {
		t.Errorf("resolveLanguage(auto, meta) = %q, want %q", got, "auto")
	}
}

func TestResolveLanguage_AutoNilMetadata(t *testing.T) {
	if got := resolveLanguage("auto", nil); got != "auto" {
		t.Errorf("resolveLanguage(auto, nil) = %q, want %q", got, "auto")
	}
}

func TestResolveLanguage_SpecificWithMetadata(t *testing.T) {
	meta := map[string]string{"nupi.lang.iso1": "pl"}
	if got := resolveLanguage("en", meta); got != "en" {
		t.Errorf("resolveLanguage(en, meta) = %q, want %q", got, "en")
	}
}

func TestResolveLanguage_SpecificNilMetadata(t *testing.T) {
	if got := resolveLanguage("de", nil); got != "de" {
		t.Errorf("resolveLanguage(de, nil) = %q, want %q", got, "de")
	}
}

func TestResolveLanguage_ClientWhitespaceISO(t *testing.T) {
	meta := map[string]string{"nupi.lang.iso1": "  pl  "}
	if got := resolveLanguage("client", meta); got != "pl" {
		t.Errorf("resolveLanguage(client, whitespace iso) = %q, want %q", got, "pl")
	}
}

func TestResolveLanguage_ClientOnlyWhitespaceISO(t *testing.T) {
	meta := map[string]string{"nupi.lang.iso1": "   "}
	if got := resolveLanguage("client", meta); got != "auto" {
		t.Errorf("resolveLanguage(client, only whitespace iso) = %q, want %q", got, "auto")
	}
}
