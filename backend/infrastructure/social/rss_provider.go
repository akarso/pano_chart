package social

import (
	"encoding/xml"
	"fmt"
	html "html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	domain "pano_chart/backend/domain/social"
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// ── RSS XML structures ──────────────────────────────────────────────────────

type rssDoc struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Creator     string `xml:"creator"`
}

// ── RSSProvider ─────────────────────────────────────────────────────────────

// RSSProvider fetches social posts via an RSS/Nitter bridge.
//
// URL pattern: {baseURL}/{handle}/rss
type RSSProvider struct {
	baseURL string       // e.g. "http://127.0.0.1:8081"
	client  *http.Client // shared, connection-pooled
}

// NewRSSProvider constructs a provider that talks to the Nitter RSS bridge.
func NewRSSProvider(baseURL string, client *http.Client) *RSSProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &RSSProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

// Platform implements social.Provider.
func (p *RSSProvider) Platform() string { return "twitter" }

// Fetch implements social.Provider.
func (p *RSSProvider) Fetch(account domain.Account) ([]domain.Post, error) {
	url := fmt.Sprintf("%s/%s/rss", p.baseURL, account.Handle)

	resp, err := p.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("rss fetch %s: %w", account.Handle, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss fetch %s: status %d", account.Handle, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rss read %s: %w", account.Handle, err)
	}

	return parseRSS(account.ID, body)
}

// parseRSS turns raw RSS XML into domain posts.
func parseRSS(accountID string, data []byte) ([]domain.Post, error) {
	var doc rssDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("rss parse: %w", err)
	}

	// Extract the handle from the accountID for retweet detection.
	ownHandle := accountID
	if idx := strings.Index(accountID, ":"); idx >= 0 {
		ownHandle = accountID[idx+1:]
	}

	posts := make([]domain.Post, 0, len(doc.Channel.Items))
	for _, item := range doc.Channel.Items {
		ts := parseRSSDate(item.PubDate)
		author := item.Creator
		if author == "" {
			// fallback: extract from accountID
			if idx := strings.Index(accountID, ":"); idx >= 0 {
				author = "@" + accountID[idx+1:]
			}
		}

		// Strip HTML tags and unescape entities so Title is plain text.
		title := html.UnescapeString(htmlTagRe.ReplaceAllString(item.Title, ""))
		title = strings.TrimSpace(title)

		// Detect retweets: title starts with "RT @" or author differs from
		// the owning account.
		isRT := strings.HasPrefix(title, "RT @") ||
			(author != "" && !strings.EqualFold(strings.TrimPrefix(author, "@"), ownHandle))

		posts = append(posts, domain.Post{
			ID:        item.GUID,
			AccountID: accountID,
			Author:    author,
			Title:     title,
			URL:       item.Link,
			Timestamp: ts,
			IsRetweet: isRT,
		})
	}
	return posts, nil
}

// parseRSSDate tries RFC1123 / RFC1123Z / RFC822 / RFC822Z formats used by
// RSS feeds.
func parseRSSDate(s string) int64 {
	for _, layout := range []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}
