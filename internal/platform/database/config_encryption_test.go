package database

import (
	"os"
	"strings"
	"testing"
)

// TestSaveConfig_EncryptsDiscordWebhook verifies the Discord webhook URL (a
// bearer credential) is stored through the encrypted serializer like the
// other integration secrets.
func TestSaveConfig_EncryptsDiscordWebhook(t *testing.T) {
	_ = os.Setenv("DB_PATH", ":memory:")
	defer func() { _ = os.Unsetenv("DB_PATH") }()
	t.Setenv("CONFIG_ENCRYPTION_KEY", "test-key")

	d, err := InitDB()
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	cfg, err := GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	const hook = "https://discord.example/api/webhooks/1/PLACEHOLDER"
	cfg.DiscordWebhook = hook
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	var raw string
	if err := d.Raw("SELECT discord_webhook FROM configs LIMIT 1").Scan(&raw).Error; err != nil {
		t.Fatalf("raw select: %v", err)
	}
	if !strings.HasPrefix(raw, "enc:v1:") || strings.Contains(raw, "PLACEHOLDER") {
		t.Errorf("discord_webhook stored unencrypted: %q", raw)
	}

	got, err := GetConfig()
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.DiscordWebhook != hook {
		t.Errorf("DiscordWebhook round-trip = %q, want %q", got.DiscordWebhook, hook)
	}
}
