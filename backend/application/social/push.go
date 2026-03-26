package social

import (
	"context"
	"fmt"
	"log"

	domain "pano_chart/backend/domain/social"
)

// PushNotifier sends push notifications to devices.
type PushNotifier interface {
	// Send delivers a push notification to the given FCM token.
	// data is an optional key-value map included in the FCM data payload
	// for client-side deep-link routing.
	Send(ctx context.Context, token, title, body string, data map[string]string) error
}

// PushConsumer reads new-post events from the dispatcher channel and sends
// push notifications to all subscribers' devices.
type PushConsumer struct {
	events  <-chan []domain.Post
	subs    SubscriptionStore
	devices DeviceTokenStore
	push    PushNotifier
}

// NewPushConsumer creates a consumer wired to the dispatcher's event channel.
func NewPushConsumer(
	events <-chan []domain.Post,
	subs SubscriptionStore,
	devices DeviceTokenStore,
	push PushNotifier,
) *PushConsumer {
	return &PushConsumer{
		events:  events,
		subs:    subs,
		devices: devices,
		push:    push,
	}
}

// Run blocks until ctx is cancelled, reading new-post batches and sending
// push notifications for each.
func (c *PushConsumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case posts, ok := <-c.events:
			if !ok {
				return
			}
			log.Printf("[push] received batch of %d posts", len(posts))
			c.handleBatch(ctx, posts)
		}
	}
}

// handleBatch groups posts by account, resolves subscribers → device tokens,
// applies per-user filter settings, and sends one notification per token.
func (c *PushConsumer) handleBatch(ctx context.Context, posts []domain.Post) {
	// Group posts by account to avoid duplicate lookups.
	byAccount := make(map[string][]domain.Post)
	for _, p := range posts {
		byAccount[p.AccountID] = append(byAccount[p.AccountID], p)
	}

	for accountID, accountPosts := range byAccount {
		// Load per-user filter configs for this account.
		filtersByUser, err := c.subs.FilterConfigForAccount(accountID)
		if err != nil {
			log.Printf("[push] filters for %s: %v", accountID, err)
			continue
		}
		log.Printf("[push] account %s: %d subscribers", accountID, len(filtersByUser))
		if len(filtersByUser) == 0 {
			continue
		}

		title := fmt.Sprintf("@%s", extractHandle(accountID))

		for userID, filter := range filtersByUser {
			// Apply this user's filter to the posts.
			filtered := applyFilter(accountPosts, filter)
			if len(filtered) == 0 {
				log.Printf("[push] all posts filtered for user %s on %s", userID, accountID)
				continue
			}

			tokens, err := c.devices.TokensForUsers([]string{userID})
			if err != nil {
				log.Printf("[push] tokens for user %s: %v", userID, err)
				continue
			}
			if len(tokens) == 0 {
				continue
			}

			// Build notification body from filtered posts.
			newest := filtered[0]
			body := newest.Title
			if len(filtered) > 1 {
				body = fmt.Sprintf("%s (+%d more)", newest.Title, len(filtered)-1)
			}

			for _, token := range tokens {
				if err := c.push.Send(ctx, token, title, body, map[string]string{"type": "twitter"}); err != nil {
					log.Printf("[push] send to token …%s: %v", lastN(token, 8), err)
				} else {
					log.Printf("[push] sent to …%s: %s", lastN(token, 8), title)
				}
			}
		}
	}
}

// extractHandle returns the handle portion from "platform:handle".
func extractHandle(accountID string) string {
	for i := range accountID {
		if accountID[i] == ':' {
			return accountID[i+1:]
		}
	}
	return accountID
}

// lastN returns the last n characters of s, or all of s if shorter.
func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
