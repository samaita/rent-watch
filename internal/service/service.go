package service

import (
	"context"
	"fmt"

	"github.com/axonigma/rent-watcher/internal/model"
	"github.com/axonigma/rent-watcher/internal/store"
	"github.com/axonigma/rent-watcher/internal/version"
)

type Service struct {
	store *store.Store
}

func New(st *store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) VersionText(context.Context) string {
	return version.String
}

func (s *Service) ListText(ctx context.Context) (string, error) {
	counts, err := s.store.Counts(ctx)
	if err != nil {
		return "", err
	}
	return formatCounts(counts), nil
}

func formatCounts(counts model.Counts) string {
	return fmt.Sprintf("available=%d\nunavailable=%d\ntotal=%d", counts.Available, counts.Unavailable, counts.Total)
}
