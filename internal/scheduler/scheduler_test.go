package scheduler

import (
	"testing"
	"time"

	"github.com/axonigma/rent-watcher/internal/config"
)

func TestRandomDelayWithinBounds(t *testing.T) {
	s := NewForTest(config.Config{
		SameSiteMinDelay: time.Minute,
		SameSiteMaxDelay: 5 * time.Minute,
	}, 123)
	for range 100 {
		delay := s.randomDelay()
		if delay < time.Minute || delay > 5*time.Minute+59*time.Second {
			t.Fatalf("delay out of bounds: %v", delay)
		}
	}
}
