package admin

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const defaultCleanUpHours = 24

var ErrInvalidHours = errors.New("hours must be greater than 0")

// Service contains admin maintenance business logic.
type Service struct {
	store Store
}

// CleanUpResult contains metadata and deletion counts from a cleanup run.
type CleanUpResult struct {
	Hours        int         `json:"hours"`
	Cutoff       time.Time   `json:"cutoff"`
	Deleted      DeletedRows `json:"deleted"`
	TotalDeleted int64       `json:"total_deleted"`
}

// NewService creates a new admin service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// CleanUpDB deletes all session-scoped data older than the configured threshold.
// If hours is nil, it defaults to 24.
func (s *Service) CleanUpDB(ctx context.Context, hours *int) (*CleanUpResult, error) {
	h := defaultCleanUpHours
	if hours != nil {
		h = *hours
	}
	if h <= 0 {
		return nil, ErrInvalidHours
	}

	cutoff := time.Now().UTC().Add(-time.Duration(h) * time.Hour)
	deleted, err := s.store.CleanUpOlderThan(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("cleanup store: %w", err)
	}

	return &CleanUpResult{
		Hours:        h,
		Cutoff:       cutoff,
		Deleted:      deleted,
		TotalDeleted: deleted.Total(),
	}, nil
}
