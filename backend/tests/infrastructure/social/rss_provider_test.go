package social_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "pano_chart/backend/domain/social"
	infrasocial "pano_chart/backend/infrastructure/social"
)

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
  <channel>
    <title>realDonaldTrump / @realDonaldTrump</title>
    <item>
      <title>RT by @realDonaldTrump: Great news for America!</title>
      <dc:creator>@realDonaldTrump</dc:creator>
      <link>https://nitter.example.com/realDonaldTrump/status/123</link>
      <guid>https://nitter.example.com/realDonaldTrump/status/123#m</guid>
      <pubDate>Sat, 14 Jun 2025 10:00:00 GMT</pubDate>
      <description><![CDATA[<p>Great news for America!</p>]]></description>
    </item>
    <item>
      <title>Another tweet by @realDonaldTrump</title>
      <dc:creator>@realDonaldTrump</dc:creator>
      <link>https://nitter.example.com/realDonaldTrump/status/122</link>
      <guid>https://nitter.example.com/realDonaldTrump/status/122#m</guid>
      <pubDate>Fri, 13 Jun 2025 15:30:00 GMT</pubDate>
      <description><![CDATA[<p>Another tweet</p>]]></description>
    </item>
  </channel>
</rss>`

func TestRSSProvider_FetchParsesPosts(t *testing.T) {
	// Serve sample RSS on a test HTTP server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realDonaldTrump/rss" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, sampleRSS)
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{
		ID:       "twitter:realDonaldTrump",
		Platform: "twitter",
		Handle:   "realDonaldTrump",
	}

	posts, err := provider.Fetch(acc)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}

	// First post.
	if posts[0].ID != "https://nitter.example.com/realDonaldTrump/status/123#m" {
		t.Errorf("unexpected post[0].ID: %s", posts[0].ID)
	}
	if posts[0].Author != "@realDonaldTrump" {
		t.Errorf("unexpected post[0].Author: %s", posts[0].Author)
	}
	if posts[0].AccountID != "twitter:realDonaldTrump" {
		t.Errorf("unexpected post[0].AccountID: %s", posts[0].AccountID)
	}
	if posts[0].Timestamp == 0 {
		t.Error("expected non-zero timestamp for post[0]")
	}

	// Second post should have older timestamp.
	if posts[1].Timestamp >= posts[0].Timestamp {
		t.Errorf("expected post[1] to be older, got %d >= %d",
			posts[1].Timestamp, posts[0].Timestamp)
	}
}

func TestRSSProvider_Platform(t *testing.T) {
	provider := infrasocial.NewRSSProvider("http://localhost:8081", nil)
	if provider.Platform() != "twitter" {
		t.Fatalf("expected 'twitter', got '%s'", provider.Platform())
	}
}

func TestRSSProvider_HandlesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:test", Handle: "test"}

	_, err := provider.Fetch(acc)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRSSProvider_HandlesInvalidXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not xml at all")
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:test", Handle: "test"}

	_, err := provider.Fetch(acc)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}
