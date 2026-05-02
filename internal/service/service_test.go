package service

import (
	"testing"
)

import "github.com/axonigma/rent-watcher/internal/model"

func TestFormatCounts(t *testing.T) {
	got := formatCounts(model.Counts{Available: 2, Unavailable: 3, Total: 5})
	want := "available=2\nunavailable=3\ntotal=5"
	if got != want {
		t.Fatalf("formatCounts() = %q, want %q", got, want)
	}
}
