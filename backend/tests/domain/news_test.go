package domain_test

import (
	"testing"
	"time"

	"pano_chart/backend/domain"
)

func TestNewNewsArticle_Valid(t *testing.T) {
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	eta := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	tags := []string{"feature", "timeframe"}

	article, err := domain.NewNewsArticle(
		"add-1w-timeframe",
		"1W Timeframe Coming Soon",
		date,
		domain.NewsStatusPlanned,
		tags,
		"Body text for testing.",
		&eta,
		"normal",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if article.Slug() != "add-1w-timeframe" {
		t.Errorf("Slug = %q, want %q", article.Slug(), "add-1w-timeframe")
	}
	if article.Title() != "1W Timeframe Coming Soon" {
		t.Errorf("Title = %q", article.Title())
	}
	if article.Date() != date {
		t.Errorf("Date = %v, want %v", article.Date(), date)
	}
	if article.Status() != domain.NewsStatusPlanned {
		t.Errorf("Status = %q", article.Status())
	}
	if len(article.Tags()) != 2 || article.Tags()[0] != "feature" {
		t.Errorf("Tags = %v", article.Tags())
	}
	if article.ETA() == nil || *article.ETA() != eta {
		t.Errorf("ETA = %v", article.ETA())
	}
	if article.Priority() != "normal" {
		t.Errorf("Priority = %q", article.Priority())
	}
}

func TestNewNewsArticle_EmptySlug(t *testing.T) {
	_, err := domain.NewNewsArticle("", "Title", time.Now(), domain.NewsStatusInfo, nil, "body", nil, "")
	if err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestNewNewsArticle_EmptyTitle(t *testing.T) {
	_, err := domain.NewNewsArticle("slug", "", time.Now(), domain.NewsStatusInfo, nil, "body", nil, "")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestNewNewsArticle_InvalidStatus(t *testing.T) {
	_, err := domain.NewNewsArticle("slug", "Title", time.Now(), domain.NewsStatus("bogus"), nil, "body", nil, "")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestNewNewsArticle_ExcerptTruncation(t *testing.T) {
	longBody := ""
	for i := 0; i < 50; i++ {
		longBody += "word "
	}
	article, err := domain.NewNewsArticle("s", "T", time.Now(), domain.NewsStatusInfo, nil, longBody, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(article.Excerpt()) > 210 {
		t.Errorf("excerpt too long: %d chars", len(article.Excerpt()))
	}
}

func TestNewNewsArticle_TagsDefensiveCopy(t *testing.T) {
	tags := []string{"a", "b"}
	article, err := domain.NewNewsArticle("s", "T", time.Now(), domain.NewsStatusInfo, tags, "body", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tags[0] = "mutated"
	if article.Tags()[0] == "mutated" {
		t.Error("tags were not defensively copied")
	}
}

func TestIsValidNewsStatus(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"planned", true},
		{"released", true},
		{"info", true},
		{"", false},
		{"bogus", false},
		{"PLANNED", false},
	}
	for _, tt := range tests {
		if got := domain.IsValidNewsStatus(tt.input); got != tt.want {
			t.Errorf("IsValidNewsStatus(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
