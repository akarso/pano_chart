package notifications

import (
	"context"
	"log"
	"time"
)

// Sender abstracts the delivery mechanism for broadcast notifications.
type Sender interface {
	Broadcast(ctx context.Context, n Notification) error
	// SendToUser delivers a notification only to the specified user's devices.
	SendToUser(ctx context.Context, userID string, n Notification) error
}

// EngineConfig holds tunables for the notification engine.
type EngineConfig struct {
	// AllowedStart is the earliest hour (inclusive) notifications may be sent.
	AllowedStart int // default 7
	// AllowedEnd is the latest hour (inclusive) notifications may be sent.
	AllowedEnd int // default 22
	// DefaultDedupTTL is applied when no per-type TTL is configured.
	DefaultDedupTTL time.Duration
}

// DefaultEngineConfig returns production defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		AllowedStart:    7,
		AllowedEnd:      22,
		DefaultDedupTTL: 24 * time.Hour,
	}
}

// Engine is the central notification dispatcher.
// It enforces a quiet-hours window and deduplication before forwarding
// to the Sender.
type Engine struct {
	sender Sender
	dedup  *Deduplicator
	cfg    EngineConfig
	now    func() time.Time // injectable clock
}

// NewEngine creates a notification engine.
func NewEngine(sender Sender, cfg EngineConfig) *Engine {
	return &Engine{
		sender: sender,
		dedup:  NewDeduplicator(),
		cfg:    cfg,
		now:    time.Now,
	}
}

// Send checks quiet hours, dedup, then dispatches through the Sender.
// Returns nil (silently dropped) for quiet-hour or duplicate hits.
func (e *Engine) Send(ctx context.Context, n Notification) error {
	if !e.withinAllowedHours() {
		log.Printf("[notify] suppressed (quiet hours): type=%s key=%s", n.Type, n.Key)
		return nil
	}

	if !e.dedup.TryReserve(n.Key, e.cfg.DefaultDedupTTL) {
		log.Printf("[notify] suppressed (dedup): type=%s key=%s", n.Type, n.Key)
		return nil
	}

	log.Printf("[notify] sending: type=%s title=%q key=%s", n.Type, n.Title, n.Key)
	err := e.sender.Broadcast(ctx, n)
	if err != nil {
		e.dedup.Release(n.Key)
		return err
	}
	return nil
}

// SendToUser checks quiet hours and per-user dedup, then delivers to a
// single user's devices.
func (e *Engine) SendToUser(ctx context.Context, userID string, n Notification) error {
	if !e.withinAllowedHours() {
		log.Printf("[notify] suppressed (quiet hours): type=%s key=%s user=%s", n.Type, n.Key, userID)
		return nil
	}

	perUserKey := userID + ":" + n.Key
	if !e.dedup.TryReserve(perUserKey, e.cfg.DefaultDedupTTL) {
		log.Printf("[notify] suppressed (dedup): type=%s key=%s user=%s", n.Type, n.Key, userID)
		return nil
	}

	log.Printf("[notify] sending to user=%s: type=%s title=%q key=%s", userID, n.Type, n.Title, n.Key)
	err := e.sender.SendToUser(ctx, userID, n)
	if err != nil {
		e.dedup.Release(perUserKey)
		return err
	}
	return nil
}

func (e *Engine) withinAllowedHours() bool {
	hour := e.now().Hour()
	return hour >= e.cfg.AllowedStart && hour <= e.cfg.AllowedEnd
}

// SetClock overrides the time source (for testing).
func (e *Engine) SetClock(fn func() time.Time) { e.now = fn }
