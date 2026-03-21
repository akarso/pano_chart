package social

// Post represents a single social media post.
type Post struct {
	ID        string // unique identifier (e.g. tweet GUID)
	AccountID string // owning account ID (platform:handle)
	Author    string // display author (may differ from account for RTs)
	Title     string // post title / text summary
	URL       string // permalink
	Timestamp int64  // unix timestamp of publication
}
