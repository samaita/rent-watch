package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/axonigma/rent-watcher/internal/model"
)

type fileSeed struct {
	Watches []watchSeed `json:"watches" yaml:"watches"`
}

type watchSeed struct {
	SiteKey string `json:"site_key" yaml:"site_key"`
	URL     string `json:"url" yaml:"url"`
	Enabled bool   `json:"enabled" yaml:"enabled"`
}

func Load(path string) ([]model.WatchPage, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var parsed fileSeed
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &parsed)
	default:
		err = json.Unmarshal(data, &parsed)
	}
	if err != nil {
		return nil, fmt.Errorf("decode seed file: %w", err)
	}

	out := make([]model.WatchPage, 0, len(parsed.Watches))
	for _, watch := range parsed.Watches {
		if watch.URL == "" || watch.SiteKey == "" {
			continue
		}
		out = append(out, model.WatchPage{
			SiteKey: watch.SiteKey,
			URL:     watch.URL,
			Enabled: watch.Enabled,
		})
	}
	return out, nil
}
