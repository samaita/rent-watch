package scheduler

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/axonigma/rent-watcher/internal/config"
	"github.com/axonigma/rent-watcher/internal/extractor"
	"github.com/axonigma/rent-watcher/internal/model"
	"github.com/axonigma/rent-watcher/internal/notifier"
	"github.com/axonigma/rent-watcher/internal/store"
	"github.com/rs/zerolog/log"
)

type Scheduler struct {
	store    *store.Store
	notifier notifier.Notifier
	cfg      config.Config
	rng      *rand.Rand
}

func New(st *store.Store, n notifier.Notifier, cfg config.Config) *Scheduler {
	return &Scheduler{
		store:    st,
		notifier: n,
		cfg:      cfg,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	go s.runCycles(ctx)
	go s.runHeartbeat(ctx)
}

func (s *Scheduler) runCycles(ctx context.Context) {
	s.runSingleCycle(ctx)
	ticker := time.NewTicker(s.cfg.CycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runSingleCycle(ctx)
		}
	}
}

func (s *Scheduler) runSingleCycle(ctx context.Context) {
	now := time.Now()
	cycle, err := s.store.GetOrCreateRunningCycle(ctx, now)
	if err != nil {
		log.Error().Err(err).Msg("get or create cycle")
		return
	}
	watches, err := s.store.PendingWatchPagesForCycle(ctx, cycle.ID)
	if err != nil {
		log.Error().Err(err).Int64("cycle_id", cycle.ID).Msg("pending watch pages")
		return
	}
	if len(watches) == 0 {
		s.finalizeCycle(ctx, cycle.ID)
		return
	}

	grouped := groupBySite(watches)
	for siteKey, siteWatches := range grouped {
		ext, err := extractor.New(siteKey, extractor.BrowserOptions{
			Headless: s.cfg.ChromeHeadless,
			ExecPath: s.cfg.ChromePath,
			Timeout:  s.cfg.ScrapeTimeout,
		})
		if err != nil {
			log.Error().Err(err).Str("site_key", siteKey).Msg("build extractor")
			continue
		}
		s.runSiteGroup(ctx, cycle.ID, ext, siteWatches)
	}
	s.finalizeCycle(ctx, cycle.ID)
}

func (s *Scheduler) runSiteGroup(ctx context.Context, cycleID int64, ext extractor.Extractor, watches []model.WatchPage) {
	for i, watch := range watches {
		if i > 0 {
			delay := s.randomDelay()
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		startedAt := time.Now()
		listings, err := ext.ExtractListings(ctx, watch)
		finishedAt := time.Now()
		if err != nil {
			if recErr := s.store.RecordFailedRun(ctx, cycleID, watch, startedAt, finishedAt, err.Error()); recErr != nil {
				log.Error().
					Err(recErr).
					Int64("cycle_id", cycleID).
					Int64("watch_page_id", watch.ID).
					Str("site_key", watch.SiteKey).
					Str("url", watch.URL).
					Msg("record failed run")
			}
			continue
		}
		events, err := s.store.RecordSuccessfulRun(ctx, cycleID, watch, startedAt, finishedAt, listings)
		if err != nil {
			log.Error().
				Err(err).
				Int64("cycle_id", cycleID).
				Int64("watch_page_id", watch.ID).
				Str("site_key", watch.SiteKey).
				Str("url", watch.URL).
				Msg("record successful run")
			continue
		}
		s.notifyEvents(ctx, events)
	}
}

func (s *Scheduler) finalizeCycle(ctx context.Context, cycleID int64) {
	events, finalized, err := s.store.FinalizeCycleIfComplete(ctx, cycleID, time.Now())
	if err != nil {
		log.Error().Err(err).Int64("cycle_id", cycleID).Msg("finalize cycle")
		return
	}
	if finalized {
		s.notifyEvents(ctx, events)
	}
}

func (s *Scheduler) notifyEvents(ctx context.Context, events []model.ListingEvent) {
	for _, event := range events {
		if err := s.notifier.Send(ctx, notifier.FormatEvent(event)); err != nil {
			log.Error().
				Err(err).
				Str("event_type", string(event.EventType)).
				Str("listing_url", event.ListingURL).
				Msg("notify event")
		}
	}
}

func (s *Scheduler) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, err := s.store.HeartbeatStatus(ctx)
			if err != nil {
				log.Error().Err(err).Msg("heartbeat status")
				continue
			}
			if err := s.notifier.Send(ctx, notifier.FormatHeartbeat(status)); err != nil {
				log.Error().Err(err).Msg("send heartbeat")
			}
		}
	}
}

func groupBySite(watches []model.WatchPage) map[string][]model.WatchPage {
	grouped := make(map[string][]model.WatchPage)
	for _, watch := range watches {
		grouped[watch.SiteKey] = append(grouped[watch.SiteKey], watch)
	}
	return grouped
}

func (s *Scheduler) randomDelay() time.Duration {
	if s.cfg.SameSiteMaxDelay <= s.cfg.SameSiteMinDelay {
		return s.cfg.SameSiteMinDelay
	}
	diff := s.cfg.SameSiteMaxDelay - s.cfg.SameSiteMinDelay
	base := s.cfg.SameSiteMinDelay + time.Duration(s.rng.Int63n(int64(diff)+1))
	secs := time.Duration(s.rng.Intn(60)) * time.Second
	return base + secs
}

func NewForTest(cfg config.Config, seed int64) *Scheduler {
	return &Scheduler{
		cfg: cfg,
		rng: rand.New(rand.NewSource(seed)),
	}
}

type noopStore struct{}

var _ = sync.Mutex{}
