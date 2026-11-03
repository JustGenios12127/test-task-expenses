package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsEnvFileWithoutOverridingEnvironment(t *testing.T) {
	dir := t.TempDir()
	content := "POSTGRES_HOST=db\nPOSTGRES_USER=\"app\"\nPOSTGRES_PASSWORD=secret\nPOSTGRES_DB=expenses\nGROQ_API_KEY=key\nTELEGRAM_BOT_TOKEN=file-token\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	t.Chdir(dir)
	for _, key := range []string{"POSTGRES_HOST", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "GROQ_API_KEY"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PostgresHost != "db" || cfg.PostgresUser != "app" || cfg.TelegramBotToken != "env-token" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadWorksWithoutEnvFile(t *testing.T) {
	t.Chdir(t.TempDir())
	for key, value := range map[string]string{
		"POSTGRES_HOST": "db", "POSTGRES_USER": "app", "POSTGRES_PASSWORD": "secret",
		"POSTGRES_DB": "expenses", "GROQ_API_KEY": "key", "TELEGRAM_BOT_TOKEN": "token",
	} {
		t.Setenv(key, value)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
}
