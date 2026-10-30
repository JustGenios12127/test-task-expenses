package config

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	HTTPPort         string
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string
	GroqAPIKey       string
	TelegramBotToken string
}

func Load() (Config, error) {
	if err := loadEnvFile(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPPort:         envOr("HTTP_PORT", "8080"),
		PostgresHost:     os.Getenv("POSTGRES_HOST"),
		PostgresPort:     envOr("POSTGRES_PORT", "5432"),
		PostgresUser:     os.Getenv("POSTGRES_USER"),
		PostgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		PostgresDB:       os.Getenv("POSTGRES_DB"),
		PostgresSSLMode:  envOr("POSTGRES_SSLMODE", "disable"),
		GroqAPIKey:       os.Getenv("GROQ_API_KEY"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Addr() string {
	return ":" + c.HTTPPort
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
		{name: "GROQ_API_KEY", value: c.GroqAPIKey},
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

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}

		key = strings.TrimSpace(key)
		value = trimQuotes(strings.TrimSpace(value))
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, lineNo)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file: %w", err)
	}
	return nil
}

func trimQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
