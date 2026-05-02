package store

import (
	"context"
	"testing"
	"time"

	"github.com/axonigma/rent-watcher/internal/model"
)

func TestFinalizeCycleMarksMissingListingUnavailable(t *testing.T) {
	ctx := context.Background()
	st, err := Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedWatchPages(ctx, []model.WatchPage{
		{SiteKey: "olx", URL: "https://example.com/1", Enabled: true},
		{SiteKey: "olx", URL: "https://example.com/2", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}

	cycle1, err := st.GetOrCreateRunningCycle(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	watches, err := st.PendingWatchPagesForCycle(ctx, cycle1.ID)
	if err != nil {
		t.Fatal(err)
	}
	priceA := int64(100)
	priceB := int64(200)
	if _, err := st.RecordSuccessfulRun(ctx, cycle1.ID, watches[0], time.Now(), time.Now(), []model.ExtractedListing{
		{CanonicalURL: "https://listing/A", Price: &priceA},
		{CanonicalURL: "https://listing/B", Price: &priceB},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordFailedRun(ctx, cycle1.ID, watches[1], time.Now(), time.Now(), "timeout"); err != nil {
		t.Fatal(err)
	}
	if _, finalized, err := st.FinalizeCycleIfComplete(ctx, cycle1.ID, time.Now()); err != nil || !finalized {
		t.Fatalf("finalize cycle1 finalized=%v err=%v", finalized, err)
	}

	cycle2, err := st.GetOrCreateRunningCycle(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	watches, err = st.PendingWatchPagesForCycle(ctx, cycle2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RecordSuccessfulRun(ctx, cycle2.ID, watches[0], time.Now(), time.Now(), []model.ExtractedListing{
		{CanonicalURL: "https://listing/A", Price: &priceA},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordFailedRun(ctx, cycle2.ID, watches[1], time.Now(), time.Now(), "timeout"); err != nil {
		t.Fatal(err)
	}
	events, finalized, err := st.FinalizeCycleIfComplete(ctx, cycle2.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !finalized {
		t.Fatal("cycle2 not finalized")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].ListingURL != "https://listing/B" {
		t.Fatalf("unexpected listing url %q", events[0].ListingURL)
	}

	counts, err := st.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Available != 1 || counts.Unavailable != 1 || counts.Total != 2 {
		t.Fatalf("counts = %+v", counts)
	}
}
