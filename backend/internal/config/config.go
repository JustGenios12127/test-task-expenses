package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort         string
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string
	LLMAPIKey        string
	LLMModel         string
	LLMBaseURL       string
	TelegramBotToken string
	Timezone         string
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	cfg := Config{
		HTTPPort:         envOr("HTTP_PORT", "8080"),
		PostgresHost:     os.Getenv("POSTGRES_HOST"),
		PostgresPort:     envOr("POSTGRES_PORT", "5432"),
		PostgresUser:     os.Getenv("POSTGRES_USER"),
		PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:       os.Getenv("POSTGRES_DB"),
		PostgresSSLMode:  envOr("POSTGRES_SSLMODE", "disable"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		LLMModel:         os.Getenv("LLM_MODEL"),
		LLMBaseURL:       os.Getenv("LLM_BASE_URL"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		Timezone:         envOr("APP_TIMEZONE", "Asia/Yekaterinburg"),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Addr() string {
	return ":" + c.HTTPPort
}

func (c Config) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("APP_TIMEZONE: %w", err)
	}
	return loc, nil
}

func (c Config) PostgresDSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.PostgresUser, c.PostgresPassword),
		Host:   net.JoinHostPort(c.PostgresHost, c.PostgresPort),
		Path:   c.PostgresDB,
	}
	query := u.Query()
	query.Set("sslmode", c.PostgresSSLMode)
	u.RawQuery = query.Encode()
	return u.String()
}

func (c Config) validate() error {
	checks := []struct {
		name  string
		value string
	}{
		{name: "POSTGRES_HOST", value: c.PostgresHost},
		{name: "POSTGRES_USER", value: c.PostgresUser},
		{name: "POSTGRES_PASSWORD", value: c.PostgresPassword},
		{name: "POSTGRES_DB", value: c.PostgresDB},
		{name: "LLM_API_KEY", value: c.LLMAPIKey},
		{name: "LLM_MODEL", value: c.LLMModel},
		{name: "LLM_BASE_URL", value: c.LLMBaseURL},
		{name: "TELEGRAM_BOT_TOKEN", value: c.TelegramBotToken},
	}

	missing := make([]string, 0)
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" {
			missing = append(missing, check.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
