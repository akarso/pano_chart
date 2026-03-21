package social

import domain "pano_chart/backend/domain/social"

// FilterNew returns only posts that have not yet been seen for the given
// account. Posts are assumed to be in reverse-chronological order (newest
// first), which is the standard RSS item ordering. The function stops at
// the first post whose ID matches account.LastSeenPostID.
func FilterNew(account domain.Account, posts []domain.Post) []domain.Post {
	if account.LastSeenPostID == "" {
		// First poll — everything is new.
		return posts
	}

	result := make([]domain.Post, 0, len(posts))
	for _, p := range posts {
		if p.ID == account.LastSeenPostID {
			break
		}
		result = append(result, p)
	}
	return result
}
