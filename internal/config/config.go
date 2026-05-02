package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	SQLitePath       string
	SeedPath         string
	TelegramBotToken string
	TelegramUserIDs  []int64
	Debug            bool
	Timezone         string
	CycleInterval    time.Duration
	HeartbeatEvery   time.Duration
	SameSiteMinDelay time.Duration
	SameSiteMaxDelay time.Duration
	ChromeHeadless   bool
	ChromePath       string
	ScrapeTimeout    time.Duration
}

func Load() (Config, error) {
	if err := loadLocalDotEnv(); err != nil {
		return Config{}, err
	}

	cfg := Config{
		SQLitePath:       getenv("SQLITE_PATH", "rent-watcher.sqlite"),
		SeedPath:         os.Getenv("SEED_PATH"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramUserIDs:  parseInt64List(os.Getenv("TELEGRAM_ALLOWED_USER_IDS")),
		Debug:            mustBool("DEBUG", false),
		Timezone:         getenv("TIMEZONE", "Asia/Jakarta"),
		CycleInterval:    mustDuration("CYCLE_INTERVAL", "12h"),
		HeartbeatEvery:   mustDuration("HEARTBEAT_INTERVAL", "1h"),
		SameSiteMinDelay: mustDuration("SAME_SITE_MIN_DELAY", "1m"),
		SameSiteMaxDelay: mustDuration("SAME_SITE_MAX_DELAY", "5m"),
		ChromeHeadless:   mustBool("CHROME_HEADLESS", true),
		ChromePath:       os.Getenv("CHROME_PATH"),
		ScrapeTimeout:    mustDuration("SCRAPE_TIMEOUT", "45s"),
	}
	if cfg.SQLitePath == "" {
		return Config{}, fmt.Errorf("SQLITE_PATH is required")
	}
	if cfg.SameSiteMaxDelay < cfg.SameSiteMinDelay {
		return Config{}, fmt.Errorf("SAME_SITE_MAX_DELAY must be >= SAME_SITE_MIN_DELAY")
	}
	return cfg, nil
}

func loadLocalDotEnv() error {
	path := filepath.Join(".", ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("parse %s:%d: missing '='", path, lineNo)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("parse %s:%d: empty key", path, lineNo)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from %s: %w", key, path, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustDuration(key, fallback string) time.Duration {
	raw := getenv(key, fallback)
	d, err := time.ParseDuration(raw)
	if err != nil {
		panic(fmt.Sprintf("invalid duration %s=%s", key, raw))
	}
	return d
}

func mustBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		panic(fmt.Sprintf("invalid bool %s=%s", key, raw))
	}
	return v
}

func parseInt64List(raw string) []int64 {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}
