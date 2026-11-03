package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsEnvFileWithoutOverridingEnvironment(t *testing.T) {
	dir := t.TempDir()
	content := "POSTGRES_HOST=db\nPOSTGRES_USER=\"app\"\nPOSTGRES_PASSWORD=secret\nPOSTGRES_DB=expenses\nLLM_API_KEY=key\nLLM_MODEL=model\nLLM_BASE_URL=http://llm\nTELEGRAM_BOT_TOKEN=file-token\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	t.Chdir(dir)
	for _, key := range []string{"POSTGRES_HOST", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "LLM_API_KEY", "LLM_MODEL", "LLM_BASE_URL"} {
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
		"POSTGRES_DB": "expenses", "LLM_API_KEY": "key", "LLM_MODEL": "model", "LLM_BASE_URL": "http://llm",
		"TELEGRAM_BOT_TOKEN": "token",
	} {
		t.Setenv(key, value)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestLoadRequiresLLMSettings(t *testing.T) {
	t.Chdir(t.TempDir())
	for key, value := range map[string]string{
		"POSTGRES_HOST": "db", "POSTGRES_USER": "app", "POSTGRES_PASSWORD": "secret",
		"POSTGRES_DB": "expenses", "TELEGRAM_BOT_TOKEN": "token",
		"LLM_API_KEY": "", "LLM_MODEL": "", "LLM_BASE_URL": "",
	} {
		t.Setenv(key, value)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_MODEL", "LLM_BASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error %q does not name %s", err, key)
		}
	}
}
