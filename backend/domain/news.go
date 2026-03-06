package domain

import (
	"fmt"
	"time"
)

// NewsStatus represents the publication state of a news article.
type NewsStatus string

const (
	NewsStatusPlanned  NewsStatus = "planned"
	NewsStatusReleased NewsStatus = "released"
	NewsStatusInfo     NewsStatus = "info"
)

// ValidNewsStatuses enumerates all valid status values.
var ValidNewsStatuses = []NewsStatus{NewsStatusPlanned, NewsStatusReleased, NewsStatusInfo}

// IsValidNewsStatus checks if a given string is a valid news status.
func IsValidNewsStatus(s string) bool {
	for _, valid := range ValidNewsStatuses {
		if string(valid) == s {
			return true
		}
	}
	return false
}

// NewsArticle represents a single news/update entry.
// It is a value object: immutable, validated at construction.
type NewsArticle struct {
	slug     string
	title    string
	date     time.Time
	status   NewsStatus
	tags     []string
	body     string
	excerpt  string
	eta      *time.Time
	priority string
}

// NewNewsArticle constructs and validates a NewsArticle.
func NewNewsArticle(
	slug, title string,
	date time.Time,
	status NewsStatus,
	tags []string,
	body string,
	eta *time.Time,
	priority string,
) (NewsArticle, error) {
	if slug == "" {
		return NewsArticle{}, fmt.Errorf("news article slug must not be empty")
	}
	if title == "" {
		return NewsArticle{}, fmt.Errorf("news article title must not be empty")
	}
	if !IsValidNewsStatus(string(status)) {
		return NewsArticle{}, fmt.Errorf("invalid news status: %q", status)
	}

	excerpt := generateExcerpt(body, 200)

	tagsCopy := make([]string, len(tags))
	copy(tagsCopy, tags)

	return NewsArticle{
		slug:     slug,
		title:    title,
		date:     date,
		status:   status,
		tags:     tagsCopy,
		body:     body,
		excerpt:  excerpt,
		eta:      eta,
		priority: priority,
	}, nil
}

// Slug returns the URL-friendly identifier.
func (n NewsArticle) Slug() string { return n.slug }

// Title returns the article title.
func (n NewsArticle) Title() string { return n.title }

// Date returns the publication date.
func (n NewsArticle) Date() time.Time { return n.date }

// Status returns the article status.
func (n NewsArticle) Status() NewsStatus { return n.status }

// Tags returns a defensive copy of tags.
func (n NewsArticle) Tags() []string {
	cp := make([]string, len(n.tags))
	copy(cp, n.tags)
	return cp
}

// Body returns the raw Markdown body.
func (n NewsArticle) Body() string { return n.body }

// Excerpt returns a short plain-text excerpt.
func (n NewsArticle) Excerpt() string { return n.excerpt }

// ETA returns the optional estimated completion date.
func (n NewsArticle) ETA() *time.Time { return n.eta }

// Priority returns the priority level (may be empty).
func (n NewsArticle) Priority() string { return n.priority }

// generateExcerpt truncates text to maxLen characters at a word boundary.
func generateExcerpt(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	truncated := text[:maxLen]
	lastSpace := -1
	for i := len(truncated) - 1; i >= 0; i-- {
		if truncated[i] == ' ' {
			lastSpace = i
			break
		}
	}
	if lastSpace > 0 {
		return truncated[:lastSpace] + "..."
	}
	return truncated + "..."
}
