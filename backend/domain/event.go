package domain

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// EventImpact represents the economic significance of an event.
type EventImpact string

const (
	EventImpactHigh   EventImpact = "high"
	EventImpactMedium EventImpact = "medium"
	EventImpactLow    EventImpact = "low"
)

// ParseEventImpact normalizes an impact string to a known EventImpact value.
// Unknown values default to medium.
func ParseEventImpact(raw string) EventImpact {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "high":
		return EventImpactHigh
	case "moderate", "medium":
		return EventImpactMedium
	case "low":
		return EventImpactLow
	default:
		return EventImpactMedium
	}
}

// Event represents a normalized macroeconomic calendar event.
// All timestamps are in UTC.
type Event struct {
	id        string
	country   string
	title     string
	impact    EventImpact
	timestamp time.Time
}

// NewEvent creates an Event. If id is empty, a deterministic ID is generated
// from country + title + timestamp.
func NewEvent(id, country, title string, impact EventImpact, timestamp time.Time) (Event, error) {
	if country == "" {
		return Event{}, fmt.Errorf("event country must not be empty")
	}
	if title == "" {
		return Event{}, fmt.Errorf("event title must not be empty")
	}
	if timestamp.IsZero() {
		return Event{}, fmt.Errorf("event timestamp must not be zero")
	}

	ts := timestamp.UTC()

	if id == "" {
		id = generateEventID(country, title, ts)
	}

	return Event{
		id:        id,
		country:   country,
		title:     title,
		impact:    impact,
		timestamp: ts,
	}, nil
}

// ID returns the event's unique identifier.
func (e Event) ID() string { return e.id }

// Country returns the originating country.
func (e Event) Country() string { return e.country }

// Title returns the event title / report name.
func (e Event) Title() string { return e.title }

// Impact returns the economic impact level.
func (e Event) Impact() EventImpact { return e.impact }

// Timestamp returns the event time in UTC.
func (e Event) Timestamp() time.Time { return e.timestamp }

// generateEventID produces a short deterministic hash from country+title+time.
func generateEventID(country, title string, ts time.Time) string {
	raw := fmt.Sprintf("%s|%s|%s", country, title, ts.Format(time.RFC3339))
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:8])
}
