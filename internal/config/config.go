package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	SQLitePath       string
	SeedPath         string
	TelegramBotToken string
	TelegramUserIDs  []int64
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
	cfg := Config{
		SQLitePath:       getenv("SQLITE_PATH", "rent-watcher.sqlite"),
		SeedPath:         os.Getenv("SEED_PATH"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramUserIDs:  parseInt64List(os.Getenv("TELEGRAM_ALLOWED_USER_IDS")),
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
