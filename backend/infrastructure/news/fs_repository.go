package news

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"pano_chart/backend/domain"
)

// filenamePattern matches YYYY-MM-DD-slug.md files.
var filenamePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)\.md$`)

// Frontmatter holds the YAML frontmatter fields.
type Frontmatter struct {
	Title    string `yaml:"title"`
	Date     string `yaml:"date"`
	Status   string `yaml:"status"`
	Tags     string `yaml:"tags"`
	ETA      string `yaml:"eta"`
	Priority string `yaml:"priority"`
}

// FsNewsRepository reads news articles from the filesystem.
// Articles are cached in memory and refreshed after TTL expiration.
type FsNewsRepository struct {
	dir string

	mu       sync.RWMutex
	articles []domain.NewsArticle
	loadedAt time.Time
	ttl      time.Duration
}

// NewFsNewsRepository creates a repository that reads .md files from dir.
func NewFsNewsRepository(dir string, cacheTTL time.Duration) *FsNewsRepository {
	return &FsNewsRepository{
		dir: dir,
		ttl: cacheTTL,
	}
}

// List returns articles sorted by date descending, limited to count.
func (r *FsNewsRepository) List(ctx context.Context, limit int) ([]domain.NewsArticle, error) {
	articles, err := r.loadArticles()
	if err != nil {
		return nil, err
	}
	if limit > 0 && limit < len(articles) {
		articles = articles[:limit]
	}
	return articles, nil
}

// GetBySlug returns a single article by its slug.
func (r *FsNewsRepository) GetBySlug(ctx context.Context, slug string) (domain.NewsArticle, error) {
	articles, err := r.loadArticles()
	if err != nil {
		return domain.NewsArticle{}, err
	}
	for _, a := range articles {
		if a.Slug() == slug {
			return a, nil
		}
	}
	return domain.NewsArticle{}, fmt.Errorf("news article not found: %s", slug)
}

func (r *FsNewsRepository) loadArticles() ([]domain.NewsArticle, error) {
	r.mu.RLock()
	if r.articles != nil && time.Since(r.loadedAt) < r.ttl {
		defer r.mu.RUnlock()
		return r.articles, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if r.articles != nil && time.Since(r.loadedAt) < r.ttl {
		return r.articles, nil
	}

	articles, err := r.readFromDisk()
	if err != nil {
		return nil, err
	}

	r.articles = articles
	r.loadedAt = time.Now()
	return articles, nil
}

func (r *FsNewsRepository) readFromDisk() ([]domain.NewsArticle, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read news directory: %w", err)
	}

	var articles []domain.NewsArticle
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := filenamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}

		dateStr := matches[1]
		slug := matches[2]

		content, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			continue // skip unreadable files
		}

		article, err := ParseArticle(slug, dateStr, string(content))
		if err != nil {
			continue // skip malformed files
		}

		articles = append(articles, article)
	}

	// Sort by date descending
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].Date().After(articles[j].Date())
	})

	return articles, nil
}

// ParseArticle parses a markdown file with YAML frontmatter.
func ParseArticle(slug, dateStr, content string) (domain.NewsArticle, error) {
	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		return domain.NewsArticle{}, err
	}

	// Use frontmatter date, fallback to filename date
	dateSource := fm.Date
	if dateSource == "" {
		dateSource = dateStr
	}

	date, err := time.Parse("2006-01-02", dateSource)
	if err != nil {
		return domain.NewsArticle{}, fmt.Errorf("invalid date %q: %w", dateSource, err)
	}

	status := domain.NewsStatus(fm.Status)
	if !domain.IsValidNewsStatus(fm.Status) {
		status = domain.NewsStatusInfo // default to info
	}

	// Parse comma-separated tags
	var tags []string
	for _, t := range strings.Split(fm.Tags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	// Parse optional ETA
	var eta *time.Time
	if fm.ETA != "" {
		parsed, err := time.Parse("2006-01-02", fm.ETA)
		if err == nil {
			eta = &parsed
		}
	}

	title := fm.Title
	if title == "" {
		title = slug // fallback to slug
	}

	return domain.NewNewsArticle(slug, title, date, status, tags, strings.TrimSpace(body), eta, fm.Priority)
}

// ParseFrontmatter splits a markdown file into Frontmatter and body.
func ParseFrontmatter(content string) (Frontmatter, string, error) {
	var fm Frontmatter

	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return fm, content, nil // no frontmatter, entire content is body
	}

	// Find closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return fm, content, nil
	}

	yamlContent := rest[:idx]
	body := rest[idx+4:] // skip \n---

	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return fm, "", fmt.Errorf("parse frontmatter YAML: %w", err)
	}

	return fm, body, nil
}
