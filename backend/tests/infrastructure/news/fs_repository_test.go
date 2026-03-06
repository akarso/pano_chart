package news_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pano_chart/backend/infrastructure/news"
)

func TestParseFrontmatter_ValidContent(t *testing.T) {
	content := "---\ntitle: Test Article\ndate: 2026-03-05\nstatus: planned\ntags: feature, timeframe\neta: 2026-03-20\npriority: normal\n---\n\nThis is the body."

	fm, body, err := news.ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Title != "Test Article" {
		t.Errorf("Title = %q", fm.Title)
	}
	if fm.Date != "2026-03-05" {
		t.Errorf("Date = %q", fm.Date)
	}
	if fm.Status != "planned" {
		t.Errorf("Status = %q", fm.Status)
	}
	if fm.Tags != "feature, timeframe" {
		t.Errorf("Tags = %q", fm.Tags)
	}
	if fm.ETA != "2026-03-20" {
		t.Errorf("ETA = %q", fm.ETA)
	}
	if fm.Priority != "normal" {
		t.Errorf("Priority = %q", fm.Priority)
	}
	if len(body) == 0 {
		t.Error("body should not be empty")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just plain text."
	_, body, err := news.ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func TestParseFrontmatter_MissingClosingDelimiter(t *testing.T) {
	content := "---\ntitle: Broken\nNo closing delimiter"
	_, body, err := news.ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != content {
		t.Errorf("body = %q", body)
	}
}

func TestParseArticle_ValidFile(t *testing.T) {
	content := "---\ntitle: Fear and Greed Index Added\ndate: 2026-03-02\nstatus: released\ntags: feature, sentiment\n---\n\nThe Index is now accessible."

	article, err := news.ParseArticle("fear-greed-index-added", "2026-03-02", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if article.Slug() != "fear-greed-index-added" {
		t.Errorf("Slug = %q", article.Slug())
	}
	if article.Title() != "Fear and Greed Index Added" {
		t.Errorf("Title = %q", article.Title())
	}
	if article.Status() != "released" {
		t.Errorf("Status = %q", article.Status())
	}
	tags := article.Tags()
	if len(tags) != 2 || tags[0] != "feature" || tags[1] != "sentiment" {
		t.Errorf("Tags = %v", tags)
	}
}

func TestParseArticle_FallbackToFileDate(t *testing.T) {
	content := "---\ntitle: No Date In Frontmatter\nstatus: info\n---\n\nBody text."

	article, err := news.ParseArticle("slug", "2026-01-15", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !article.Date().Equal(expected) {
		t.Errorf("Date = %v, want %v", article.Date(), expected)
	}
}

func TestParseArticle_FallbackStatus(t *testing.T) {
	content := "---\ntitle: Bad Status\nstatus: unknown_value\n---\n\nBody."

	article, err := news.ParseArticle("slug", "2026-01-01", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if article.Status() != "info" {
		t.Errorf("Status = %q, want info (fallback)", article.Status())
	}
}

func TestParseArticle_WithETA(t *testing.T) {
	content := "---\ntitle: With ETA\ndate: 2026-03-05\nstatus: planned\neta: 2026-03-20\n---\n\nBody."

	article, err := news.ParseArticle("slug", "2026-03-05", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if article.ETA() == nil {
		t.Fatal("ETA should not be nil")
	}
	expected := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	if !article.ETA().Equal(expected) {
		t.Errorf("ETA = %v, want %v", article.ETA(), expected)
	}
}

func TestParseArticle_TitleFallbackToSlug(t *testing.T) {
	content := "---\nstatus: info\n---\n\nNo title in frontmatter."

	article, err := news.ParseArticle("my-slug", "2026-01-01", content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if article.Title() != "my-slug" {
		t.Errorf("Title = %q, want slug fallback %q", article.Title(), "my-slug")
	}
}

func TestFsNewsRepository_ListAndGetBySlug(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "2026-03-05-feature-a.md", "---\ntitle: Feature A\ndate: 2026-03-05\nstatus: released\ntags: feature\n---\n\nFeature A body.")
	writeFile(t, dir, "2026-03-01-feature-b.md", "---\ntitle: Feature B\ndate: 2026-03-01\nstatus: planned\ntags: feature\n---\n\nFeature B body.")
	writeFile(t, dir, "README.md", "Not a news file")

	repo := news.NewFsNewsRepository(dir, 0)

	articles, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].Slug() != "feature-a" {
		t.Errorf("articles[0].Slug = %q, want feature-a (newest first)", articles[0].Slug())
	}
	if articles[1].Slug() != "feature-b" {
		t.Errorf("articles[1].Slug = %q", articles[1].Slug())
	}

	limited, err := repo.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 article, got %d", len(limited))
	}

	article, err := repo.GetBySlug(context.Background(), "feature-b")
	if err != nil {
		t.Fatalf("GetBySlug error: %v", err)
	}
	if article.Title() != "Feature B" {
		t.Errorf("Title = %q", article.Title())
	}

	_, err = repo.GetBySlug(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent slug")
	}
}

func TestFsNewsRepository_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	repo := news.NewFsNewsRepository(dir, 0)

	articles, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 0 {
		t.Errorf("expected 0 articles, got %d", len(articles))
	}
}

func TestFsNewsRepository_NonexistentDirectory(t *testing.T) {
	repo := news.NewFsNewsRepository("/nonexistent/path", 0)

	_, err := repo.List(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write test file %s: %v", name, err)
	}
}
