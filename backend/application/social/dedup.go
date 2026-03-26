package social

import domain "pano_chart/backend/domain/social"

// FilterNew returns only posts that have not yet been seen for the given
// account. Posts are assumed to be in reverse-chronological order (newest
// first), which is the standard RSS item ordering. The function stops at
// the first post whose ID matches account.LastSeenPostID.
//
// When LastSeenPostID is set but not found in the post list (e.g. it
// rotated off a high-volume feed), the function falls back to
// LastSeenTimestamp: only posts newer than that timestamp are returned.
// If neither ID nor timestamp is available the function returns all posts
// (first-poll behaviour).
func FilterNew(account domain.Account, posts []domain.Post) []domain.Post {
	if account.LastSeenPostID == "" {
		// First poll — everything is new.
		return posts
	}

	result := make([]domain.Post, 0, len(posts))
	found := false
	for _, p := range posts {
		if p.ID == account.LastSeenPostID {
			found = true
			break
		}
		result = append(result, p)
	}

	if found {
		return result
	}

	// LastSeenPostID was not found in the feed — it has rotated off.
	// Fall back to timestamp comparison if available.
	if account.LastSeenTimestamp > 0 {
		filtered := make([]domain.Post, 0, len(posts))
		for _, p := range posts {
			if p.Timestamp > account.LastSeenTimestamp {
				filtered = append(filtered, p)
			}
		}
		return filtered
	}

	// No timestamp available either — return all (first-poll-like).
	return posts
}
