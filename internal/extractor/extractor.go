package extractor

import (
	"context"
	"fmt"

	"github.com/axonigma/rent-watcher/internal/model"
)

type Extractor interface {
	ExtractListings(ctx context.Context, watch model.WatchPage) ([]model.ExtractedListing, error)
}

func New(siteKey string, opts BrowserOptions) (Extractor, error) {
	switch siteKey {
	case "olx":
		return NewOLX(opts), nil
	default:
		return nil, fmt.Errorf("unsupported site_key %q", siteKey)
	}
}
