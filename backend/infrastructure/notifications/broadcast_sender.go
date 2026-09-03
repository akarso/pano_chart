package notifications

import (
	"context"
	"fmt"
	"log"

	appnotify "pano_chart/backend/application/notifications"
	appsocial "pano_chart/backend/application/social"
)

// Compile-time check.
var _ appnotify.Sender = (*BroadcastSender)(nil)

// BroadcastSender delivers notifications to every registered device
// via the existing FCM push infrastructure.
type BroadcastSender struct {
	devices appsocial.DeviceTokenStore
	push    appsocial.PushNotifier
}

// NewBroadcastSender creates a sender that broadcasts to all device tokens.
func NewBroadcastSender(
	devices appsocial.DeviceTokenStore,
	push appsocial.PushNotifier,
) *BroadcastSender {
	return &BroadcastSender{devices: devices, push: push}
}

// Broadcast sends the notification to every registered FCM token.
func (b *BroadcastSender) Broadcast(ctx context.Context, n appnotify.Notification) error {
	tokens, err := b.devices.AllTokens()
	if err != nil {
		return fmt.Errorf("fetching tokens: %w", err)
	}
	if len(tokens) == 0 {
		log.Printf("[broadcast] no registered devices — skipping %s", n.Key)
		return nil
	}

	log.Printf("[broadcast] sending %q to %d device(s)", n.Title, len(tokens))

	var lastErr error
	for _, tok := range tokens {
		if err := b.push.Send(ctx, tok, n.Title, n.Body, n.Data); err != nil {
			log.Printf("[broadcast] send error token=…%s: %v", lastN(tok, 8), err)
			lastErr = err
		}
	}
	return lastErr
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// SendToUser sends the notification only to devices belonging to userID.
func (b *BroadcastSender) SendToUser(ctx context.Context, userID string, n appnotify.Notification) error {
	tokens, err := b.devices.TokensForUsers([]string{userID})
	if err != nil {
		return fmt.Errorf("fetching tokens for user %s: %w", userID, err)
	}
	if len(tokens) == 0 {
		log.Printf("[broadcast] no devices for user %s — skipping %s", userID, n.Key)
		return nil
	}

	log.Printf("[broadcast] sending %q to %d device(s) for user %s", n.Title, len(tokens), userID)

	var lastErr error
	for _, tok := range tokens {
		if err := b.push.Send(ctx, tok, n.Title, n.Body, n.Data); err != nil {
			log.Printf("[broadcast] send error token=…%s: %v", lastN(tok, 8), err)
			lastErr = err
		}
	}
	return lastErr
}
