package models

import (
	"errors"
	"strings"
	"testing"
)

func TestEncryptValue_NoKeyKeepsPlaintext(t *testing.T) {
	t.Setenv("CONFIG_ENCRYPTION_KEY", "")
	got, err := encryptValue("secret")
	if err != nil || got != "secret" {
		t.Errorf("encryptValue() = %q, %v; want plaintext", got, err)
	}
}

func TestEncryptValue_RoundTrip(t *testing.T) {
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")

	enc, err := encryptValue("smtp-password")
	if err != nil {
		t.Fatalf("encryptValue: %v", err)
	}
	if !strings.HasPrefix(enc, encryptedPrefix) || strings.Contains(enc, "smtp-password") {
		t.Fatalf("value not encrypted: %q", enc)
	}

	dec, err := decryptValue(enc)
	if err != nil || dec != "smtp-password" {
		t.Errorf("decryptValue() = %q, %v", dec, err)
	}

	if empty, _ := encryptValue(""); empty != "" {
		t.Errorf("empty value should stay empty, got %q", empty)
	}
}

func TestDecryptValue(t *testing.T) {
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")
	enc, _ := encryptValue("token")

	if got, err := decryptValue("legacy-plaintext"); err != nil || got != "legacy-plaintext" {
		t.Errorf("legacy plaintext: got %q, %v", got, err)
	}

	t.Setenv("CONFIG_ENCRYPTION_KEY", "other-key")
	if _, err := decryptValue(enc); err == nil {
		t.Error("expected error with the wrong key")
	}

	t.Setenv("CONFIG_ENCRYPTION_KEY", "")
	if _, err := decryptValue(enc); !errors.Is(err, ErrEncryptionKeyMissing) {
		t.Errorf("expected ErrEncryptionKeyMissing, got %v", err)
	}

	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")
	if _, err := decryptValue(encryptedPrefix + "!!!"); err == nil {
		t.Error("expected error for malformed value")
	}
	if _, err := decryptValue(encryptedPrefix + "AAAA"); err == nil {
		t.Error("expected error for truncated value")
	}
}
