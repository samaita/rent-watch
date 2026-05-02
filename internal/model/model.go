package model

import "time"

const (
	AvailabilityAvailable   = "available"
	AvailabilityUnavailable = "unavailable"

	CycleStatusRunning   = "running"
	CycleStatusCompleted = "completed"

	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"

	EventTypePriceChanged          = "price_changed"
	EventTypeAvailabilityAvailable = "availability_available"
	EventTypeAvailabilityGone      = "availability_unavailable"
)

type WatchPage struct {
	ID        int64
	SiteKey   string
	URL       string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CrawlCycle struct {
	ID         int64
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type CrawlRun struct {
	ID          int64
	CycleID     int64
	WatchPageID int64
	Status      string
	ErrorText   string
	FoundCount  int
	StartedAt   time.Time
	FinishedAt  time.Time
}

type Listing struct {
	ID                 int64
	CanonicalURL       string
	SiteKey            string
	WatchPageID        *int64
	PhotoURL           string
	Price              *int64
	SizeM2             *float64
	Bedrooms           *int64
	Bathrooms          *int64
	LocationKelurahan  string
	AvailabilityStatus string
	RawJSON            string
	FirstSeenAt        time.Time
	LastSeenAt         time.Time
	LastSeenCycleID    int64
}

type ListingEvent struct {
	ID         int64
	ListingID  int64
	CycleID    int64
	EventType  string
	OldValue   string
	NewValue   string
	CreatedAt  time.Time
	ListingURL string
}

type ExtractedListing struct {
	CanonicalURL      string
	PhotoURL          string
	Price             *int64
	SizeM2            *float64
	Bedrooms          *int64
	Bathrooms         *int64
	LocationKelurahan string
	RawJSON           string
}

type Counts struct {
	Available   int
	Unavailable int
	Total       int
}

type HeartbeatStatus struct {
	Counts              Counts
	WatchPages          int
	LastCompletedCycle  *time.Time
	LatestFailedWatches []FailedWatch
}

type FailedWatch struct {
	URL       string
	SiteKey   string
	ErrorText string
}
