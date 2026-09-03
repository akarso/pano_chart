package ports

import (
	"context"
	"time"

	"pano_chart/backend/domain"
)

// EventProviderPort fetches economic calendar events from an external source.
type EventProviderPort interface {
	// FetchEvents retrieves events for the given date range and optional country.
	// Dates are inclusive. Country may be empty (= all countries).
	FetchEvents(ctx context.Context, dateFrom, dateTo time.Time, country string) ([]domain.Event, error)
}
