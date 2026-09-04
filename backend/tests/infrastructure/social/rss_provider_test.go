package social_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	posts, err := provider.Fetch(context.Background(), acc)
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

	_, err := provider.Fetch(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// TestRSSProvider_AbortsOnContextCancellation is the regression test for
// PR-076 CR follow-up: Fetch must actually abort an in-flight request when
// its context is cancelled, not merely rely on the client's own Timeout —
// otherwise Watcher shutdown can't count on pollNext returning promptly
// (see application/social.Watcher.pollNext and domain/social.Provider).
// Uses a handler that blocks until the request context is cancelled (never
// finishes on its own) to prove cancellation — not the client's Timeout —
// is what unblocks Fetch.
func TestRSSProvider_AbortsOnContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never responds on its own
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:test", Handle: "test"}

	ctx, cancel := context.WithCancel(context.Background())

	fetchDone := make(chan error, 1)
	go func() {
		_, err := provider.Fetch(ctx, acc)
		fetchDone <- err
	}()

	// Give Fetch time to actually be in-flight before cancelling, so this
	// exercises cancellation of a request already sent, not a pre-send
	// short-circuit.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-fetchDone:
		if err == nil {
			t.Fatal("expected an error from a cancelled fetch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fetch did not return promptly after context cancellation — " +
			"it's blocking on something other than ctx, defeating graceful shutdown")
	}
}

func TestRSSProvider_HandlesInvalidXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not xml at all")
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:test", Handle: "test"}

	_, err := provider.Fetch(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

const htmlEntityRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>test / @test</title>
    <item>
      <title>It&apos;s a great day &amp; I can&apos;t wait</title>
      <dc:creator>@test</dc:creator>
      <link>https://example.com/1</link>
      <guid>guid1</guid>
      <pubDate>Mon, 01 Jan 2024 12:00:00 GMT</pubDate>
    </item>
    <item>
      <title>RT @someone: Big announcement &#x2014; breaking news</title>
      <dc:creator>@someone</dc:creator>
      <link>https://example.com/2</link>
      <guid>guid2</guid>
      <pubDate>Mon, 01 Jan 2024 11:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

func TestRSSProvider_UnescapesHTMLEntities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, htmlEntityRSS)
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:test", Platform: "twitter", Handle: "test"}

	posts, err := provider.Fetch(context.Background(), acc)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}

	// Apostrophes and ampersands should be unescaped.
	expected := "It's a great day & I can't wait"
	if posts[0].Title != expected {
		t.Errorf("expected title %q, got %q", expected, posts[0].Title)
	}
}

const htmlTagRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>leon / @leon</title>
    <item>
      <title>&lt;p&gt;true&lt;/p&gt;&lt;span class=&quot;reply&quot;&gt;in reply to @someone&lt;/span&gt;</title>
      <dc:creator>@leon</dc:creator>
      <link>https://example.com/1</link>
      <guid>guid-html1</guid>
      <pubDate>Mon, 01 Jan 2024 12:00:00 GMT</pubDate>
    </item>
    <item>
      <title>Clean text without any markup at all</title>
      <dc:creator>@leon</dc:creator>
      <link>https://example.com/2</link>
      <guid>guid-html2</guid>
      <pubDate>Mon, 01 Jan 2024 11:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

func TestRSSProvider_StripsHTMLTagsFromTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, htmlTagRSS)
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:leon", Platform: "twitter", Handle: "leon"}

	posts, err := provider.Fetch(context.Background(), acc)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}

	// HTML tags should be stripped; only plain text remains.
	expected := "truein reply to @someone"
	if posts[0].Title != expected {
		t.Errorf("expected title %q, got %q", expected, posts[0].Title)
	}

	// Clean title should be unchanged.
	if posts[1].Title != "Clean text without any markup at all" {
		t.Errorf("unexpected title: %q", posts[1].Title)
	}
}

func TestRSSProvider_DetectsRetweets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, htmlEntityRSS)
	}))
	defer srv.Close()

	provider := infrasocial.NewRSSProvider(srv.URL, srv.Client())
	acc := domain.Account{ID: "twitter:test", Platform: "twitter", Handle: "test"}

	posts, err := provider.Fetch(context.Background(), acc)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// First post: own author, original title → not a retweet.
	if posts[0].IsRetweet {
		t.Error("expected post[0] to not be a retweet")
	}

	// Second post: "RT @someone" prefix and different creator → retweet.
	if !posts[1].IsRetweet {
		t.Error("expected post[1] to be a retweet")
	}
}
