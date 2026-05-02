package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"github.com/axonigma/rent-watcher/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Init(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`CREATE TABLE IF NOT EXISTS watch_pages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site_key TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS crawl_cycles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS crawl_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cycle_id INTEGER NOT NULL,
			watch_page_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			error_text TEXT NOT NULL DEFAULT '',
			found_count INTEGER NOT NULL DEFAULT 0,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			UNIQUE(cycle_id, watch_page_id)
		);`,
		`CREATE TABLE IF NOT EXISTS listings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			canonical_url TEXT NOT NULL UNIQUE,
			site_key TEXT NOT NULL,
			watch_page_id INTEGER,
			photo_url TEXT NOT NULL DEFAULT '',
			price INTEGER,
			size_m2 REAL,
			bedrooms INTEGER,
			bathrooms INTEGER,
			location_kelurahan TEXT NOT NULL DEFAULT '',
			availability_status TEXT NOT NULL,
			raw_json TEXT NOT NULL DEFAULT '',
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			last_seen_cycle_id INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS listing_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			listing_id INTEGER NOT NULL,
			cycle_id INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			old_value TEXT NOT NULL DEFAULT '',
			new_value TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SeedWatchPages(ctx context.Context, watches []model.WatchPage) error {
	count, err := s.CountWatchPages(ctx)
	if err != nil {
		return err
	}
	if count > 0 || len(watches) == 0 {
		return nil
	}
	now := nowString(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, watch := range watches {
		if _, err := tx.ExecContext(ctx, `INSERT INTO watch_pages(site_key,url,enabled,created_at,updated_at) VALUES (?,?,?,?,?)`,
			watch.SiteKey, watch.URL, boolToInt(watch.Enabled), now, now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CountWatchPages(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_pages`).Scan(&count)
	return count, err
}

func (s *Store) ListEnabledWatchPages(ctx context.Context) ([]model.WatchPage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,site_key,url,enabled,created_at,updated_at FROM watch_pages WHERE enabled=1 ORDER BY site_key,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WatchPage
	for rows.Next() {
		var w model.WatchPage
		var enabled int
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&w.ID, &w.SiteKey, &w.URL, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		w.Enabled = enabled == 1
		w.CreatedAt = mustParseTime(createdAt)
		w.UpdatedAt = mustParseTime(updatedAt)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) GetOrCreateRunningCycle(ctx context.Context, now time.Time) (model.CrawlCycle, error) {
	var cycleID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM crawl_cycles WHERE status=? ORDER BY id DESC LIMIT 1`, model.CycleStatusRunning).
		Scan(&cycleID)
	if err == nil {
		return hydrateCycleRow(ctx, s.db, cycleID)
	}
	if err != sql.ErrNoRows {
		return model.CrawlCycle{}, err
	}

	res, err := s.db.ExecContext(ctx, `INSERT INTO crawl_cycles(status,started_at,finished_at) VALUES (?,?,NULL)`, model.CycleStatusRunning, nowString(now))
	if err != nil {
		return model.CrawlCycle{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.CrawlCycle{}, err
	}
	return hydrateCycleRow(ctx, s.db, id)
}

func hydrateCycleRow(ctx context.Context, q queryer, id int64) (model.CrawlCycle, error) {
	var cycle model.CrawlCycle
	var startedAt string
	var finished sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT id,status,started_at,finished_at FROM crawl_cycles WHERE id=?`, id).
		Scan(&cycle.ID, &cycle.Status, &startedAt, &finished); err != nil {
		return model.CrawlCycle{}, err
	}
	cycle.StartedAt = mustParseTime(startedAt)
	if finished.Valid {
		t := mustParseTime(finished.String)
		cycle.FinishedAt = &t
	}
	return cycle, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) PendingWatchPagesForCycle(ctx context.Context, cycleID int64) ([]model.WatchPage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wp.id,wp.site_key,wp.url,wp.enabled,wp.created_at,wp.updated_at
		FROM watch_pages wp
		WHERE wp.enabled=1
		AND NOT EXISTS (
			SELECT 1 FROM crawl_runs cr
			WHERE cr.cycle_id = ? AND cr.watch_page_id = wp.id
		)
		ORDER BY wp.site_key, wp.id
	`, cycleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WatchPage
	for rows.Next() {
		var w model.WatchPage
		var enabled int
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&w.ID, &w.SiteKey, &w.URL, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		w.Enabled = enabled == 1
		w.CreatedAt = mustParseTime(createdAt)
		w.UpdatedAt = mustParseTime(updatedAt)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) RecordSuccessfulRun(ctx context.Context, cycleID int64, watch model.WatchPage, startedAt, finishedAt time.Time, listings []model.ExtractedListing) ([]model.ListingEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO crawl_runs(cycle_id,watch_page_id,status,error_text,found_count,started_at,finished_at)
		VALUES (?,?,?,?,?,?,?)
	`, cycleID, watch.ID, model.RunStatusSuccess, "", len(listings), nowString(startedAt), nowString(finishedAt)); err != nil {
		return nil, err
	}

	var events []model.ListingEvent
	for _, extracted := range listings {
		extractedRaw := extracted.RawJSON
		if extractedRaw == "" {
			payload, err := json.Marshal(extracted)
			if err != nil {
				return nil, err
			}
			extractedRaw = string(payload)
		}

		listing, found, err := getListingByURL(ctx, tx, extracted.CanonicalURL)
		if err != nil {
			return nil, err
		}
		if !found {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO listings(canonical_url,site_key,watch_page_id,photo_url,price,size_m2,bedrooms,bathrooms,location_kelurahan,availability_status,raw_json,first_seen_at,last_seen_at,last_seen_cycle_id)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			`,
				extracted.CanonicalURL, watch.SiteKey, watch.ID, extracted.PhotoURL, nullableInt64(extracted.Price), nullableFloat64(extracted.SizeM2),
				nullableInt64(extracted.Bedrooms), nullableInt64(extracted.Bathrooms), extracted.LocationKelurahan, model.AvailabilityAvailable,
				extractedRaw, nowString(startedAt), nowString(finishedAt), cycleID,
			)
			if err != nil {
				return nil, err
			}
			listingID, err := res.LastInsertId()
			if err != nil {
				return nil, err
			}
			ev := model.ListingEvent{
				ListingID:  listingID,
				CycleID:    cycleID,
				EventType:  model.EventTypeAvailabilityAvailable,
				OldValue:   "",
				NewValue:   model.AvailabilityAvailable,
				CreatedAt:  finishedAt,
				ListingURL: extracted.CanonicalURL,
			}
			if err := insertEvent(ctx, tx, ev); err != nil {
				return nil, err
			}
			events = append(events, ev)
			continue
		}

		if listing.AvailabilityStatus == model.AvailabilityUnavailable {
			ev := model.ListingEvent{
				ListingID:  listing.ID,
				CycleID:    cycleID,
				EventType:  model.EventTypeAvailabilityAvailable,
				OldValue:   model.AvailabilityUnavailable,
				NewValue:   model.AvailabilityAvailable,
				CreatedAt:  finishedAt,
				ListingURL: extracted.CanonicalURL,
			}
			if err := insertEvent(ctx, tx, ev); err != nil {
				return nil, err
			}
			events = append(events, ev)
		}
		if priceChanged(listing.Price, extracted.Price) {
			ev := model.ListingEvent{
				ListingID:  listing.ID,
				CycleID:    cycleID,
				EventType:  model.EventTypePriceChanged,
				OldValue:   formatNullableInt64(listing.Price),
				NewValue:   formatNullableInt64(extracted.Price),
				CreatedAt:  finishedAt,
				ListingURL: extracted.CanonicalURL,
			}
			if err := insertEvent(ctx, tx, ev); err != nil {
				return nil, err
			}
			events = append(events, ev)
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE listings
			SET watch_page_id=?, photo_url=?, price=?, size_m2=?, bedrooms=?, bathrooms=?, location_kelurahan=?, availability_status=?, raw_json=?, last_seen_at=?, last_seen_cycle_id=?
			WHERE id=?
		`, watch.ID, extracted.PhotoURL, nullableInt64(extracted.Price), nullableFloat64(extracted.SizeM2), nullableInt64(extracted.Bedrooms),
			nullableInt64(extracted.Bathrooms), extracted.LocationKelurahan, model.AvailabilityAvailable, extractedRaw, nowString(finishedAt), cycleID, listing.ID,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) RecordFailedRun(ctx context.Context, cycleID int64, watch model.WatchPage, startedAt, finishedAt time.Time, errText string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO crawl_runs(cycle_id,watch_page_id,status,error_text,found_count,started_at,finished_at)
		VALUES (?,?,?,?,?,?,?)
	`, cycleID, watch.ID, model.RunStatusFailed, errText, 0, nowString(startedAt), nowString(finishedAt))
	return err
}

func (s *Store) FinalizeCycleIfComplete(ctx context.Context, cycleID int64, now time.Time) ([]model.ListingEvent, bool, error) {
	var watchCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM watch_pages WHERE enabled=1`).Scan(&watchCount); err != nil {
		return nil, false, err
	}
	var runCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crawl_runs WHERE cycle_id=?`, cycleID).Scan(&runCount); err != nil {
		return nil, false, err
	}
	if runCount < watchCount {
		return nil, false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, canonical_url
		FROM listings
		WHERE availability_status=? AND last_seen_cycle_id<>?
	`, model.AvailabilityAvailable, cycleID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var events []model.ListingEvent
	for rows.Next() {
		var listingID int64
		var url string
		if err := rows.Scan(&listingID, &url); err != nil {
			return nil, false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE listings SET availability_status=? WHERE id=?`, model.AvailabilityUnavailable, listingID); err != nil {
			return nil, false, err
		}
		ev := model.ListingEvent{
			ListingID:  listingID,
			CycleID:    cycleID,
			EventType:  model.EventTypeAvailabilityGone,
			OldValue:   model.AvailabilityAvailable,
			NewValue:   model.AvailabilityUnavailable,
			CreatedAt:  now,
			ListingURL: url,
		}
		if err := insertEvent(ctx, tx, ev); err != nil {
			return nil, false, err
		}
		events = append(events, ev)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE crawl_cycles SET status=?, finished_at=? WHERE id=?`, model.CycleStatusCompleted, nowString(now), cycleID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return events, true, nil
}

func (s *Store) Counts(ctx context.Context) (model.Counts, error) {
	var counts model.Counts
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM listings WHERE availability_status=?`, model.AvailabilityAvailable).Scan(&counts.Available); err != nil {
		return counts, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM listings WHERE availability_status=?`, model.AvailabilityUnavailable).Scan(&counts.Unavailable); err != nil {
		return counts, err
	}
	counts.Total = counts.Available + counts.Unavailable
	return counts, nil
}

func (s *Store) HeartbeatStatus(ctx context.Context) (model.HeartbeatStatus, error) {
	counts, err := s.Counts(ctx)
	if err != nil {
		return model.HeartbeatStatus{}, err
	}
	watchCount, err := s.CountWatchPages(ctx)
	if err != nil {
		return model.HeartbeatStatus{}, err
	}
	status := model.HeartbeatStatus{
		Counts:     counts,
		WatchPages: watchCount,
	}

	var finished sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT finished_at FROM crawl_cycles WHERE status=? ORDER BY id DESC LIMIT 1`, model.CycleStatusCompleted).Scan(&finished); err == nil && finished.Valid {
		t := mustParseTime(finished.String)
		status.LastCompletedCycle = &t
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT wp.url, wp.site_key, cr.error_text
		FROM crawl_runs cr
		JOIN watch_pages wp ON wp.id = cr.watch_page_id
		WHERE cr.id IN (
			SELECT id FROM crawl_runs
			WHERE status=?
			ORDER BY cycle_id DESC, id DESC
		)
		ORDER BY cr.cycle_id DESC, cr.id DESC
		LIMIT 5
	`, model.RunStatusFailed)
	if err != nil {
		return model.HeartbeatStatus{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var failed model.FailedWatch
		if err := rows.Scan(&failed.URL, &failed.SiteKey, &failed.ErrorText); err != nil {
			return model.HeartbeatStatus{}, err
		}
		status.LatestFailedWatches = append(status.LatestFailedWatches, failed)
	}
	return status, rows.Err()
}

func getListingByURL(ctx context.Context, q scanner, canonicalURL string) (model.Listing, bool, error) {
	var listing model.Listing
	var watchPageID sql.NullInt64
	var price sql.NullInt64
	var size sql.NullFloat64
	var bedrooms sql.NullInt64
	var bathrooms sql.NullInt64
	var firstSeenAt string
	var lastSeenAt string
	err := q.QueryRowContext(ctx, `
		SELECT id,canonical_url,site_key,watch_page_id,photo_url,price,size_m2,bedrooms,bathrooms,location_kelurahan,availability_status,raw_json,first_seen_at,last_seen_at,last_seen_cycle_id
		FROM listings WHERE canonical_url=?
	`, canonicalURL).Scan(
		&listing.ID, &listing.CanonicalURL, &listing.SiteKey, &watchPageID, &listing.PhotoURL, &price, &size, &bedrooms, &bathrooms,
		&listing.LocationKelurahan, &listing.AvailabilityStatus, &listing.RawJSON, &firstSeenAt, &lastSeenAt, &listing.LastSeenCycleID,
	)
	if err == sql.ErrNoRows {
		return model.Listing{}, false, nil
	}
	if err != nil {
		return model.Listing{}, false, err
	}
	if watchPageID.Valid {
		id := watchPageID.Int64
		listing.WatchPageID = &id
	}
	if price.Valid {
		v := price.Int64
		listing.Price = &v
	}
	if size.Valid {
		v := size.Float64
		listing.SizeM2 = &v
	}
	if bedrooms.Valid {
		v := bedrooms.Int64
		listing.Bedrooms = &v
	}
	if bathrooms.Valid {
		v := bathrooms.Int64
		listing.Bathrooms = &v
	}
	listing.FirstSeenAt = mustParseTime(firstSeenAt)
	listing.LastSeenAt = mustParseTime(lastSeenAt)
	return listing, true, nil
}

type scanner interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func insertEvent(ctx context.Context, tx *sql.Tx, event model.ListingEvent) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO listing_events(listing_id,cycle_id,event_type,old_value,new_value,created_at)
		VALUES (?,?,?,?,?,?)
	`, event.ListingID, event.CycleID, event.EventType, event.OldValue, event.NewValue, nowString(event.CreatedAt))
	return err
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableFloat64(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func formatNullableInt64(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func priceChanged(oldValue, newValue *int64) bool {
	if oldValue == nil && newValue == nil {
		return false
	}
	if oldValue == nil || newValue == nil {
		return true
	}
	return *oldValue != *newValue
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nowString(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		panic(fmt.Sprintf("parse time %q: %v", raw, err))
	}
	return t
}
